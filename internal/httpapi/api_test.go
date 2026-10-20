package httpapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/example/ecommerce-support/internal/ai"
	"github.com/example/ecommerce-support/internal/config"
	"github.com/example/ecommerce-support/internal/conversation"
	"github.com/example/ecommerce-support/internal/database"
	"github.com/example/ecommerce-support/internal/httpapi"
	"github.com/example/ecommerce-support/internal/repository/sqlite"
	"github.com/example/ecommerce-support/internal/seed"
	"github.com/example/ecommerce-support/internal/service"
	"github.com/example/ecommerce-support/internal/tools"
)

type testServer struct {
	t       *testing.T
	ctx     context.Context
	handler http.Handler
	db      *sql.DB
	store   *sqlite.Store
	model   ai.ChatModel
	now     time.Time
}

func newTestServer(t *testing.T, model ai.ChatModel, mutate func(*config.Config)) *testServer {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "api.db"))
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

	ts := &testServer{t: t, ctx: ctx, db: db, store: sqlite.New(db), model: model,
		now: time.Now().UTC().Truncate(time.Second)}

	services := service.New(ts.store, service.Options{
		PendingActionTTL: 10 * time.Minute,
		Now:              func() time.Time { return ts.now },
	})
	registry := tools.New(services, tools.Options{Timeout: 5 * time.Second})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	orchestrator := ai.NewOrchestrator(model, registry, ts.store, ai.NewAssembler(6, 40000),
		ai.Limits{MaxRounds: 5, MaxToolCalls: 8}, logger)
	convSvc := conversation.New(ts.store, orchestrator, services.Actions, conversation.Options{Logger: logger})

	cfg := config.Config{
		AppEnv: "test", HTTPAddr: ":0", DBPath: "test.db", LogLevel: "error",
		DevAPIEnabled: true, LLMEnabled: model != nil,
		LLMTimeout: 5 * time.Second, LLMMaxToolRounds: 5, LLMMaxToolCalls: 8,
		ConversationRecentTurns: 6, ConversationMaxChars: 40000,
		PendingActionTTL: 10 * time.Minute, ToolTimeout: 5 * time.Second,
		AfterSalesWindowDays: 7, PriceProtectionDays: 15,
	}
	if mutate != nil {
		mutate(&cfg)
	}
	ts.handler = httpapi.NewServer(cfg, db, ts.store, services, convSvc, nil, logger).Handler()
	return ts
}

func (ts *testServer) do(method, path, customer string, body any) (*httptest.ResponseRecorder, map[string]any) {
	ts.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			ts.t.Fatalf("encode body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if customer != "" {
		req.Header.Set("X-Demo-Customer-ID", customer)
	}
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)

	decoded := map[string]any{}
	if rec.Body.Len() > 0 {
		_ = json.Unmarshal(rec.Body.Bytes(), &decoded)
	}
	return rec, decoded
}

// dataObject returns the "data" object of an envelope.
func dataObject(t *testing.T, body map[string]any) map[string]any {
	t.Helper()
	data, ok := body["data"].(map[string]any)
	if !ok {
		t.Fatalf("response has no data object: %v", body)
	}
	return data
}

func dataList(t *testing.T, body map[string]any) []any {
	t.Helper()
	list, ok := body["data"].([]any)
	if !ok {
		t.Fatalf("response data is not a list: %v", body)
	}
	return list
}

// orderInState finds a real order in the requested state, so the HTTP tests do
// not depend on which row happens to be ORD-1000x.
func (ts *testServer) orderInState(customerNo, status string) string {
	ts.t.Helper()
	var no string
	err := ts.db.QueryRow(
		`SELECT o.order_no FROM orders o JOIN customers c ON c.id = o.customer_id
		  WHERE c.customer_no = ? AND o.order_status = ? ORDER BY o.created_at DESC LIMIT 1`,
		customerNo, status).Scan(&no)
	if err != nil {
		ts.t.Fatalf("no %s order for %s: %v", status, customerNo, err)
	}
	return no
}

