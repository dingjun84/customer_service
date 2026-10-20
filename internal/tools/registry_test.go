package tools_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/example/ecommerce-support/internal/database"
	"github.com/example/ecommerce-support/internal/domain"
	"github.com/example/ecommerce-support/internal/repository/sqlite"
	"github.com/example/ecommerce-support/internal/seed"
	"github.com/example/ecommerce-support/internal/service"
	"github.com/example/ecommerce-support/internal/tools"
)

type fixture struct {
	registry     *tools.Registry
	ctx          context.Context
	db           *sql.DB
	cust1        string
	cust2        string
	conversation string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "tools.db"))
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
	svc := service.New(store, service.Options{
		PendingActionTTL: 10 * time.Minute,
		Now:              time.Now,
	})
	registry := tools.New(svc, tools.Options{Timeout: 5 * time.Second})

	cust1, err := store.GetCustomerByNo(ctx, "CUST-1001")
	if err != nil {
		t.Fatalf("customer 1: %v", err)
	}
	cust2, err := store.GetCustomerByNo(ctx, "CUST-1002")
	if err != nil {
		t.Fatalf("customer 2: %v", err)
	}
	convID := "conv-test"
	err = store.CreateConversation(ctx, &domain.Conversation{
		ID: convID, ConversationNo: "CONV-TEST", CustomerID: cust1.ID, Status: domain.ConversationOpen,
		Channel: "test", CreatedAt: time.Now().UTC().Truncate(time.Second), UpdatedAt: time.Now().UTC().Truncate(time.Second),
	})
	if err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	return &fixture{registry: registry, ctx: ctx, db: db, cust1: cust1.ID, cust2: cust2.ID, conversation: convID}
}

func (f *fixture) exec(customerID, toolName, args string) tools.Result {
	return f.registry.Execute(f.ctx, tools.ExecutionContext{
		CustomerID: customerID, ConversationID: f.conversation, RequestID: "req-test",
	}, toolName, json.RawMessage(args))
}

// orderNo picks a real order in the requested state instead of hard-coding an
// ORD-… number: the dataset may grow, the test should not care.
func (f *fixture) orderNo(t *testing.T, customerID, status string) string {
	t.Helper()
	var no string
	err := f.db.QueryRowContext(f.ctx,
		`SELECT order_no FROM orders WHERE customer_id = ? AND order_status = ? ORDER BY created_at DESC LIMIT 1`,
		customerID, status).Scan(&no)
	if err != nil {
		t.Fatalf("no %s order for customer: %v", status, err)
	}
	return no
}

