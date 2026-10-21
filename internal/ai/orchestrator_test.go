package ai_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/example/ecommerce-support/internal/ai"
	"github.com/example/ecommerce-support/internal/database"
	"github.com/example/ecommerce-support/internal/domain"
	"github.com/example/ecommerce-support/internal/repository/sqlite"
	"github.com/example/ecommerce-support/internal/seed"
	"github.com/example/ecommerce-support/internal/service"
	"github.com/example/ecommerce-support/internal/tools"
)

type harness struct {
	t     *testing.T
	ctx   context.Context
	db    *sql.DB
	store *sqlite.Store
	reg   *tools.Registry
	conv  *domain.Conversation
	cust  string
}

func newHarness(t *testing.T, model ai.ChatModel, limits ai.Limits) (*harness, *ai.Orchestrator) {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "ai.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := database.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := seed.New(db, nil).Run(ctx, false); err != nil {
		t.Fatalf("seed: %v", err)
	}
	store := sqlite.New(db)
	svc := service.New(store, service.Options{PendingActionTTL: 10 * time.Minute, Now: time.Now})
	reg := tools.New(svc, tools.Options{Timeout: 5 * time.Second})

	cust, err := store.GetCustomerByNo(ctx, "CUST-1001")
	if err != nil {
		t.Fatalf("customer: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	conv := &domain.Conversation{
		ID: domain.NewID(), ConversationNo: "CONV-AI-TEST", CustomerID: cust.ID,
		Status: domain.ConversationOpen, Channel: "test", CreatedAt: now, UpdatedAt: now,
	}
	if err := store.CreateConversation(ctx, conv); err != nil {
		t.Fatalf("create conversation: %v", err)
	}
	if err := store.AppendMessage(ctx, &domain.Message{
		ID: domain.NewID(), ConversationID: conv.ID, Role: domain.RoleUser,
		Content: "我的包裹到哪了？", MetadataJSON: "{}", CreatedAt: now,
	}); err != nil {
		t.Fatalf("append user message: %v", err)
	}

	orch := ai.NewOrchestrator(model, reg, store, ai.NewAssembler(6, 40000), limits, nil)
	return &harness{t: t, ctx: ctx, db: db, store: store, reg: reg, conv: conv, cust: cust.ID}, orch
}

// shippedOrder returns a real in-transit order of the demo customer together
// with one of its parcel numbers, so the orchestrator test does not hard-code
// ORD-10003 or a shipment id that the dataset may renumber.
func (h *harness) shippedOrder() (orderNo, shipmentNo string) {
	h.t.Helper()
	err := h.db.QueryRowContext(h.ctx,
		`SELECT o.order_no, s.shipment_no FROM orders o
		   JOIN shipments s ON s.order_id = o.id
		  WHERE o.customer_id = ? AND o.order_status = 'shipped'
		  ORDER BY o.created_at DESC LIMIT 1`, h.cust).Scan(&orderNo, &shipmentNo)
	if err != nil {
		h.t.Fatalf("find shipped order: %v", err)
	}
	return orderNo, shipmentNo
}

func (h *harness) run(orch *ai.Orchestrator) *ai.TurnResult {
	h.t.Helper()
	history, err := h.store.ListRecentMessages(h.ctx, h.conv.ID, 20)
	if err != nil {
		h.t.Fatalf("history: %v", err)
	}
	result, err := orch.Run(h.ctx, ai.TurnInput{
		Conversation: h.conv, CustomerID: h.cust, RequestID: "req-ai", History: history,
	})
	if err != nil {
		h.t.Fatalf("run: %v", err)
	}
	return result
}

func TestOrchestratorDirectAnswer(t *testing.T) {
	model := ai.NewFake(ai.TextResponse("你好，有什么可以帮你？"))
	h, orch := newHarness(t, model, ai.Limits{MaxRounds: 5, MaxToolCalls: 8})

	result := h.run(orch)
	if result.Assistant == nil || result.Assistant.Content == "" {
		t.Fatal("expected an assistant message")
	}
	if len(result.ToolTraces) != 0 {
		t.Fatalf("tool traces = %d, want 0", len(result.ToolTraces))
	}
	if model.Calls() != 1 {
		t.Fatalf("model calls = %d, want 1", model.Calls())
	}
}