func (ts *testServer) orderPaidMinor(orderNo string) int64 {
	ts.t.Helper()
	var paid int64
	if err := ts.db.QueryRow(`SELECT paid_amount_minor FROM orders WHERE order_no = ?`, orderNo).Scan(&paid); err != nil {
		ts.t.Fatalf("read paid amount of %s: %v", orderNo, err)
	}
	return paid
}

func errorCode(t *testing.T, body map[string]any) string {
	t.Helper()
	errObj, ok := body["error"].(map[string]any)
	if !ok {
		t.Fatalf("response has no error object: %v", body)
	}
	code, _ := errObj["code"].(string)
	return code
}

func (ts *testServer) newConversation(customer string) string {
	ts.t.Helper()
	rec, body := ts.do(http.MethodPost, "/api/v1/conversations", customer, map[string]any{"channel": "web_demo"})
	if rec.Code != http.StatusCreated {
		ts.t.Fatalf("create conversation status = %d body = %v", rec.Code, body)
	}
	return dataObject(ts.t, body)["conversation_id"].(string)
}

func TestHealthAndReadiness(t *testing.T) {
	ts := newTestServer(t, nil, nil)

	rec, body := ts.do(http.MethodGet, "/healthz", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz = %d", rec.Code)
	}
	if dataObject(t, body)["status"] != "ok" {
		t.Fatalf("healthz body = %v", body)
	}

	rec, body = ts.do(http.MethodGet, "/readyz", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("readyz = %d", rec.Code)
	}
	// Compare against the migration set actually embedded in this build rather
	// than a hardcoded version, so adding a migration does not break readiness.
	wantVersion, err := database.SchemaVersion(ts.ctx, ts.db)
	if err != nil {
		t.Fatalf("schema version: %v", err)
	}
	if dataObject(t, body)["schema_version"] != float64(wantVersion) {
		t.Fatalf("readyz body = %v, want schema_version %d", body, wantVersion)
	}
}