// oldestDelivered returns the completed order whose delivery is furthest in the
// past, so the after-sales window test does not depend on the calendar.
func (f *fixture) oldestDelivered(t *testing.T, customerID string) string {
	t.Helper()
	rows, err := f.db.QueryContext(f.ctx,
		`SELECT order_no FROM orders WHERE customer_id = ? AND order_status = 'completed'
		   AND delivered_at IS NOT NULL ORDER BY delivered_at ASC LIMIT 1`, customerID)
	if err != nil {
		t.Fatalf("query delivered orders: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatalf("no completed order for customer")
	}
	var no string
	if err := rows.Scan(&no); err != nil {
		t.Fatalf("scan order: %v", err)
	}
	return no
}

func TestUnknownToolIsRejected(t *testing.T) {
	f := newFixture(t)
	res := f.exec(f.cust1, "drop_all_orders", `{}`)
	if res.OK {
		t.Fatal("unknown tool must not execute")
	}
	if res.Error == nil || res.Error.Code != tools.UnknownToolCode {
		t.Fatalf("error = %+v, want unknown_tool", res.Error)
	}
}

func TestInvalidJSONArgumentsRejected(t *testing.T) {
	f := newFixture(t)
	res := f.exec(f.cust1, tools.ToolGetOrderDetail, `{"order_no": `)
	if res.OK || res.Error.Code != "validation_error" {
		t.Fatalf("res = %+v, want validation_error", res)
	}
}

func TestUnknownFieldRejected(t *testing.T) {
	f := newFixture(t)
	// customer_id is not part of the schema: the model cannot choose a customer.
	res := f.exec(f.cust1, tools.ToolGetOrderDetail, `{"order_no":"ORD-10003","customer_id":"CUST-1002"}`)
	if res.OK {
		t.Fatal("unknown field must be rejected")
	}
	if res.Error.Code != "validation_error" {
		t.Fatalf("code = %s", res.Error.Code)
	}
}

func TestEnumViolationRejected(t *testing.T) {
	f := newFixture(t)
	res := f.exec(f.cust1, tools.ToolCheckEligibility, `{"action_type":"delete_order","order_no":"ORD-10003"}`)
	if res.OK || res.Error.Code != "validation_error" {
		t.Fatalf("res = %+v, want validation_error", res)
	}
}

func TestListMyOrdersOnlyReturnsOwnOrders(t *testing.T) {
	f := newFixture(t)
	res := f.exec(f.cust2, tools.ToolListMyOrders, `{"limit":20}`)
	if !res.OK {
		t.Fatalf("tool failed: %+v", res)
	}
	data := res.Data.(map[string]any)
	orders := data["orders"].([]map[string]any)
	if len(orders) == 0 {
		t.Fatal("customer 2 should see their own orders")
	}
	owned := map[string]bool{}
	rows, err := f.db.QueryContext(f.ctx, `SELECT order_no FROM orders WHERE customer_id = ?`, f.cust2)
	if err != nil {
		t.Fatalf("query orders: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var no string
		if err := rows.Scan(&no); err != nil {
			t.Fatalf("scan: %v", err)
		}
		owned[no] = true
	}
	for _, o := range orders {
		if !owned[o["order_no"].(string)] {
			t.Fatalf("leaked another customer's order: %v", o["order_no"])
		}
	}
}

func TestGetOrderDetailRejectsForeignOrder(t *testing.T) {
	f := newFixture(t)
	res := f.exec(f.cust2, tools.ToolGetOrderDetail, `{"order_no":"ORD-10003"}`)
	if res.OK {
		t.Fatal("foreign order must not be readable")
	}
	if res.Error.Code != "forbidden" {
		t.Fatalf("code = %s, want forbidden", res.Error.Code)
	}
}

func TestGetLogisticsStatusReturnsRealEvents(t *testing.T) {
	f := newFixture(t)
	res := f.exec(f.cust1, tools.ToolGetLogisticsStatus, fmt.Sprintf(`{"order_no":%q}`, f.orderNo(t, f.cust1, "shipped")))
	if !res.OK {
		t.Fatalf("tool failed: %+v", res)
	}
	data := res.Data.(map[string]any)
	shipments := data["shipments"].([]map[string]any)
	if len(shipments) == 0 {
		t.Fatal("shipped order must expose at least one parcel")
	}
	events := shipments[0]["events"].([]map[string]any)
	if len(events) < 3 {
		t.Fatalf("events = %d, want at least 3", len(events))
	}
}

func TestCancelShippedOrderIsNotEligible(t *testing.T) {
	f := newFixture(t)
	res := f.exec(f.cust1, tools.ToolCheckEligibility,
		fmt.Sprintf(`{"action_type":"cancel_order","order_no":%q}`, f.orderNo(t, f.cust1, "shipped")))
	if !res.OK {
		t.Fatalf("tool failed: %+v", res)
	}
	result, ok := res.Data.(*service.EligibilityResult)
	if !ok {
		t.Fatalf("unexpected data type %T", res.Data)
	}
	if result.Eligible {
		t.Fatal("shipped order must not be cancellable")
	}
	if result.ReasonCode != "already_shipped" {
		t.Fatalf("reason = %s, want already_shipped", result.ReasonCode)
	}
}

func TestModifyOrderCreatesPendingActionOnly(t *testing.T) {
	f := newFixture(t)
	orderNo := f.orderNo(t, f.cust1, "paid")
	res := f.exec(f.cust1, tools.ToolModifyOrder,
		fmt.Sprintf(`{"order_no":%q,"changes":{"shipping_address":"新的收货地址 9 号"}}`, orderNo))
	if !res.OK {
		t.Fatalf("tool failed: %+v", res)
	}
	data := res.Data.(map[string]any)
	if data["action_id"] == nil {
		t.Fatalf("expected pending action, got %+v", data)
	}
	if data["requires_confirmation"] != true {
		t.Fatalf("requires_confirmation = %v", data["requires_confirmation"])
	}
	// Read back: the order must still carry the old address.
	detail := f.exec(f.cust1, tools.ToolGetOrderDetail, fmt.Sprintf(`{"order_no":%q}`, orderNo))
	receiver := detail.Data.(map[string]any)["receiver"].(map[string]any)
	if receiver["address"] == "新的收货地址 9 号" {
		t.Fatal("proposal must not mutate the order")
	}
}

func TestSubmitReturnOutsideWindowIsRejectedByTool(t *testing.T) {
	f := newFixture(t)
	res := f.exec(f.cust1, tools.ToolSubmitServiceRequest,
		fmt.Sprintf(`{"case_type":"return","order_no":%q,"quantity":1}`, f.oldestDelivered(t, f.cust1)))
	if !res.OK {
		t.Fatalf("tool failed: %+v", res)
	}
	data := res.Data.(map[string]any)
	if data["action_id"] != nil {
		t.Fatal("out-of-window return must not create a pending action")
	}
	eligibility, ok := data["eligibility"].(service.EligibilityResult)
	if !ok {
		t.Fatalf("unexpected eligibility type %T", data["eligibility"])
	}
	if eligibility.ReasonCode != "out_of_window" {
		t.Fatalf("reason = %s, want out_of_window", eligibility.ReasonCode)
	}
}
