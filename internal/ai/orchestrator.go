package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/example/ecommerce-support/internal/domain"
	"github.com/example/ecommerce-support/internal/repository"
	"github.com/example/ecommerce-support/internal/tools"
)

// Limits are the hard caps of one user turn (spec §3.2).
type Limits struct {
	MaxRounds    int
	MaxToolCalls int
}

// TurnInput is one user turn to process.
type TurnInput struct {
	Conversation *domain.Conversation
	CustomerID   string
	RequestID    string
	History      []domain.Message
}

// ToolTrace is a per-call summary returned to the API caller.
type ToolTrace struct {
	ToolCallID string `json:"tool_call_id"`
	ToolName   string `json:"tool_name"`
	Status     string `json:"status"`
	LatencyMS  int64  `json:"latency_ms"`
	ErrorCode  string `json:"error_code,omitempty"`
}

// TurnResult is the outcome of one assistant turn.
type TurnResult struct {
	Assistant      *domain.Message
	ToolTraces     []ToolTrace
	PendingActions []domain.PendingAction
	Rounds         int
	ToolCalls      int
	Degraded       bool
}

// Orchestrator runs the model/tool loop. It owns no business rules: every effect
// goes through the tool registry and then through application services.
type Orchestrator struct {
	model     ChatModel
	registry  *tools.Registry
	store     repository.Store
	assembler *Assembler
	limits    Limits
	logger    *slog.Logger
}

