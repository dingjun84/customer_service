package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/example/ecommerce-support/internal/domain"
)

// OpenAICompatible calls any /chat/completions endpoint that supports tool calling
// (OpenAI, DeepSeek, Moonshot, vLLM, llama.cpp server, ...).
type OpenAICompatible struct {
	BaseURL    string
	APIKey     string
	Model      string
	HTTPClient *http.Client
	// MaxRetries applies to network errors and 429/5xx only.
	MaxRetries int
	// Provider labels the call log ("openai", "deepseek", ...). Optional.
	Provider string
	// Recorder, when non-nil, writes one structured start/end pair per HTTP
	// attempt. A nil recorder disables model-call logging.
	Recorder *Recorder
}

// NewOpenAICompatible builds an adapter with a bounded timeout.
func NewOpenAICompatible(baseURL, apiKey, model string, timeout time.Duration) *OpenAICompatible {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &OpenAICompatible{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		APIKey:     apiKey,
		Model:      model,
		HTTPClient: &http.Client{Timeout: timeout},
		MaxRetries: 2,
	}
}

type chatRequest struct {
	Model       string           `json:"model"`
	Messages    []chatMessage    `json:"messages"`
	Tools       []map[string]any `json:"tools,omitempty"`
	ToolChoice  string           `json:"tool_choice,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
}

type chatMessage struct {
	Role       string         `json:"role"`
	Content    string         `json:"content"`
	ToolCalls  []chatToolCall `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
	Name       string         `json:"name,omitempty"`
}

type chatToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content   string         `json:"content"`
			ToolCalls []chatToolCall `json:"tool_calls"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	// Usage is optional: some OpenAI-compatible servers omit it (or send null).
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error"`
}

// Respond performs one chat completion with bounded retries. Every attempt is
// reported to the recorder separately: retries are the most common reason a
// "slow" turn is actually two fast calls plus a backoff.
func (c *OpenAICompatible) Respond(ctx context.Context, req ModelRequest) (ModelResponse, error) {
	if c.BaseURL == "" || c.Model == "" {
		return ModelResponse{}, fmt.Errorf("llm: base url or model is not configured")
	}

	body := chatRequest{
		Model:       c.Model,
		Messages:    make([]chatMessage, 0, len(req.Messages)),
		Tools:       req.Tools,
		ToolChoice:  "auto",
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
	}
	for _, m := range req.Messages {
		msg := chatMessage{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID, Name: m.ToolName}
		for _, tc := range m.ToolCalls {
			call := chatToolCall{ID: tc.ID, Type: "function"}
			call.Function.Name = tc.Name
			call.Function.Arguments = tc.Arguments
			msg.ToolCalls = append(msg.ToolCalls, call)
		}
		body.Messages = append(body.Messages, msg)
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return ModelResponse{}, fmt.Errorf("llm: encode request: %w", err)
	}

	base := c.baseEvent(req, payload)

	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ModelResponse{}, ctx.Err()
			case <-time.After(time.Duration(attempt) * 400 * time.Millisecond):
			}
		}
		resp, retryable, err := c.do(ctx, payload, base, attempt)
		if err == nil {
			return resp, nil
		}
		lastErr = err
		if !retryable {
			break
		}
	}
	return ModelResponse{}, lastErr
}

func (c *OpenAICompatible) endpoint() string {
	return c.BaseURL + "/chat/completions"
}

func (c *OpenAICompatible) provider() string {
	if c.Provider != "" {
		return c.Provider
	}
	return "openai_compatible"
}

// previewChars caps per-message previews recorded at CallLogDetail.
const previewChars = 200

// baseEvent describes the request once; each attempt clones it and adds its own
// identity, so a retry is visible as a second line with the same request shape.
func (c *OpenAICompatible) baseEvent(req ModelRequest, payload []byte) CallEvent {
	rec := c.Recorder
	if !rec.enabled() {
		return CallEvent{}
	}
	e := CallEvent{
		RequestID:      req.Meta.RequestID,
		ConversationID: req.Meta.ConversationID,
		TurnID:         req.Meta.TurnID,
		Round:          req.Meta.Round,
		Provider:       c.provider(),
		Model:          c.Model,
		Endpoint:       c.endpoint(),
		MessageCount:   len(req.Messages),
		Roles:          rolesOf(req.Messages),
		PromptChars:    PromptChars(req.Messages),
		ToolCount:      len(req.Tools),
		Temperature:    req.Temperature,
		MaxTokens:      req.MaxTokens,
		RequestBytes:   len(payload),
	}
	if rec.detail() {
		e.Messages = summarizeMessages(req.Messages, previewChars)
		e.ToolNames = toolNamesOf(req.Tools)
	}
	if rec.full() {
		e.RequestBody = rec.cap(string(payload))
	}
	return e
}

