package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// safeBuffer is a bytes.Buffer that tolerates concurrent writes from the recorder.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) events(t *testing.T) []CallEvent {
	t.Helper()
	b.mu.Lock()
	raw := b.buf.String()
	b.mu.Unlock()

	var out []CallEvent
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var e CallEvent
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			t.Fatalf("log line is not JSON: %v (%s)", err, line)
		}
		out = append(out, e)
	}
	return out
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestMaskPIIKeepsBusinessIdentifiers(t *testing.T) {
	in := "手机 13812345678 身份证 440305199001011234 邮箱 cas@example.com 订单 ORD-10003 退款 128.00"
	out := maskPII(in)
	for _, leaked := range []string{"13812345678", "440305199001011234", "cas@example.com"} {
		if strings.Contains(out, leaked) {
			t.Fatalf("%s leaked: %s", leaked, out)
		}
	}
	// The identifiers an operator actually needs must survive.
	for _, kept := range []string{"138****5678", "440305********1234", "ORD-10003", "128.00", "c***@example.com"} {
		if !strings.Contains(out, kept) {
			t.Fatalf("expected %q in %s", kept, out)
		}
	}
}

func TestRecorderDetailOmitsBodies(t *testing.T) {
	sink := &safeBuffer{}
	rec := NewRecorder(RecorderOptions{Level: CallLogDetail, Logger: discardLogger(), Writer: sink})

	ev := CallEvent{CallID: "c1", Model: "m", RequestBody: "SECRET-BODY", Messages: []CallMessage{{Role: "user", Chars: 3}}}
	rec.start(ev)
	rec.end(ev)

	events := sink.events(t)
	if len(events) != 2 {
		t.Fatalf("events = %d, want 2 (start + end)", len(events))
	}
	if events[0].Phase != "start" || events[1].Phase != "end" {
		t.Fatalf("phases = %q, %q", events[0].Phase, events[1].Phase)
	}
	// Request material belongs to the start line only: duplicating it in the end
	// line doubles the file for nothing.
	if len(events[0].Messages) != 1 {
		t.Fatalf("detail level should keep message summaries on start, got %d", len(events[0].Messages))
	}
	for _, e := range events {
		if e.RequestBody != "" {
			t.Fatal("request body must not be recorded below CallLogFull")
		}
		if len(e.Messages) != 0 && e.Phase == "end" {
			t.Fatal("end line must not repeat the messages")
		}
	}
}

func TestRecorderFullMasksAndCapsBodies(t *testing.T) {
	sink := &safeBuffer{}
	rec := NewRecorder(RecorderOptions{Level: CallLogFull, BodyChars: 40, Logger: discardLogger(), Writer: sink})

	body := "用户电话 13812345678 说" + strings.Repeat("废话", 100)
	rec.start(CallEvent{CallID: "c1", RequestBody: rec.cap(body)})

	events := sink.events(t)
	if len(events) != 1 {
		t.Fatalf("events = %d", len(events))
	}
	if strings.Contains(events[0].RequestBody, "13812345678") {
		t.Fatal("phone number leaked into the request body")
	}
	if n := len([]rune(events[0].RequestBody)); n > 41 {
		t.Fatalf("body not capped: %d runes", n)
	}
}

func TestRecorderOffWritesNothing(t *testing.T) {
	sink := &safeBuffer{}
	rec := NewRecorder(RecorderOptions{Level: CallLogOff, Logger: discardLogger(), Writer: sink})
	rec.start(CallEvent{CallID: "c1"})
	rec.end(CallEvent{CallID: "c1"})
	if got := sink.buf.String(); got != "" {
		t.Fatalf("off level wrote %q", got)
	}
}

