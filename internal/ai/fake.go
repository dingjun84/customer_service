package ai

import (
	"context"
	"errors"
	"sync"
)

// FakeChatModel replays scripted responses. It is the deterministic model used by
// unit and end-to-end tests: no API key and no network access required.
type FakeChatModel struct {
	mu        sync.Mutex
	responses []ModelResponse
	errs      []error
	requests  []ModelRequest
	index     int
}

// NewFake builds a scripted model. Each Respond consumes the next response.
func NewFake(responses ...ModelResponse) *FakeChatModel {
	return &FakeChatModel{responses: responses}
}

// NewFakeWithErrors builds a model that returns errors for the given call indexes.
func NewFakeWithErrors(errs []error, responses ...ModelResponse) *FakeChatModel {
	return &FakeChatModel{responses: responses, errs: errs}
}

// Respond implements ChatModel.
func (f *FakeChatModel) Respond(_ context.Context, req ModelRequest) (ModelResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req)

	if f.index < len(f.errs) && f.errs[f.index] != nil {
		err := f.errs[f.index]
		f.index++
		return ModelResponse{}, err
	}
	if f.index >= len(f.responses) {
		return ModelResponse{}, errors.New("fake model: no scripted response left")
	}
	resp := f.responses[f.index]
	f.index++
	return resp, nil
}

// Reset replaces the script and clears the recorded requests. Tests use it when
// the tool arguments depend on data they only discover after seeding (for
// example the order number they picked from the database).
func (f *FakeChatModel) Reset(responses ...ModelResponse) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.responses = responses
	f.errs = nil
	f.requests = nil
	f.index = 0
}

// Requests returns the requests received so far.
func (f *FakeChatModel) Requests() []ModelRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]ModelRequest, len(f.requests))
	copy(out, f.requests)
	return out
}

// Calls returns how many completions were requested.
func (f *FakeChatModel) Calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

// ToolCallResponse is a convenience builder for a scripted tool call.
func ToolCallResponse(id, tool, args string) ModelResponse {
	return ModelResponse{
		ToolCalls:    []ToolCall{{ID: id, Name: tool, Arguments: args}},
		FinishReason: "tool_calls",
	}
}

// TextResponse is a convenience builder for a scripted final answer.
func TextResponse(text string) ModelResponse {
	return ModelResponse{Content: text, FinishReason: "stop"}
}
