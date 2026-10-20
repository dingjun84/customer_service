package ai

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/example/ecommerce-support/internal/domain"
)

// This file owns the observability of model calls. Nothing here influences the
// conversation: a recorder may be nil, may fail to write, and the turn still
// completes. The goal is that any single turn can be reconstructed from the log
// alone — who asked, what exactly was sent, what came back, how long it took,
// and how much it cost in tokens.

// CallLogLevel controls how much of one model call is written out.
type CallLogLevel int

const (
	// CallLogOff disables model-call logging entirely.
	CallLogOff CallLogLevel = iota
	// CallLogSummary records correlation ids, request shape, timing, outcome and
	// token usage. No message text.
	CallLogSummary
	// CallLogDetail adds per-message roles plus truncated content and tool-call
	// arguments. This is the level to run when a turn misbehaves.
	CallLogDetail
	// CallLogFull adds the complete request and response bodies (masked and
	// size-capped). Use it to reproduce a provider-side complaint verbatim.
	CallLogFull
)

// String returns the environment-variable spelling of the level.
func (l CallLogLevel) String() string {
	switch l {
	case CallLogSummary:
		return "summary"
	case CallLogDetail:
		return "detail"
	case CallLogFull:
		return "full"
	default:
		return "off"
	}
}

// ParseCallLogLevel maps LLM_LOG_LEVEL to a level. An empty value means summary
// so that a misconfigured deployment still records correlation ids.
func ParseCallLogLevel(raw string) (CallLogLevel, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "summary":
		return CallLogSummary, nil
	case "off", "none", "disabled":
		return CallLogOff, nil
	case "detail":
		return CallLogDetail, nil
	case "full":
		return CallLogFull, nil
	default:
		return CallLogOff, fmt.Errorf("LLM_LOG_LEVEL must be off/summary/detail/full, got %q", raw)
	}
}

// CallMeta identifies the user request a model call belongs to. The caller fills
// it in; it is never sent to the provider.
type CallMeta struct {
	ConversationID string
	RequestID      string
	TurnID         string
	Round          int
}

// CallUsage is the token accounting reported by the provider.
type CallUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// CallToolCall is one tool call the model asked for, as seen in the response.
type CallToolCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
}

// CallMessage is the per-message projection recorded at CallLogDetail.
type CallMessage struct {
	Role       string   `json:"role"`
	ToolName   string   `json:"tool_name,omitempty"`
	ToolCallID string   `json:"tool_call_id,omitempty"`
	Chars      int      `json:"chars"`
	Preview    string   `json:"preview,omitempty"`
	Calls      []string `json:"calls,omitempty"`
}