// TestOpenAICompatibleLogsSuccessfulCall covers the fields an operator needs to
// reconstruct one turn: correlation ids, timing, usage, and the tool call.
func TestOpenAICompatibleLogsSuccessfulCall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-request-id", "upstream-abc")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices":[{
				"message":{"content":"好的","tool_calls":[
					{"id":"call_1","type":"function","function":{"name":"get_order_detail","arguments":"{\"order_no\":\"ORD-10003\"}"}}
				]},
				"finish_reason":"tool_calls"
			}],
			"usage":{"prompt_tokens":120,"completion_tokens":18,"total_tokens":138}
		}`))
	}))
	defer srv.Close()

	sink := &safeBuffer{}
	model := NewOpenAICompatible(srv.URL, "secret-key", "demo-model", 5*time.Second)
	model.Recorder = NewRecorder(RecorderOptions{Level: CallLogDetail, Logger: discardLogger(), Writer: sink})

	_, err := model.Respond(context.Background(), ModelRequest{
		Messages: []Message{
			{Role: RoleSystem, Content: "你是客服"},
			{Role: RoleUser, Content: "ORD-10003 到哪了"},
		},
		Meta: CallMeta{ConversationID: "conv-1", RequestID: "req-1", TurnID: "turn-1", Round: 2},
	})
	if err != nil {
		t.Fatalf("respond: %v", err)
	}

	events := sink.events(t)
	if len(events) != 2 {
		t.Fatalf("events = %d, want start+end", len(events))
	}
	start, end := events[0], events[1]
	if start.CallID != end.CallID || start.CallID == "" {
		t.Fatalf("call ids do not pair: %q / %q", start.CallID, end.CallID)
	}
	if start.ConversationID != "conv-1" || start.RequestID != "req-1" || start.TurnID != "turn-1" || start.Round != 2 {
		t.Fatalf("correlation lost: %+v", start)
	}
	if start.MessageCount != 2 || len(start.Roles) != 2 || start.PromptChars == 0 {
		t.Fatalf("request shape missing: %+v", start)
	}
	if len(start.Messages) != 2 || start.Messages[1].Preview == "" {
		t.Fatalf("detail messages missing: %+v", start.Messages)
	}
	if !end.OK || end.HTTPStatus != 200 || end.FinishReason != "tool_calls" {
		t.Fatalf("end not marked ok: %+v", end)
	}
	if end.ProviderRequestID != "upstream-abc" {
		t.Fatalf("provider request id = %q", end.ProviderRequestID)
	}
	if end.Usage == nil || end.Usage.TotalTokens != 138 || end.Usage.PromptTokens != 120 {
		t.Fatalf("usage = %+v", end.Usage)
	}
	if len(end.ToolCalls) != 1 || end.ToolCalls[0].Name != "get_order_detail" ||
		!strings.Contains(end.ToolCalls[0].Arguments, "ORD-10003") {
		t.Fatalf("tool calls = %+v", end.ToolCalls)
	}
	if !strings.Contains(end.ContentPreview, "好的") {
		t.Fatalf("content preview = %q", end.ContentPreview)
	}
}

// TestOpenAICompatibleLogsUpstreamErrorBody is the case that used to be
// un-diagnosable: a 400 whose reason only exists in the response body.
func TestOpenAICompatibleLogsUpstreamErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"tools[0].function.name is invalid","type":"invalid_request_error"}}`))
	}))
	defer srv.Close()

	sink := &safeBuffer{}
	model := NewOpenAICompatible(srv.URL, "", "demo-model", 5*time.Second)
	model.Recorder = NewRecorder(RecorderOptions{Level: CallLogSummary, Logger: discardLogger(), Writer: sink})

	if _, err := model.Respond(context.Background(), ModelRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}}); err == nil {
		t.Fatal("expected error on 400")
	}

	events := sink.events(t)
	end := events[len(events)-1]
	if end.OK {
		t.Fatal("failed call marked ok")
	}
	if end.ErrorKind != errKindHTTPStatus || end.HTTPStatus != 400 {
		t.Fatalf("kind = %q status = %d", end.ErrorKind, end.HTTPStatus)
	}
	if !strings.Contains(end.UpstreamBody, "tools[0].function.name is invalid") {
		t.Fatalf("upstream body not kept: %q", end.UpstreamBody)
	}
	if end.Error == "" {
		t.Fatal("error text missing")
	}
}

// TestOpenAICompatibleLogsEachRetry makes retries visible: a "slow" answer that
// is really one 503 plus one success must not look like a single call.
func TestOpenAICompatibleLogsEachRetry(t *testing.T) {
	var n int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n++
		if n == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	sink := &safeBuffer{}
	model := NewOpenAICompatible(srv.URL, "", "demo-model", 5*time.Second)
	model.Recorder = NewRecorder(RecorderOptions{Level: CallLogSummary, Logger: discardLogger(), Writer: sink})

	if _, err := model.Respond(context.Background(), ModelRequest{Messages: []Message{{Role: RoleUser, Content: "hi"}}}); err != nil {
		t.Fatalf("respond: %v", err)
	}

	events := sink.events(t)
	if len(events) != 4 {
		t.Fatalf("events = %d, want 4 (2 attempts x start+end)", len(events))
	}
	if events[1].Attempt != 0 || events[1].OK {
		t.Fatalf("first attempt should fail: %+v", events[1])
	}
	if events[3].Attempt != 1 || !events[3].OK {
		t.Fatalf("second attempt should succeed: %+v", events[3])
	}
}

func TestParseCallLogLevel(t *testing.T) {
	cases := map[string]CallLogLevel{
		"":       CallLogSummary,
		"off":    CallLogOff,
		"DETAIL": CallLogDetail,
		" full ": CallLogFull,
	}
	for raw, want := range cases {
		got, err := ParseCallLogLevel(raw)
		if err != nil || got != want {
			t.Fatalf("ParseCallLogLevel(%q) = %v, %v", raw, got, err)
		}
	}
	if _, err := ParseCallLogLevel("verbose"); err == nil {
		t.Fatal("expected error for unknown level")
	}
}