func TestIdentityRequired(t *testing.T) {
	ts := newTestServer(t, nil, nil)
	rec, body := ts.do(http.MethodGet, "/api/v1/orders", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
	if errorCode(t, body) != "unauthorized" {
		t.Fatalf("code = %s", errorCode(t, body))
	}

	rec, _ = ts.do(http.MethodGet, "/api/v1/orders", "CUST-9999", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unknown customer status = %d, want 401", rec.Code)
	}
}

func TestCrossCustomerOrderAccessForbidden(t *testing.T) {
	ts := newTestServer(t, nil, nil)
	rec, body := ts.do(http.MethodGet, "/api/v1/orders/"+ts.orderInState("CUST-1001", "shipped"), "CUST-1002", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403", rec.Code)
	}
	if errorCode(t, body) != "forbidden" {
		t.Fatalf("code = %s", errorCode(t, body))
	}
	// The body must not leak any order field.
	if _, ok := body["data"]; ok {
		t.Fatal("forbidden response must not contain data")
	}
}

func TestOrderDetailAndNotFound(t *testing.T) {
	ts := newTestServer(t, nil, nil)
	shipped := ts.orderInState("CUST-1001", "shipped")
	rec, body := ts.do(http.MethodGet, "/api/v1/orders/"+shipped, "CUST-1001", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %v", rec.Code, body)
	}
	order := dataObject(t, body)
	if order["order_status"] != "shipped" {
		t.Fatalf("status = %v", order["order_status"])
	}
	items := order["items"].([]any)
	if len(items) == 0 {
		t.Fatal("order must expose its lines")
	}

	rec, body = ts.do(http.MethodGet, "/api/v1/orders/ORD-40400", "CUST-1001", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing order status = %d", rec.Code)
	}
	if errorCode(t, body) != "not_found" {
		t.Fatalf("code = %s", errorCode(t, body))
	}
}

func TestLimitValidation(t *testing.T) {
	ts := newTestServer(t, nil, nil)
	rec, body := ts.do(http.MethodGet, "/api/v1/orders?limit=500", "CUST-1001", nil)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	if errorCode(t, body) != "validation_error" {
		t.Fatalf("code = %s", errorCode(t, body))
	}
}

func TestChatWithToolCall(t *testing.T) {
	model := ai.NewFake(
		ai.ToolCallResponse("c1", tools.ToolGetLogisticsStatus, `{"order_no":"ORD-10003"}`),
		ai.TextResponse("你的包裹已到达深圳南山营业点，正在派送中。"),
	)
	ts := newTestServer(t, model, nil)
	convID := ts.newConversation("CUST-1001")

	rec, body := ts.do(http.MethodPost, "/api/v1/conversations/"+convID+"/messages", "CUST-1001",
		map[string]any{"message": "我的订单 ORD-10003 到哪了？", "client_message_id": "cm-1"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %v", rec.Code, body)
	}
	turn := dataObject(t, body)
	assistant := turn["assistant_message"].(map[string]any)
	if assistant["content"] == "" {
		t.Fatal("assistant message is empty")
	}
	traces := turn["tool_executions"].([]any)
	if len(traces) != 1 {
		t.Fatalf("tool executions = %d", len(traces))
	}
	trace := traces[0].(map[string]any)
	if trace["status"] != "succeeded" {
		t.Fatalf("trace status = %v", trace["status"])
	}

	// Duplicate client_message_id replays the stored answer without a new model call.
	rec, body = ts.do(http.MethodPost, "/api/v1/conversations/"+convID+"/messages", "CUST-1001",
		map[string]any{"message": "我的订单 ORD-10003 到哪了？", "client_message_id": "cm-1"})
	if rec.Code != http.StatusOK {
		t.Fatalf("replay status = %d", rec.Code)
	}
	if dataObject(t, body)["idempotent"] != true {
		t.Fatal("replay should be marked idempotent")
	}
	if calls := model.Calls(); calls != 2 {
		t.Fatalf("model calls = %d, want 2 (no call on replay)", calls)
	}
}

func TestModifyOrderConfirmFlow(t *testing.T) {
	model := ai.NewFake(
		ai.ToolCallResponse("c1", tools.ToolModifyOrder,
			`{"order_no":"ORD-10002","changes":{"shipping_address":"南山区新地址 66 号"}}`),
		ai.TextResponse("已为你生成地址修改预览，请点击确认后生效。"),
	)
	ts := newTestServer(t, model, nil)
	convID := ts.newConversation("CUST-1001")

	rec, body := ts.do(http.MethodPost, "/api/v1/conversations/"+convID+"/messages", "CUST-1001",
		map[string]any{"message": "帮我把 ORD-10002 的地址改成南山区新地址 66 号"})
	if rec.Code != http.StatusOK {
		t.Fatalf("send status = %d body = %v", rec.Code, body)
	}
	turn := dataObject(t, body)
	pending := turn["pending_actions"].([]any)
	if len(pending) != 1 {
		t.Fatalf("pending actions = %d, want 1 (%v)", len(pending), turn)
	}
	action := pending[0].(map[string]any)
	actionID := action["action_id"].(string)
	if action["summary"] == "" {
		t.Fatal("pending action must carry a human readable summary")
	}

	// The proposal must not have changed the order yet.
	_, orderBody := ts.do(http.MethodGet, "/api/v1/orders/ORD-10002", "CUST-1001", nil)
	receiver := dataObject(t, orderBody)["receiver"].(map[string]any)
	if receiver["address"] == "南山区新地址 66 号" {
		t.Fatal("address changed before confirmation")
	}

	rec, body = ts.do(http.MethodPost,
		"/api/v1/conversations/"+convID+"/actions/"+actionID+"/confirm", "CUST-1001", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm status = %d body = %v", rec.Code, body)
	}
	result := dataObject(t, body)
	if result["status"] != "executed" {
		t.Fatalf("action status = %v", result["status"])
	}

	_, orderBody = ts.do(http.MethodGet, "/api/v1/orders/ORD-10002", "CUST-1001", nil)
	receiver = dataObject(t, orderBody)["receiver"].(map[string]any)
	if receiver["address"] != "南山区新地址 66 号" {
		t.Fatalf("address = %v, want updated value", receiver["address"])
	}

	// Repeat confirm is idempotent, not a second write.
	rec, body = ts.do(http.MethodPost,
		"/api/v1/conversations/"+convID+"/actions/"+actionID+"/confirm", "CUST-1001", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("second confirm status = %d", rec.Code)
	}
	if dataObject(t, body)["idempotent"] != true {
		t.Fatal("second confirm should be idempotent")
	}

	// Audit trail exists for the order.
	var audits int
	if err := ts.db.QueryRow(`SELECT COUNT(1) FROM action_audit_logs WHERE target_type = 'order'`).Scan(&audits); err != nil {
		t.Fatalf("count audits: %v", err)
	}
	if audits == 0 {
		t.Fatal("expected an audit record for the modification")
	}
}

func TestCancelActionFlowCreatesRefund(t *testing.T) {
	probe := newTestServer(t, nil, nil)
	orderNo := probe.orderInState("CUST-1001", "paid")
	paid := probe.orderPaidMinor(orderNo)

	model := ai.NewFake(
		ai.ToolCallResponse("c1", tools.ToolRequestCancellation,
			fmt.Sprintf(`{"order_no":%q,"reason":"不想要了"}`, orderNo)),
		ai.TextResponse("已生成取消申请，请确认后执行。"),
	)
	ts := newTestServer(t, model, nil)
	convID := ts.newConversation("CUST-1001")

	_, body := ts.do(http.MethodPost, "/api/v1/conversations/"+convID+"/messages", "CUST-1001",
		map[string]any{"message": "帮我取消 " + orderNo})
	pending := dataObject(t, body)["pending_actions"].([]any)
	if len(pending) != 1 {
		t.Fatalf("pending = %d", len(pending))
	}
	actionID := pending[0].(map[string]any)["action_id"].(string)

	rec, body := ts.do(http.MethodPost,
		"/api/v1/conversations/"+convID+"/actions/"+actionID+"/confirm", "CUST-1001", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("confirm = %d body = %v", rec.Code, body)
	}
	result := dataObject(t, body)["result"].(map[string]any)
	if result["order_status"] != "cancelled" {
		t.Fatalf("result = %v", result)
	}
	if result["refund_no"] == nil {
		t.Fatalf("expected refund record, got %v", result)
	}
	if result["refund_amount_minor"] != float64(paid) {
		t.Fatalf("refund amount = %v, want %d", result["refund_amount_minor"], paid)
	}

	_, orderBody := ts.do(http.MethodGet, "/api/v1/orders/"+orderNo, "CUST-1001", nil)
	if dataObject(t, orderBody)["order_status"] != "cancelled" {
		t.Fatal("order was not cancelled")
	}
}

func TestRejectActionLeavesOrderUntouched(t *testing.T) {
	model := ai.NewFake(
		ai.ToolCallResponse("c1", tools.ToolRequestCancellation, `{"order_no":"ORD-10002"}`),
		ai.TextResponse("请确认是否取消。"),
	)
	ts := newTestServer(t, model, nil)
	convID := ts.newConversation("CUST-1001")

	_, body := ts.do(http.MethodPost, "/api/v1/conversations/"+convID+"/messages", "CUST-1001",
		map[string]any{"message": "取消 ORD-10002"})
	pending := dataObject(t, body)["pending_actions"].([]any)
	actionID := pending[0].(map[string]any)["action_id"].(string)

	rec, body := ts.do(http.MethodPost,
		"/api/v1/conversations/"+convID+"/actions/"+actionID+"/reject", "CUST-1001", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("reject = %d body = %v", rec.Code, body)
	}
	if dataObject(t, body)["status"] != "rejected" {
		t.Fatalf("status = %v", dataObject(t, body)["status"])
	}
	_, orderBody := ts.do(http.MethodGet, "/api/v1/orders/ORD-10002", "CUST-1001", nil)
	if dataObject(t, orderBody)["order_status"] == "cancelled" {
		t.Fatal("rejected action must not change the order")
	}

	// Confirming a rejected action is a conflict.
	rec, body = ts.do(http.MethodPost,
		"/api/v1/conversations/"+convID+"/actions/"+actionID+"/confirm", "CUST-1001", nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("confirm-after-reject status = %d body = %v", rec.Code, body)
	}
}

func TestExpiredActionReturns410(t *testing.T) {
	model := ai.NewFake(
		ai.ToolCallResponse("c1", tools.ToolRequestCancellation, `{"order_no":"ORD-10002"}`),
		ai.TextResponse("请确认。"),
	)
	ts := newTestServer(t, model, nil)
	convID := ts.newConversation("CUST-1001")

	_, body := ts.do(http.MethodPost, "/api/v1/conversations/"+convID+"/messages", "CUST-1001",
		map[string]any{"message": "取消 ORD-10002"})
	actionID := dataObject(t, body)["pending_actions"].([]any)[0].(map[string]any)["action_id"].(string)

	// Move the service clock past the TTL.
	ts.now = ts.now.Add(11 * time.Minute)

	rec, body := ts.do(http.MethodPost,
		"/api/v1/conversations/"+convID+"/actions/"+actionID+"/confirm", "CUST-1001", nil)
	if rec.Code != http.StatusGone {
		t.Fatalf("status = %d, want 410 (body=%v)", rec.Code, body)
	}
	if errorCode(t, body) != "action_expired" {
		t.Fatalf("code = %s", errorCode(t, body))
	}
	_, orderBody := ts.do(http.MethodGet, "/api/v1/orders/ORD-10002", "CUST-1001", nil)
	if dataObject(t, orderBody)["order_status"] == "cancelled" {
		t.Fatal("expired action must not change the order")
	}
}

func TestCrossCustomerActionAccessForbidden(t *testing.T) {
	model := ai.NewFake(
		ai.ToolCallResponse("c1", tools.ToolRequestCancellation, `{"order_no":"ORD-10002"}`),
		ai.TextResponse("请确认。"),
	)
	ts := newTestServer(t, model, nil)
	convID := ts.newConversation("CUST-1001")
	_, body := ts.do(http.MethodPost, "/api/v1/conversations/"+convID+"/messages", "CUST-1001",
		map[string]any{"message": "取消 ORD-10002"})
	actionID := dataObject(t, body)["pending_actions"].([]any)[0].(map[string]any)["action_id"].(string)

	// Customer 2 knows the IDs but must not be able to confirm.
	rec, body := ts.do(http.MethodPost,
		"/api/v1/conversations/"+convID+"/actions/"+actionID+"/confirm", "CUST-1002", nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403 (body=%v)", rec.Code, body)
	}
}

func TestChatWithoutLLMReturns503(t *testing.T) {
	ts := newTestServer(t, nil, nil)
	convID := ts.newConversation("CUST-1001")

	rec, body := ts.do(http.MethodPost, "/api/v1/conversations/"+convID+"/messages", "CUST-1001",
		map[string]any{"message": "你好"})
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if errorCode(t, body) != "llm_unavailable" {
		t.Fatalf("code = %s", errorCode(t, body))
	}
	// The user message must still be persisted.
	_, msgs := ts.do(http.MethodGet, "/api/v1/conversations/"+convID+"/messages", "CUST-1001", nil)
	list := dataList(t, msgs)
	if len(list) != 1 {
		t.Fatalf("messages = %d, want 1", len(list))
	}
}

func TestDevAPIHiddenWhenDisabled(t *testing.T) {
	ts := newTestServer(t, nil, func(c *config.Config) { c.DevAPIEnabled = false })
	convID := ts.newConversation("CUST-1001")
	rec, _ := ts.do(http.MethodGet, "/api/v1/dev/conversations/"+convID+"/trace", "CUST-1001", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestDevAPITraceShowsToolCalls(t *testing.T) {
	model := ai.NewFake(
		ai.ToolCallResponse("c1", tools.ToolGetOrderDetail, `{"order_no":"ORD-10003"}`),
		ai.TextResponse("订单在运输途中。"),
	)
	ts := newTestServer(t, model, nil)
	convID := ts.newConversation("CUST-1001")
	ts.do(http.MethodPost, "/api/v1/conversations/"+convID+"/messages", "CUST-1001",
		map[string]any{"message": "查一下 ORD-10003"})

	rec, body := ts.do(http.MethodGet, "/api/v1/dev/conversations/"+convID+"/trace", "CUST-1001", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("trace status = %d body = %v", rec.Code, body)
	}
	trace := dataObject(t, body)["tool_executions"].([]any)
	if len(trace) != 1 {
		t.Fatalf("trace entries = %d", len(trace))
	}
	entry := trace[0].(map[string]any)
	if entry["tool_name"] != tools.ToolGetOrderDetail {
		t.Fatalf("tool = %v", entry["tool_name"])
	}

	// A different customer cannot read this trace.
	rec, _ = ts.do(http.MethodGet, "/api/v1/dev/conversations/"+convID+"/trace", "CUST-1002", nil)
	if rec.Code != http.StatusForbidden && rec.Code != http.StatusNotFound {
		t.Fatalf("cross-customer trace status = %d", rec.Code)
	}
}

func TestUnknownRouteReturnsJSON404(t *testing.T) {
	ts := newTestServer(t, nil, nil)
	rec, body := ts.do(http.MethodGet, "/api/v1/nope", "CUST-1001", nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if errorCode(t, body) != "not_found" {
		t.Fatalf("code = %s", errorCode(t, body))
	}
}

func TestRequestBodyLimit(t *testing.T) {
	ts := newTestServer(t, nil, nil)
	convID := ts.newConversation("CUST-1001")
	huge := make([]byte, 1<<21)
	for i := range huge {
		huge[i] = 'a'
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/"+convID+"/messages",
		bytes.NewReader(append(append([]byte(`{"message":"`), huge...), []byte(`"}`)...)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Demo-Customer-ID", "CUST-1001")
	rec := httptest.NewRecorder()
	ts.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

// TestDevCustomerDirectoryMatchesDataset guards the identity switcher: the
// browser console builds its customer list from this endpoint, so it must list
// every seeded customer with counts that match the real tables.
func TestDevCustomerDirectoryMatchesDataset(t *testing.T) {
	ts := newTestServer(t, nil, nil)

	rec, body := ts.do(http.MethodGet, "/api/v1/dev/customers", "CUST-1001", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %v", rec.Code, body)
	}
	list := dataList(t, body)
	if len(list) == 0 {
		t.Fatal("customer directory is empty")
	}

	var customers int
	if err := ts.db.QueryRow(`SELECT COUNT(*) FROM customers`).Scan(&customers); err != nil {
		t.Fatalf("count customers: %v", err)
	}
	if len(list) != customers {
		t.Fatalf("directory has %d customers, table has %d", len(list), customers)
	}

	meta := body["meta"].(map[string]any)
	var orders int
	if err := ts.db.QueryRow(`SELECT COUNT(*) FROM orders`).Scan(&orders); err != nil {
		t.Fatalf("count orders: %v", err)
	}
	if int(meta["order_total"].(float64)) != orders {
		t.Fatalf("meta.order_total = %v, want %d", meta["order_total"], orders)
	}

	for _, raw := range list {
		entry := raw.(map[string]any)
		no := entry["customer_no"].(string)
		var actual int
		err := ts.db.QueryRow(
			`SELECT COUNT(*) FROM orders o JOIN customers c ON c.id = o.customer_id WHERE c.customer_no = ?`,
			no).Scan(&actual)
		if err != nil {
			t.Fatalf("count orders of %s: %v", no, err)
		}
		if int(entry["order_count"].(float64)) != actual {
			t.Fatalf("%s order_count = %v, want %d", no, entry["order_count"], actual)
		}
	}
}

// TestDevCustomerDirectoryHiddenInProd makes sure the console falls back to its
// hard-coded pair instead of leaking a dev-only listing in production. Either
// the route is absent (404) or the demo identity header is rejected (401); both
// mean no directory is published outside dev/test.
func TestDevCustomerDirectoryHiddenInProd(t *testing.T) {
	ts := newTestServer(t, nil, func(c *config.Config) {
		c.AppEnv = "prod"
		c.DevAPIEnabled = false
	})
	rec, _ := ts.do(http.MethodGet, "/api/v1/dev/customers", "CUST-1001", nil)
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 404 or 401", rec.Code)
	}
}