func TestOrchestratorSingleToolCallThenAnswer(t *testing.T) {
	model := ai.NewFake()
	h, orch := newHarness(t, model, ai.Limits{MaxRounds: 5, MaxToolCalls: 8})
	orderNo, shipmentNo := h.shippedOrder()
	model.Reset(
		ai.ToolCallResponse("call-1", tools.ToolGetLogisticsStatus, fmt.Sprintf(`{"order_no":%q}`, orderNo)),
		ai.TextResponse("你的包裹在深圳南山营业点，正在派送中。"),
	)

	result := h.run(orch)
	if len(result.ToolTraces) != 1 {
		t.Fatalf("tool traces = %d, want 1", len(result.ToolTraces))
	}
	if result.ToolTraces[0].Status != domain.ToolSucceeded {
		t.Fatalf("tool status = %s", result.ToolTraces[0].Status)
	}
	if !strings.Contains(result.Assistant.Content, "派送") {
		t.Fatalf("assistant = %q", result.Assistant.Content)
	}

	// The second model call must contain the tool result, so the answer can be grounded.
	requests := model.Requests()
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
	last := requests[1].Messages
	found := false
	for _, m := range last {
		if m.Role == ai.RoleTool && strings.Contains(m.Content, shipmentNo) {
			found = true
		}
	}
	if !found {
		t.Fatal("second request missing tool result")
	}
}

// The offline demo model must produce notes too, otherwise the feature cannot be
// shown without an API key.
func TestOrchestratorOfflineModelWritesNotes(t *testing.T) {
	h, orch := newHarness(t, ai.NewOfflineModel(), ai.Limits{MaxRounds: 5, MaxToolCalls: 8})

	result := h.run(orch)
	if len(result.Reasons) < 2 {
		t.Fatalf("reasons = %+v, want at least one tool round and one answer", result.Reasons)
	}
	for _, row := range result.Reasons {
		if row.Reason == "" {
			t.Fatalf("offline round %d (%s) has no note: %+v", row.Round, row.Kind, row)
		}
	}
	if result.Reasons[0].Kind != ai.ReasonKindToolCall || len(result.Reasons[0].Tools) == 0 {
		t.Fatalf("first round should be a tool round: %+v", result.Reasons[0])
	}
	if last := result.Reasons[len(result.Reasons)-1]; last.Kind != ai.ReasonKindAnswer {
		t.Fatalf("last round should be the answer: %+v", last)
	}
}

// The note the model writes before acting must never reach the customer, must be
// reported per round, and must survive a reload from the database.
func TestOrchestratorSeparatesReasonNotes(t *testing.T) {
	model := ai.NewFake()
	h, orch := newHarness(t, model, ai.Limits{MaxRounds: 5, MaxToolCalls: 8})
	orderNo, _ := h.shippedOrder()
	model.Reset(
		ai.ModelResponse{
			Content:   "<备注>物流是实时数据，先查轨迹</备注>",
			ToolCalls: []ai.ToolCall{{ID: "call-1", Name: tools.ToolGetLogisticsStatus, Arguments: fmt.Sprintf(`{"order_no":%q}`, orderNo)}},
		},
		ai.TextResponse("备注：轨迹已返回，直接回答\n\n你的包裹正在派送中。"),
	)

	result := h.run(orch)

	if len(result.Reasons) != 2 {
		t.Fatalf("reasons = %d, want 2 (%+v)", len(result.Reasons), result.Reasons)
	}
	if result.Reasons[0].Kind != ai.ReasonKindToolCall || result.Reasons[0].Tools[0] != tools.ToolGetLogisticsStatus {
		t.Fatalf("tool reason = %+v", result.Reasons[0])
	}
	if result.Reasons[0].Reason != "物流是实时数据，先查轨迹" {
		t.Fatalf("tool note = %q", result.Reasons[0].Reason)
	}
	if result.Reasons[1].Kind != ai.ReasonKindAnswer || result.Reasons[1].Reason != "轨迹已返回，直接回答" {
		t.Fatalf("answer reason = %+v", result.Reasons[1])
	}

	// The note is metadata, not content: the bubble must contain only the answer.
	if strings.Contains(result.Assistant.Content, "备注") {
		t.Fatalf("assistant content leaked the note: %q", result.Assistant.Content)
	}
	if !strings.Contains(result.Assistant.MetadataJSON, "轨迹已返回，直接回答") {
		t.Fatalf("metadata = %q", result.Assistant.MetadataJSON)
	}

	// A reloaded conversation rebuilds the same note from metadata.
	stored, err := h.store.ListRecentMessages(h.ctx, h.conv.ID, 20)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	var found bool
	for _, m := range stored {
		if m.Role == domain.RoleAssistant && strings.Contains(m.MetadataJSON, "reason") &&
			!strings.Contains(m.Content, "备注") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no assistant row kept the note in metadata: %+v", stored)
	}
}

func TestOrchestratorToolErrorDoesNotFakeSuccess(t *testing.T) {
	model := ai.NewFake(
		ai.ToolCallResponse("call-1", tools.ToolGetOrderDetail, `{"order_no":"ORD-99999"}`),
		ai.TextResponse("抱歉，没有查到这笔订单，请核对订单号。"),
	)
	h, orch := newHarness(t, model, ai.Limits{MaxRounds: 5, MaxToolCalls: 8})

	result := h.run(orch)
	if len(result.ToolTraces) != 1 {
		t.Fatalf("tool traces = %d, want 1", len(result.ToolTraces))
	}
	if result.ToolTraces[0].Status == domain.ToolSucceeded {
		t.Fatal("missing order must not be reported as success")
	}
	if result.ToolTraces[0].ErrorCode == "" {
		t.Fatal("expected an error code on the trace")
	}
}

