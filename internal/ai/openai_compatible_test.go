package ai_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/example/ecommerce-support/internal/ai"
)

func TestOpenAICompatibleParsesToolCalls(t *testing.T) {
	var captured map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret-key" {
			t.Errorf("authorization = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"choices":[{
				"message":{"content":"","tool_calls":[
					{"id":"call_1","type":"function","function":{"name":"get_order_detail","arguments":"{\"order_no\":\"ORD-10003\"}"}}
				]},
				"finish_reason":"tool_calls"
			}]
		}`))
	}))
	defer srv.Close()

	model := ai.NewOpenAICompatible(srv.URL+"/v1", "secret-key", "demo", 5*time.Second)
	resp, err := model.Respond(context.Background(), ai.ModelRequest{
		Messages: []ai.Message{{Role: ai.RoleUser, Content: "查订单"}},
		Tools:    []map[string]any{{"type": "function", "function": map[string]any{"name": "get_order_detail"}}},
	})
	if err != nil {
		t.Fatalf("respond: %v", err)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0].Name != "get_order_detail" {
		t.Fatalf("tool calls = %+v", resp.ToolCalls)
	}
	if resp.FinishReason != "tool_calls" {
		t.Fatalf("finish reason = %s", resp.FinishReason)
	}
	if captured["model"] != "demo" {
		t.Fatalf("model in payload = %v", captured["model"])
	}
	if _, ok := captured["tools"]; !ok {
		t.Fatal("tools were not sent")
	}
}

func TestOpenAICompatibleRetriesOn503(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer srv.Close()

	model := ai.NewOpenAICompatible(srv.URL, "", "demo", 5*time.Second)
	resp, err := model.Respond(context.Background(), ai.ModelRequest{Messages: []ai.Message{{Role: ai.RoleUser, Content: "hi"}}})
	if err != nil {
		t.Fatalf("respond: %v", err)
	}
	if resp.Content != "ok" {
		t.Fatalf("content = %q", resp.Content)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func TestOpenAICompatibleDoesNotRetryOn401(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	model := ai.NewOpenAICompatible(srv.URL, "bad", "demo", 5*time.Second)
	if _, err := model.Respond(context.Background(), ai.ModelRequest{Messages: []ai.Message{{Role: ai.RoleUser, Content: "hi"}}}); err == nil {
		t.Fatal("expected error on 401")
	}
	if atomic.LoadInt32(&calls) != 1 {
		t.Fatalf("calls = %d, want 1 (no retry on auth failure)", calls)
	}
}

func TestOpenAICompatibleRejectsEmptyConfig(t *testing.T) {
	model := ai.NewOpenAICompatible("", "", "", time.Second)
	_, err := model.Respond(context.Background(), ai.ModelRequest{})
	if err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("err = %v, want configuration error", err)
	}
}
