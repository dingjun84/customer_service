package tools

import (
	"context"
	"encoding/json"
	"time"

	"github.com/example/ecommerce-support/internal/domain"
	"github.com/example/ecommerce-support/internal/service"
)

// UnknownToolCode is returned when the model hallucinates a tool name.
const UnknownToolCode = "unknown_tool"

// ExecutionContext carries the authenticated identity of one tool call. The model
// can never set these values: they come from middleware + conversation state.
type ExecutionContext struct {
	CustomerID     string
	ConversationID string
	RequestID      string
}

// Failure is the safe error shape returned to the model.
type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Result is the uniform tool return value (spec §9.3).
type Result struct {
	OK        bool     `json:"ok"`
	Data      any      `json:"data,omitempty"`
	Error     *Failure `json:"error"`
	RequestID string   `json:"request_id,omitempty"`
}

// JSON serialises the result for persistence in messages/tool traces.
func (r Result) JSON() string {
	raw, err := json.Marshal(r)
	if err != nil {
		return `{"ok":false,"error":{"code":"internal_error","message":"结果序列化失败"}}`
	}
	return string(raw)
}

// Handler executes one validated tool call.
type Handler func(ctx context.Context, ec ExecutionContext, args Args) (any, error)

// Options configures the registry.
type Options struct {
	// Timeout bounds a single tool execution; 0 falls back to 10s.
	Timeout time.Duration
}

// Registry is the only path from the model into application services.
type Registry struct {
	svc      *service.Services
	defs     []Definition
	byName   map[string]Definition
	handlers map[string]Handler
	timeout  time.Duration
}

// New builds the registry with every MVP tool registered.
func New(svc *service.Services, opts Options) *Registry {
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	r := &Registry{
		svc:      svc,
		defs:     Definitions(),
		byName:   map[string]Definition{},
		handlers: map[string]Handler{},
		timeout:  timeout,
	}
	for _, d := range r.defs {
		r.byName[d.Name] = d
	}
	r.registerHandlers()
	return r
}

// Definitions returns the catalog (copy-safe for read-only use).
func (r *Registry) Definitions() []Definition { return r.defs }

// OpenAITools renders every definition in OpenAI tool-calling format.
func (r *Registry) OpenAITools() []map[string]any {
	out := make([]map[string]any, 0, len(r.defs))
	for _, d := range r.defs {
		out = append(out, d.OpenAITool())
	}
	return out
}

// Has reports whether a tool name is registered.
func (r *Registry) Has(name string) bool {
	_, ok := r.handlers[name]
	return ok
}

// Execute validates and dispatches one tool call. It never panics: unexpected
// panics become tool_execution_failed results.
func (r *Registry) Execute(ctx context.Context, ec ExecutionContext, name string, rawArgs json.RawMessage) (result Result) {
	result.RequestID = ec.RequestID

	handler, ok := r.handlers[name]
	if !ok {
		result.Error = &Failure{Code: UnknownToolCode, Message: "不支持的工具：" + name}
		return result
	}
	def := r.byName[name]

	args, err := DecodeArgs(rawArgs)
	if err != nil {
		result.Error = &Failure{Code: string(domain.CodeValidation), Message: err.Error()}
		return result
	}
	if err := Validate(def.Parameters, map[string]any(args)); err != nil {
		result.Error = &Failure{Code: string(domain.CodeValidation), Message: err.Error()}
		return result
	}

	callCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	defer func() {
		if rec := recover(); rec != nil {
			result.OK = false
			result.Data = nil
			result.Error = &Failure{Code: string(domain.CodeToolExecutionFailed), Message: "工具执行异常，请稍后重试或转人工"}
		}
	}()

	data, err := handler(callCtx, ec, args)
	if err != nil {
		appErr := domain.AsAppError(err)
		result.Error = &Failure{Code: string(appErr.Code), Message: appErr.Message}
		return result
	}
	result.OK = true
	result.Error = nil
	result.Data = data
	return result
}

func (r *Registry) register(name string, h Handler) { r.handlers[name] = h }