func TestOrchestratorUnknownToolMarkedRejected(t *testing.T) {
	model := ai.NewFake(
		ai.ToolCallResponse("call-1", "issue_refund_now", `{}`),
		ai.TextResponse("抱歉，我无法执行这个操作。"),
	)
	h, orch := newHarness(t, model, ai.Limits{MaxRounds: 5, MaxToolCalls: 8})

	result := h.run(orch)
	if len(result.ToolTraces) != 1 {
		t.Fatalf("tool traces = %d", len(result.ToolTraces))
	}
	if result.ToolTraces[0].Status != domain.ToolRejected {
		t.Fatalf("status = %s, want rejected", result.ToolTraces[0].Status)
	}
}

func TestOrchestratorEnforcesToolCallLimit(t *testing.T) {
	model := ai.NewFake(ai.ModelResponse{
		ToolCalls: []ai.ToolCall{
			{ID: "c1", Name: tools.ToolGetOrderDetail, Arguments: `{"order_no":"ORD-10003"}`},
			{ID: "c2", Name: tools.ToolGetOrderDetail, Arguments: `{"order_no":"ORD-10004"}`},
			{ID: "c3", Name: tools.ToolGetOrderDetail, Arguments: `{"order_no":"ORD-10005"}`},
		},
		FinishReason: "tool_calls",
	})
	h, orch := newHarness(t, model, ai.Limits{MaxRounds: 5, MaxToolCalls: 2})

	result := h.run(orch)
	if !result.Degraded {
		t.Fatal("expected degraded result when tool call limit is exceeded")
	}
	if len(result.ToolTraces) != 0 {
		t.Fatalf("tool traces = %d, want 0 (no call executed)", len(result.ToolTraces))
	}
	if model.Calls() != 1 {
		t.Fatalf("model calls = %d, want 1", model.Calls())
	}
}

func TestOrchestratorEnforcesRoundLimit(t *testing.T) {
	// Always asks for a tool call: the loop must stop at MaxRounds.
	model := ai.NewFake(
		ai.ToolCallResponse("c1", tools.ToolGetOrderDetail, `{"order_no":"ORD-10003"}`),
		ai.ToolCallResponse("c2", tools.ToolGetOrderDetail, `{"order_no":"ORD-10003"}`),
		ai.ToolCallResponse("c3", tools.ToolGetOrderDetail, `{"order_no":"ORD-10003"}`),
	)
	h, orch := newHarness(t, model, ai.Limits{MaxRounds: 2, MaxToolCalls: 8})

	result := h.run(orch)
	if !result.Degraded {
		t.Fatal("expected degraded result after max rounds")
	}
	if model.Calls() != 2 {
		t.Fatalf("model calls = %d, want 2", model.Calls())
	}
}

func TestOrchestratorLLMDisabled(t *testing.T) {
	h, orch := newHarness(t, nil, ai.Limits{MaxRounds: 3, MaxToolCalls: 5})
	history, _ := h.store.ListRecentMessages(h.ctx, h.conv.ID, 5)
	_, err := orch.Run(h.ctx, ai.TurnInput{Conversation: h.conv, CustomerID: h.cust, History: history})
	if err == nil {
		t.Fatal("expected llm_unavailable error")
	}
	if appErr := domain.AsAppError(err); appErr.Code != domain.CodeLLMUnavailable {
		t.Fatalf("code = %s, want llm_unavailable", appErr.Code)
	}
}

func TestAssemblerKeepsOnlyRecentTurns(t *testing.T) {
	now := time.Now().UTC()
	conv := &domain.Conversation{ID: "c1", ActiveTaskStateJSON: `{"pending_action_id":"a1"}`}
	history := []domain.Message{}
	for i := 0; i < 20; i++ {
		role := domain.RoleUser
		if i%2 == 1 {
			role = domain.RoleAssistant
		}
		history = append(history, domain.Message{
			ID: domain.NewID(), ConversationID: "c1", Role: role,
			Content: strings.Repeat("x", 10), CreatedAt: now.Add(time.Duration(i) * time.Second),
		})
	}
	assembler := ai.NewAssembler(2, 40000)
	msgs := assembler.Build(conv, history)

	// system + task state + at most two turns (4 messages).
	if len(msgs) > 6 {
		t.Fatalf("messages = %d, want <= 6", len(msgs))
	}
	if msgs[0].Role != ai.RoleSystem || msgs[1].Role != ai.RoleSystem {
		t.Fatalf("expected system prompt and task state first, got %v", msgs[:2])
	}
	if !strings.Contains(msgs[1].Content, "pending_action_id") {
		t.Fatal("task state must be included")
	}
}