// NewOrchestrator wires the loop. A nil model means "LLM disabled": Run returns a
// clear llm_unavailable error instead of pretending to answer.
func NewOrchestrator(model ChatModel, registry *tools.Registry, store repository.Store,
	assembler *Assembler, limits Limits, logger *slog.Logger) *Orchestrator {
	if limits.MaxRounds <= 0 {
		limits.MaxRounds = 5
	}
	if limits.MaxToolCalls <= 0 {
		limits.MaxToolCalls = 8
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Orchestrator{
		model: model, registry: registry, store: store,
		assembler: assembler, limits: limits, logger: logger,
	}
}

const (
	safeLimitAnswer = "抱歉，这个问题需要更多步骤才能处理完。你可以补充一下具体需求，或者我帮你转接人工客服继续处理。"
	safeEmptyAnswer = "抱歉，我暂时没能生成有效回复。请换个说法再试一次，或者我帮你转接人工客服。"
)

// Run executes the tool-calling loop and persists every message it produces.
func (o *Orchestrator) Run(ctx context.Context, in TurnInput) (*TurnResult, error) {
	if o.model == nil {
		return nil, domain.NewError(domain.CodeLLMUnavailable, "智能客服模型未启用，暂时无法进行对话")
	}
	if in.Conversation == nil {
		return nil, domain.NewError(domain.CodeInternal, "会话上下文缺失")
	}

	started := time.Now()
	// turn_id ties every model call and tool trace of this user message together,
	// so one line in the log is enough to find the whole turn.
	turnID := "turn_" + domain.NewID()
	meta := CallMeta{
		ConversationID: in.Conversation.ID,
		RequestID:      in.RequestID,
		TurnID:         turnID,
	}

	msgs := o.assembler.Build(in.Conversation, in.History)
	result := &TurnResult{ToolTraces: []ToolTrace{}}
	usedCalls := 0

	defer func() {
		o.logger.Info("llm turn finished",
			"conversation_id", meta.ConversationID,
			"request_id", meta.RequestID,
			"turn_id", turnID,
			"rounds", result.Rounds,
			"tool_calls", result.ToolCalls,
			"degraded", result.Degraded,
			"prompt_chars", PromptChars(msgs),
			"message_count", len(msgs),
			"latency_ms", time.Since(started).Milliseconds())
	}()

	for round := 1; round <= o.limits.MaxRounds; round++ {
		result.Rounds = round
		callMeta := meta
		callMeta.Round = round
		resp, err := o.model.Respond(ctx, ModelRequest{
			Messages:    msgs,
			Tools:       o.registry.OpenAITools(),
			Temperature: 0.2,
			Meta:        callMeta,
		})
		if err != nil {
			o.logger.Warn("llm round failed",
				"conversation_id", in.Conversation.ID,
				"request_id", in.RequestID,
				"turn_id", turnID,
				"round", round,
				"error", err.Error())
			return nil, domain.NewError(domain.CodeLLMUnavailable, "智能客服暂时不可用，请稍后重试或转人工").Wrap(err)
		}

		if len(resp.ToolCalls) == 0 {
			return o.finish(ctx, in, result, resp.Content, false)
		}

		calls := o.assignCallIDs(resp.ToolCalls)
		o.persistAssistantCalls(ctx, in, resp.Content, calls)

		if usedCalls+len(calls) > o.limits.MaxToolCalls {
			for _, call := range calls {
				limited := tools.Result{
					Error: &tools.Failure{Code: string(domain.CodeToolExecutionFailed), Message: "单轮工具调用次数已达上限，未执行"},
				}
				o.persistToolMessage(ctx, in, call, limited)
			}
			return o.finish(ctx, in, result, safeLimitAnswer, true)
		}

		assistantMsg := Message{Role: RoleAssistant, Content: resp.Content}
		for _, call := range calls {
			assistantMsg.ToolCalls = append(assistantMsg.ToolCalls,
				ToolCall{ID: call.ID, Name: call.Name, Arguments: call.Arguments})
		}
		msgs = append(msgs, assistantMsg)

		for _, call := range calls {
			usedCalls++
			execResult, trace := o.executeTool(ctx, in, call)
			result.ToolTraces = append(result.ToolTraces, trace)
			result.ToolCalls = usedCalls
			o.persistToolMessage(ctx, in, call, execResult)
			msgs = append(msgs, Message{
				Role:       RoleTool,
				Content:    execResult.JSON(),
				ToolCallID: call.ID,
				ToolName:   call.Name,
			})
		}
	}

	// Rounds exhausted: return a safe, non-committal answer instead of looping.
	return o.finish(ctx, in, result, safeLimitAnswer, true)
}

// assignCallIDs gives every tool call a unique server-side ID. Provider IDs are not
// trusted: they repeat across turns and the trace table requires uniqueness.
func (o *Orchestrator) assignCallIDs(calls []ToolCall) []ToolCall {
	out := make([]ToolCall, 0, len(calls))
	for _, c := range calls {
		out = append(out, ToolCall{
			ID:        "call_" + domain.NewID(),
			Name:      c.Name,
			Arguments: c.Arguments,
		})
	}
	return out
}

func (o *Orchestrator) finish(ctx context.Context, in TurnInput, result *TurnResult, text string, degraded bool) (*TurnResult, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		text = safeEmptyAnswer
		degraded = true
	}
	msg := &domain.Message{
		ID:             domain.NewID(),
		ConversationID: in.Conversation.ID,
		Role:           domain.RoleAssistant,
		Content:        text,
		MetadataJSON:   "{}",
		CreatedAt:      time.Now().UTC().Truncate(time.Second),
	}
	if degraded {
		msg.MetadataJSON = `{"degraded":true}`
	}
	if err := o.store.AppendMessage(ctx, msg); err != nil {
		return nil, domain.AsAppError(err)
	}
	result.Assistant = msg
	result.Degraded = degraded

	pending, err := o.store.ListPendingActions(ctx, in.Conversation.ID, domain.ActionPending, 5)
	if err != nil {
		o.logger.Warn("list pending actions failed", "conversation_id", in.Conversation.ID, "error", err.Error())
	} else {
		result.PendingActions = pending
	}
	return result, nil
}

// persistAssistantCalls stores one assistant row per tool call. Rows are merged
// back into a single assistant message when rebuilding context.
func (o *Orchestrator) persistAssistantCalls(ctx context.Context, in TurnInput, content string, calls []ToolCall) {
	now := time.Now().UTC().Truncate(time.Second)
	for i, call := range calls {
		text := ""
		if i == 0 {
			text = content
		}
		msg := &domain.Message{
			ID:                domain.NewID(),
			ConversationID:    in.Conversation.ID,
			Role:              domain.RoleAssistant,
			Content:           text,
			ToolName:          call.Name,
			ToolCallID:        call.ID,
			ToolArgumentsJSON: truncate(call.Arguments, 4000),
			MetadataJSON:      "{}",
			CreatedAt:         now,
		}
		if err := o.store.AppendMessage(ctx, msg); err != nil {
			o.logger.Error("persist assistant tool call failed",
				"conversation_id", in.Conversation.ID, "error", err.Error())
		}
	}
}

