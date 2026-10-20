package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
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
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// Respond performs one chat completion with bounded retries.
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

	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ModelResponse{}, ctx.Err()
			case <-time.After(time.Duration(attempt) * 400 * time.Millisecond):
			}
		}
		resp, retryable, err := c.do(ctx, payload)
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

func (c *OpenAICompatible) do(ctx context.Context, payload []byte) (ModelResponse, bool, error) {
	url := c.BaseURL + "/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return ModelResponse{}, false, fmt.Errorf("llm: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
	}

	resp, err := c.HTTPClient.Do(httpReq)
	if err != nil {
		// Timeouts and connection errors are worth one retry; the key is never logged.
		return ModelResponse{}, true, fmt.Errorf("llm: request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return ModelResponse{}, true, fmt.Errorf("llm: read response: %w", err)
	}
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		return ModelResponse{}, true, fmt.Errorf("llm: upstream status %d", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ModelResponse{}, false, fmt.Errorf("llm: upstream status %d", resp.StatusCode)
	}

	var parsed chatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return ModelResponse{}, false, fmt.Errorf("llm: decode response: %w", err)
	}
	if parsed.Error != nil {
		return ModelResponse{}, false, fmt.Errorf("llm: provider error: %s", parsed.Error.Type)
	}
	if len(parsed.Choices) == 0 {
		return ModelResponse{}, false, fmt.Errorf("llm: empty choices")
	}

	choice := parsed.Choices[0]
	out := ModelResponse{Content: choice.Message.Content, FinishReason: choice.FinishReason}
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