// do performs a single attempt and records both halves of it. The deferred
// block is the only place that knows how the attempt ended, which keeps every
// return path logged — including the ones that return early.
func (c *OpenAICompatible) do(ctx context.Context, payload []byte, base CallEvent, attempt int) (out ModelResponse, retryable bool, err error) {
	callID := "llm_" + domain.NewID()
	started := time.Now()

	ev := base
	ev.CallID = callID
	ev.Attempt = attempt
	if ev.Endpoint == "" {
		ev.Endpoint = c.endpoint()
	}
	if ev.Provider == "" {
		ev.Provider = c.provider()
	}
	if ev.Model == "" {
		ev.Model = c.Model
	}

	if c.Recorder.enabled() {
		c.Recorder.start(ev)
	}

	var (
		httpStatus    int
		respBytes     int
		respText      string
		upstream      string
		finishReason  string
		providerReqID string
		errKind       string
	)

	defer func() {
		if !c.Recorder.enabled() {
			return
		}
		end := ev
		end.LatencyMS = time.Since(started).Milliseconds()
		end.HTTPStatus = httpStatus
		end.ResponseBytes = respBytes
		end.FinishReason = finishReason
		end.ProviderRequestID = providerReqID
		end.ContentChars = len([]rune(out.Content))
		end.Usage = out.Usage
		if err != nil {
			// The raw upstream text is the only place a provider's real complaint
			// lives; successful bodies are already summarised by the fields above.
			end.UpstreamBody = upstream
		}
		if c.Recorder.full() {
			end.ResponseBody = c.Recorder.cap(respText)
		}
		if c.Recorder.detail() {
			end.ContentPreview = truncate(maskPII(out.Content), previewChars)
			for _, tc := range out.ToolCalls {
				end.ToolCalls = append(end.ToolCalls, CallToolCall{
					Name:      tc.Name,
					Arguments: truncate(maskPII(tc.Arguments), previewChars),
				})
			}
		}
		if err != nil {
			end.Error = err.Error()
			end.ErrorKind = errKind
		} else {
			end.OK = true
		}
		c.Recorder.end(end)
	}()

	httpReq, reqErr := http.NewRequestWithContext(ctx, http.MethodPost, ev.Endpoint, bytes.NewReader(payload))
	if reqErr != nil {
		errKind = errKindRequest
		return ModelResponse{}, false, fmt.Errorf("llm: build request: %w", reqErr)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, doErr := c.HTTPClient.Do(httpReq)
	if doErr != nil {
		// Timeouts and connection errors are worth one retry; the key is never logged.
		switch {
		case errors.Is(doErr, context.DeadlineExceeded):
			errKind = errKindTimeout
		case errors.Is(doErr, context.Canceled):
			errKind = errKindCanceled
		default:
			errKind = errKindNetwork
		}
		return ModelResponse{}, true, fmt.Errorf("llm: request failed: %w", doErr)
	}
	defer resp.Body.Close()
	httpStatus = resp.StatusCode
	providerReqID = firstHeader(resp.Header, "x-request-id", "openai-request-id", "x-amzn-requestid")

	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if readErr != nil {
		errKind = errKindNetwork
		return ModelResponse{}, true, fmt.Errorf("llm: read response: %w", readErr)
	}
	respBytes = len(raw)
	// Keep the upstream body for the log even on success: a 200 that carries an
	// unexpected shape is exactly what a "model answered nonsense" report needs.
	upstream = string(raw)
	respText = upstream

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		errKind = errKindHTTPStatus
		return ModelResponse{}, true, fmt.Errorf("llm: upstream status %d", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errKind = errKindHTTPStatus
		return ModelResponse{}, false, fmt.Errorf("llm: upstream status %d", resp.StatusCode)
	}

	var parsed chatResponse
	if err = json.Unmarshal(raw, &parsed); err != nil {
		errKind = errKindDecode
		return ModelResponse{}, false, fmt.Errorf("llm: decode response: %w", err)
	}
	if parsed.Error != nil {
		errKind = errKindProvider
		return ModelResponse{}, false, fmt.Errorf("llm: provider error: %s: %s", parsed.Error.Type, parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		errKind = errKindEmptyChoice
		return ModelResponse{}, false, fmt.Errorf("llm: empty choices")
	}

	choice := parsed.Choices[0]
	finishReason = choice.FinishReason
	out = ModelResponse{
		Content:      choice.Message.Content,
		FinishReason: choice.FinishReason,
	}
	if parsed.Usage != nil {
		out.Usage = &CallUsage{
			PromptTokens:     parsed.Usage.PromptTokens,
			CompletionTokens: parsed.Usage.CompletionTokens,
			TotalTokens:      parsed.Usage.TotalTokens,
		}
	}
	for _, tc := range choice.Message.ToolCalls {
		if tc.Function.Name == "" {
			continue
		}
		out.ToolCalls = append(out.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	return out, false, nil
}

// firstHeader returns the first non-empty header among names. Providers put their
// support reference in different places; having it turns "add a ticket" into a
// lookup.
func firstHeader(h http.Header, names ...string) string {
	for _, n := range names {
		if v := strings.TrimSpace(h.Get(n)); v != "" {
			return v
		}
	}
	return ""
}