// CallEvent is one structured line about one HTTP attempt. Phase "start" is
// written before the request leaves the process, phase "end" once the attempt
// returns, so a crashed or hung call still leaves evidence. Both lines share the
// same call_id.
type CallEvent struct {
	TS             string `json:"ts"`
	Phase          string `json:"phase"`
	CallID         string `json:"call_id"`
	RequestID      string `json:"request_id,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
	TurnID         string `json:"turn_id,omitempty"`
	Round          int    `json:"round,omitempty"`
	Attempt        int    `json:"attempt"`
	Provider       string `json:"provider,omitempty"`
	Model          string `json:"model,omitempty"`
	Endpoint       string `json:"endpoint,omitempty"`

	// Request shape: enough to spot a runaway prompt without storing it.
	MessageCount int           `json:"message_count,omitempty"`
	Roles        []string      `json:"roles,omitempty"`
	PromptChars  int           `json:"prompt_chars,omitempty"`
	ToolCount    int           `json:"tool_count,omitempty"`
	ToolNames    []string      `json:"tool_names,omitempty"`
	Temperature  float64       `json:"temperature,omitempty"`
	MaxTokens    int           `json:"max_tokens,omitempty"`
	RequestBytes int           `json:"request_bytes,omitempty"`
	Messages     []CallMessage `json:"messages,omitempty"`
	RequestBody  string        `json:"request_body,omitempty"`

	// Response.
	LatencyMS         int64          `json:"latency_ms,omitempty"`
	HTTPStatus        int            `json:"http_status,omitempty"`
	OK                bool           `json:"ok,omitempty"`
	ErrorKind         string         `json:"error_kind,omitempty"`
	Error             string         `json:"error,omitempty"`
	FinishReason      string         `json:"finish_reason,omitempty"`
	ContentChars      int            `json:"content_chars,omitempty"`
	ContentPreview    string         `json:"content_preview,omitempty"`
	ToolCalls         []CallToolCall `json:"tool_calls,omitempty"`
	ResponseBytes     int            `json:"response_bytes,omitempty"`
	Usage             *CallUsage     `json:"usage,omitempty"`
	ProviderRequestID string         `json:"provider_request_id,omitempty"`
	UpstreamBody      string         `json:"upstream_body,omitempty"`
	ResponseBody      string         `json:"response_body,omitempty"`
}

// Error kinds. They answer "whose fault is it" without reading the message.
const (
	errKindRequest     = "request"     // the request could not even be built
	errKindNetwork     = "network"     // dial/TLS/EOF: transport never completed
	errKindTimeout     = "timeout"     // ctx or client deadline
	errKindCanceled    = "canceled"    // caller went away
	errKindHTTPStatus  = "http_status" // non-2xx from the provider
	errKindDecode      = "decode"      // body was not the expected JSON
	errKindProvider    = "provider"    // provider returned {"error": ...}
	errKindEmptyChoice = "empty_choices"
)

// Recorder writes structured model-call records. It is safe for concurrent use
// and never returns an error to the caller: a broken log must not break chat.
type Recorder struct {
	level     CallLogLevel
	bodyChars int
	logger    *slog.Logger
	writer    io.Writer

	mu      sync.Mutex
	dropped int
}

// RecorderOptions configures a Recorder. Writer is the JSONL sink (one event per
// line); it may be nil, in which case only slog records are emitted.
type RecorderOptions struct {
	Level     CallLogLevel
	BodyChars int
	Logger    *slog.Logger
	Writer    io.Writer
}

// DefaultBodyChars caps the request/response bodies kept at CallLogFull.
const DefaultBodyChars = 4000

// NewRecorder builds a recorder. A nil logger falls back to slog.Default.
func NewRecorder(opts RecorderOptions) *Recorder {
	if opts.BodyChars <= 0 {
		opts.BodyChars = DefaultBodyChars
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Recorder{
		level:     opts.Level,
		bodyChars: opts.BodyChars,
		logger:    opts.Logger,
		writer:    opts.Writer,
	}
}

// Level reports the configured verbosity (CallLogOff for a nil recorder).
func (r *Recorder) Level() CallLogLevel {
	if r == nil {
		return CallLogOff
	}
	return r.level
}

func (r *Recorder) enabled() bool { return r != nil && r.level > CallLogOff }
func (r *Recorder) detail() bool  { return r != nil && r.level >= CallLogDetail }
func (r *Recorder) full() bool    { return r != nil && r.level >= CallLogFull }

func (r *Recorder) cap(s string) string {
	return truncate(maskPII(s), r.bodyChars)
}

// start emits the pre-flight line: what is about to be sent.
func (r *Recorder) start(e CallEvent) {
	if !r.enabled() {
		return
	}
	e.TS = time.Now().UTC().Format(time.RFC3339Nano)
	e.Phase = "start"
	if r.detail() {
		// Per-message detail is already attached by the adapter; here we only
		// strip the bodies that belong to the "end" line.
		e.ResponseBody, e.ContentPreview, e.UpstreamBody = "", "", ""
	}
	if !r.full() {
		e.RequestBody = ""
	}
	r.emit(e, slog.LevelDebug)
}

// end emits the post-flight line: what came back, or why nothing did.
//
// The request side belongs to the "start" line, so messages and the request body
// are dropped here; the two lines are joined by call_id and never duplicate text.
func (r *Recorder) end(e CallEvent) {
	if !r.enabled() {
		return
	}
	e.TS = time.Now().UTC().Format(time.RFC3339Nano)
	e.Phase = "end"
	e.Messages, e.RequestBody = nil, ""
	if !r.detail() {
		e.ToolCalls, e.ContentPreview = nil, ""
	}
	if !r.full() {
		e.ResponseBody = ""
	}
	if e.UpstreamBody != "" {
		e.UpstreamBody = truncate(maskPII(e.UpstreamBody), 1024)
	}
	level := slog.LevelInfo
	if !e.OK {
		level = slog.LevelWarn
	}
	r.emit(e, level)
}

func (r *Recorder) emit(e CallEvent, level slog.Level) {
	if r.writer != nil {
		if line, err := json.Marshal(e); err == nil {
			r.mu.Lock()
			if _, err := r.writer.Write(append(line, '\n')); err != nil {
				r.dropped++
			}
			r.mu.Unlock()
		}
	}
	r.logSlog(e, level)
}

// logSlog mirrors the event into the process log with a flat, greppable shape.
func (r *Recorder) logSlog(e CallEvent, level slog.Level) {
	attrs := []any{
		"phase", e.Phase,
		"call_id", e.CallID,
		"conversation_id", e.ConversationID,
		"request_id", e.RequestID,
		"round", e.Round,
		"attempt", e.Attempt,
		"model", e.Model,
	}
	if e.Phase == "start" {
		attrs = append(attrs,
			"message_count", e.MessageCount,
			"prompt_chars", e.PromptChars,
			"tool_count", e.ToolCount,
			"request_bytes", e.RequestBytes)
		r.log(level, "llm call", attrs...)
		return
	}

	attrs = append(attrs,
		"latency_ms", e.LatencyMS,
		"finish_reason", e.FinishReason,
		"content_chars", e.ContentChars)
	if e.HTTPStatus != 0 {
		attrs = append(attrs, "http_status", e.HTTPStatus)
	}
	if names := toolCallNames(e.ToolCalls); names != "" {
		attrs = append(attrs, "tool_calls", names)
	}
	if e.Usage != nil {
		attrs = append(attrs, "prompt_tokens", e.Usage.PromptTokens,
			"completion_tokens", e.Usage.CompletionTokens,
			"total_tokens", e.Usage.TotalTokens)
	}
	if e.ResponseBytes != 0 {
		attrs = append(attrs, "response_bytes", e.ResponseBytes)
	}
	if e.ProviderRequestID != "" {
		attrs = append(attrs, "provider_request_id", e.ProviderRequestID)
	}
	if e.Error != "" {
		attrs = append(attrs, "error_kind", e.ErrorKind, "error", e.Error)
	}
	if e.UpstreamBody != "" {
		attrs = append(attrs, "upstream_body", e.UpstreamBody)
	}
	msg := "llm call ok"
	if !e.OK {
		msg = "llm call failed"
	}
	r.log(level, msg, attrs...)
}

// log routes to the matching slog method: slog.Logger.Log needs a context, and
// a model call has nothing useful to hang off it.
func (r *Recorder) log(level slog.Level, msg string, attrs ...any) {
	switch {
	case level >= slog.LevelError:
		r.logger.Error(msg, attrs...)
	case level >= slog.LevelWarn:
		r.logger.Warn(msg, attrs...)
	case level >= slog.LevelInfo:
		r.logger.Info(msg, attrs...)
	default:
		r.logger.Debug(msg, attrs...)
	}
}

// Dropped reports how many JSONL lines could not be written (a full disk, a
// closed file). Exposed for the shutdown summary.
func (r *Recorder) Dropped() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dropped
}

func toolCallNames(calls []CallToolCall) string {
	if len(calls) == 0 {
		return ""
	}
	names := make([]string, 0, len(calls))
	for _, c := range calls {
		names = append(names, c.Name)
	}
	return strings.Join(names, ",")
}

// summarizeMessages builds the per-message projection used at CallLogDetail.
// Content is truncated to a preview: enough to recognise the message, not enough
// to accidentally become a second copy of the transcript.
func summarizeMessages(msgs []Message, previewChars int) []CallMessage {
	if previewChars <= 0 {
		previewChars = 200
	}
	out := make([]CallMessage, 0, len(msgs))
	for _, m := range msgs {
		cm := CallMessage{
			Role:       m.Role,
			ToolName:   m.ToolName,
			ToolCallID: m.ToolCallID,
			Chars:      len([]rune(m.Content)),
			Preview:    truncate(maskPII(m.Content), previewChars),
		}
		for _, tc := range m.ToolCalls {
			cm.Calls = append(cm.Calls, tc.Name+"("+truncate(maskPII(tc.Arguments), previewChars)+")")
		}
		out = append(out, cm)
	}
	return out
}

func rolesOf(msgs []Message) []string {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m.Role)
	}
	return out
}

func toolNamesOf(tools []map[string]any) []string {
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		fn, ok := t["function"].(map[string]any)
		if !ok {
			continue
		}
		if name, ok := fn["name"].(string); ok && name != "" {
			out = append(out, name)
		}
	}
	return out
}

var (
	maskIDCard = regexp.MustCompile(`\b\d{17}[\dXx]\b`)
	maskPhone  = regexp.MustCompile(`\b1[3-9]\d{9}\b`)
	maskEmail  = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
)

// maskPII removes the three identifiers that must never reach a log file. Order
// numbers (ORD-10003) and amounts are deliberately kept: they are what makes a
// log line actionable.
func maskPII(s string) string {
	if s == "" {
		return s
	}
	s = maskIDCard.ReplaceAllStringFunc(s, func(v string) string {
		return v[:6] + strings.Repeat("*", 8) + v[len(v)-4:]
	})
	s = maskPhone.ReplaceAllStringFunc(s, domain.MaskPhone)
	s = maskEmail.ReplaceAllStringFunc(s, func(v string) string {
		at := strings.Index(v, "@")
		return v[:1] + "***" + v[at:]
	})
	return s
}
