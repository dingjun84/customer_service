// Package ai isolates the LLM provider behind a narrow interface and implements
// the tool-calling conversation loop.
//
// Nothing here touches the database directly: tools are executed through the
// registry, which calls application services.
package ai

import (
	"context"
)

// Role values mirror the OpenAI chat roles.
const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// ToolCall is one model-requested function call.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Message is a provider-neutral chat message.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolName   string     `json:"name,omitempty"`
}

// ModelRequest is one completion request.
type ModelRequest struct {
	Messages    []Message
	Tools       []map[string]any
	Temperature float64
	MaxTokens   int
	// Meta is correlation context for the call log. Adapters must never send it
	// to the provider.
	Meta CallMeta
}

// ModelResponse is one completion result.
type ModelResponse struct {
	Content      string
	ToolCalls    []ToolCall
	FinishReason string
	PromptChars  int
	// Reason is the model's own one-line justification for this round: why it is
	// calling these tools, or why it is answering directly. It is a demo-grade
	// explainability note, parsed out of Content by NormalizeResponse unless an
	// adapter already knows it. Never sent back to the provider.
	Reason string
	// Usage is provider-reported token accounting; nil when the provider omits it.
	Usage *CallUsage
}

// ChatModel is the only LLM dependency of the business code.
type ChatModel interface {
	Respond(ctx context.Context, req ModelRequest) (ModelResponse, error)
}