func (o *Orchestrator) persistToolMessage(ctx context.Context, in TurnInput, call ToolCall, result tools.Result) {
	raw := result.JSON()
	msg := &domain.Message{
		ID:             domain.NewID(),
		ConversationID: in.Conversation.ID,
		Role:           domain.RoleTool,
		Content:        summarizeResult(result),
		ToolName:       call.Name,
		ToolCallID:     call.ID,
		ToolResultJSON: truncate(raw, 8000),
		MetadataJSON:   "{}",
		CreatedAt:      time.Now().UTC().Truncate(time.Second),
	}
	if err := o.store.AppendMessage(ctx, msg); err != nil {
		o.logger.Error("persist tool message failed",
			"conversation_id", in.Conversation.ID, "tool", call.Name, "error", err.Error())
	}
}

// executeTool records the tool_execution trace around the registry call.
func (o *Orchestrator) executeTool(ctx context.Context, in TurnInput, call ToolCall) (tools.Result, ToolTrace) {
	now := time.Now().UTC()
	execID := domain.NewID()
	argsJSON := truncate(sanitizeArgs(call.Name, call.Arguments), 4000)

	record := &domain.ToolExecution{
		ID:             execID,
		ConversationID: in.Conversation.ID,
		ToolCallID:     call.ID,
		ToolName:       call.Name,
		InputJSON:      argsJSON,
		Status:         domain.ToolStarted,
		CreatedAt:      now.Truncate(time.Second),
	}
	if err := o.store.CreateToolExecution(ctx, record); err != nil {
		o.logger.Error("create tool execution failed", "tool", call.Name, "error", err.Error())
	}

	started := time.Now()
	result := o.registry.Execute(ctx, tools.ExecutionContext{
		CustomerID:     in.CustomerID,
		ConversationID: in.Conversation.ID,
		RequestID:      in.RequestID,
	}, call.Name, json.RawMessage(call.Arguments))
	latency := time.Since(started).Milliseconds()

	status, code, safeMessage := domain.ToolSucceeded, "", ""
	if !result.OK {
		code = result.Error.Code
		safeMessage = result.Error.Message
		if code == tools.UnknownToolCode || code == string(domain.CodeValidation) {
			status = domain.ToolRejected
		} else {
			status = domain.ToolFailed
		}
	}
	output := ""
	if result.OK {
		output = truncate(result.JSON(), 8000)
	}
	if err := o.store.FinishToolExecution(ctx, execID, status, output, code, safeMessage, latency, time.Now().UTC()); err != nil {
		o.logger.Error("finish tool execution failed", "tool", call.Name, "error", err.Error())
	}

	return result, ToolTrace{
		ToolCallID: call.ID,
		ToolName:   call.Name,
		Status:     status,
		LatencyMS:  latency,
		ErrorCode:  code,
	}
}

// sanitizeArgs masks personal data before persisting tool input traces.
func sanitizeArgs(toolName, raw string) string {
	if raw == "" {
		return "{}"
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return "{}"
	}
	if changes, ok := parsed["changes"].(map[string]any); ok {
		if phone, ok := changes["receiver_phone"].(string); ok && phone != "" {
			changes["receiver_phone"] = domain.MaskPhone(phone)
		}
		for _, key := range []string{"shipping_address", "order_remark", "details", "description"} {
			if v, ok := changes[key].(string); ok {
				changes[key] = truncate(v, 40)
			}
		}
	}
	for _, key := range []string{"details", "description"} {
		if v, ok := parsed[key].(string); ok {
			parsed[key] = truncate(v, 200)
		}
	}
	encoded, err := json.Marshal(parsed)
	if err != nil {
		return "{}"
	}
	return string(encoded)
}

func summarizeResult(result tools.Result) string {
	if !result.OK {
		if result.Error != nil {
			return fmt.Sprintf("tool error: %s", result.Error.Code)
		}
		return "tool error"
	}
	raw, err := json.Marshal(result.Data)
	if err != nil {
		return "ok"
	}
	return truncate(string(raw), 400)
}

func truncate(s string, max int) string {
	if max <= 0 {
		return s
	}
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max]) + "…"
}
