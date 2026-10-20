package service_test

import (
	"testing"
	"time"

	"github.com/example/ecommerce-support/internal/domain"
	"github.com/example/ecommerce-support/internal/service"
)

func TestConfirmCancelUpdatesOrderCreatesRefundAndAudit(t *testing.T) {
	f := newFixture(t)
	paidAt := fixedNow.Add(-3 * time.Hour)
	orderID, _ := f.insertOrderWithItem("ORD-40001", orderOpts{status: domain.OrderPaid, paidMinor: 129900, paidAt: &paidAt})
	f.insertPayment(orderID, "PAY-40001", 129900)

	proposal, err := f.svc.Actions.Propose(f.ctx, service.ProposalRequest{
		CustomerID: f.custID, ConversationID: f.convID, RequestID: "req-1",
		ActionType: domain.ActionCancelOrder, OrderNo: "ORD-40001", Reason: "不想要了",
	})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if proposal.Action == nil {
		t.Fatalf("expected pending action, eligibility = %+v", proposal.Eligibility)
	}
	// Proposing must not mutate the order.
	if status := f.orderStatus(orderID); status != domain.OrderPaid {
		t.Fatalf("order status changed on proposal: %s", status)
	}

	res, err := f.svc.Actions.Confirm(f.ctx, f.custID, f.convID, proposal.Action.ID, "req-2")
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if res.Status != domain.ActionExecuted {
		t.Fatalf("status = %s, want executed", res.Status)
	}
	if status := f.orderStatus(orderID); status != domain.OrderCancelled {
		t.Fatalf("order status = %s, want cancelled", status)
	}
	var refundNo string
	var amount int64
	if err := f.db.QueryRow(`SELECT refund_no, amount_minor FROM refunds WHERE order_id = ?`, orderID).
		Scan(&refundNo, &amount); err != nil {
		t.Fatalf("refund not created: %v", err)
	}
	if amount != 129900 {
		t.Fatalf("refund amount = %d, want 129900", amount)
	}
	if f.auditCount(orderID) == 0 {
		t.Fatal("expected audit log for cancellation")
	}
}

func TestConfirmIsIdempotent(t *testing.T) {
	f := newFixture(t)
	paidAt := fixedNow.Add(-3 * time.Hour)
	orderID, _ := f.insertOrderWithItem("ORD-40002", orderOpts{status: domain.OrderPaid, paidMinor: 129900, paidAt: &paidAt})
	f.insertPayment(orderID, "PAY-40002", 129900)

	proposal, err := f.svc.Actions.Propose(f.ctx, service.ProposalRequest{
		CustomerID: f.custID, ConversationID: f.convID, RequestID: "req-3",
		ActionType: domain.ActionCancelOrder, OrderNo: "ORD-40002",
	})
	if err != nil || proposal.Action == nil {
		t.Fatalf("propose: %v (%+v)", err, proposal)
	}

	if _, err := f.svc.Actions.Confirm(f.ctx, f.custID, f.convID, proposal.Action.ID, "req-4"); err != nil {
		t.Fatalf("first confirm: %v", err)
	}
	second, err := f.svc.Actions.Confirm(f.ctx, f.custID, f.convID, proposal.Action.ID, "req-5")
	if err != nil {
		t.Fatalf("second confirm should be idempotent, got error: %v", err)
	}
	if !second.Idempotent {
		t.Fatal("second confirm should report idempotent=true")
	}
	var refunds int
	if err := f.db.QueryRow(`SELECT COUNT(1) FROM refunds WHERE order_id = ?`, orderID).Scan(&refunds); err != nil {
		t.Fatalf("count refunds: %v", err)
	}
	if refunds != 1 {
		t.Fatalf("refund rows = %d, want 1", refunds)
	}
	if f.auditCount(orderID) != 1 {
		t.Fatalf("audit rows = %d, want 1", f.auditCount(orderID))
	}
}

func TestRejectLeavesOrderUntouched(t *testing.T) {
	f := newFixture(t)
	orderID, _ := f.insertOrderWithItem("ORD-40003", orderOpts{status: domain.OrderPaid, paidMinor: 129900})

	proposal, err := f.svc.Actions.Propose(f.ctx, service.ProposalRequest{
		CustomerID: f.custID, ConversationID: f.convID, RequestID: "req-6",
		ActionType: domain.ActionCancelOrder, OrderNo: "ORD-40003",
	})
	if err != nil || proposal.Action == nil {
		t.Fatalf("propose: %v (%+v)", err, proposal)
	}
	res, err := f.svc.Actions.Reject(f.ctx, f.custID, f.convID, proposal.Action.ID)
	if err != nil {
		t.Fatalf("reject: %v", err)
	}
	if res.Status != domain.ActionRejected {
		t.Fatalf("status = %s, want rejected", res.Status)
	}
	if status := f.orderStatus(orderID); status != domain.OrderPaid {
		t.Fatalf("order changed after reject: %s", status)
	}
	var refunds int
	if err := f.db.QueryRow(`SELECT COUNT(1) FROM refunds WHERE order_id = ?`, orderID).Scan(&refunds); err != nil {
		t.Fatalf("count refunds: %v", err)
	}
	if refunds != 0 {
		t.Fatalf("refunds = %d, want 0", refunds)
	}
	// Confirming a rejected action must fail with a conflict.
	if _, err := f.svc.Actions.Confirm(f.ctx, f.custID, f.convID, proposal.Action.ID, "req-7"); err == nil {
		t.Fatal("confirm after reject must fail")
	}
}

func TestExpiredActionCannotBeConfirmed(t *testing.T) {
	f := newFixture(t)
	orderID, _ := f.insertOrderWithItem("ORD-40004", orderOpts{status: domain.OrderPaid, paidMinor: 129900})

	proposal, err := f.svc.Actions.Propose(f.ctx, service.ProposalRequest{
		CustomerID: f.custID, ConversationID: f.convID, RequestID: "req-8",
		ActionType: domain.ActionCancelOrder, OrderNo: "ORD-40004",
	})
	if err != nil || proposal.Action == nil {
		t.Fatalf("propose: %v (%+v)", err, proposal)
	}

	// Move the clock past the TTL: the action must expire instead of executing.
	expired := fixedNow.Add(11 * time.Minute)
	svc := service.New(f.store, service.Options{
		PendingActionTTL: 10 * time.Minute,
		Now:              func() time.Time { return expired },
	})
	_, err = svc.Actions.Confirm(f.ctx, f.custID, f.convID, proposal.Action.ID, "req-9")
	if err == nil {
		t.Fatal("expected action_expired error")
	}
	if appErr := domain.AsAppError(err); appErr.Code != domain.CodeActionExpired {
		t.Fatalf("code = %s, want action_expired", appErr.Code)
	}
	if status := f.orderStatus(orderID); status != domain.OrderPaid {
		t.Fatalf("order changed after expiry: %s", status)
	}
}

func TestConfirmModifyWritesWhitelistedFieldsOnly(t *testing.T) {
	f := newFixture(t)
	orderID, _ := f.insertOrderWithItem("ORD-40005", orderOpts{
		status: domain.OrderPaid, paidMinor: 129900, address: "旧地址 1 号",
	})

	proposal, err := f.svc.Actions.Propose(f.ctx, service.ProposalRequest{
		CustomerID: f.custID, ConversationID: f.convID, RequestID: "req-10",
		ActionType: domain.ActionModifyOrder, OrderNo: "ORD-40005",
		Changes: map[string]any{"shipping_address": "新地址 88 号", "receiver_phone": "13712345678"},
	})
	if err != nil || proposal.Action == nil {
		t.Fatalf("propose: %v (%+v)", err, proposal)
	}
	if _, err := f.svc.Actions.Confirm(f.ctx, f.custID, f.convID, proposal.Action.ID, "req-11"); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	var address, phone string
	var version int
	if err := f.db.QueryRow(`SELECT shipping_address, receiver_phone_masked, version FROM orders WHERE id = ?`, orderID).
		Scan(&address, &phone, &version); err != nil {
		t.Fatalf("read order: %v", err)
	}
	if address != "新地址 88 号" {
		t.Fatalf("address = %s", address)
	}
	if phone != "137****5678" {
		t.Fatalf("phone = %s, want masked value", phone)
	}
	if version != 2 {
		t.Fatalf("version = %d, want 2", version)
	}
}

func TestConfirmModifyConflictsWhenOrderChanged(t *testing.T) {
	f := newFixture(t)
	orderID, _ := f.insertOrderWithItem("ORD-40006", orderOpts{status: domain.OrderPaid, paidMinor: 129900})

	proposal, err := f.svc.Actions.Propose(f.ctx, service.ProposalRequest{
		CustomerID: f.custID, ConversationID: f.convID, RequestID: "req-12",
		ActionType: domain.ActionModifyOrder, OrderNo: "ORD-40006",
		Changes: map[string]any{"shipping_address": "第一次修改"},
	})
	if err != nil || proposal.Action == nil {
		t.Fatalf("propose: %v (%+v)", err, proposal)
	}
	// Simulate a concurrent mutation that bumps the version.
	if _, err := f.db.Exec(`UPDATE orders SET version = version + 1 WHERE id = ?`, orderID); err != nil {
		t.Fatalf("bump version: %v", err)
	}

	_, err = f.svc.Actions.Confirm(f.ctx, f.custID, f.convID, proposal.Action.ID, "req-13")
	if err == nil {
		t.Fatal("expected conflict after concurrent change")
	}
	if appErr := domain.AsAppError(err); appErr.Code != domain.CodeConflict {
		t.Fatalf("code = %s, want conflict", appErr.Code)
	}
}

func TestConfirmServiceRequestCreatesCaseAndRefund(t *testing.T) {
	f := newFixture(t)
	deliveredAt := fixedNow.Add(-2 * 24 * time.Hour)
	orderID, itemID := f.insertOrderWithItem("ORD-40007", orderOpts{
		status: domain.OrderCompleted, paidMinor: 259800, paidAt: timePtr(fixedNow.Add(-8 * 24 * time.Hour)),
		shippedAt: timePtr(fixedNow.Add(-6 * 24 * time.Hour)), deliveredAt: &deliveredAt,
		quantity: 2, unitPrice: 129900,
	})
	f.insertPayment(orderID, "PAY-40007", 259800)

	proposal, err := f.svc.Actions.Propose(f.ctx, service.ProposalRequest{
		CustomerID: f.custID, ConversationID: f.convID, RequestID: "req-14",
		ActionType: domain.CaseTypeRefund, OrderNo: "ORD-40007", OrderItemID: itemID,
		Quantity: 1, Reason: "商品有瑕疵",
	})
	if err != nil || proposal.Action == nil {
		t.Fatalf("propose: %v (%+v)", err, proposal)
	}
	res, err := f.svc.Actions.Confirm(f.ctx, f.custID, f.convID, proposal.Action.ID, "req-15")
	if err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if res.Result["case_no"] == nil {
		t.Fatalf("result missing case_no: %v", res.Result)
	}
	if res.Result["refund_amount_minor"] != int64(129900) {
		t.Fatalf("refund amount = %v, want 129900", res.Result["refund_amount_minor"])
	}
	var cases int
	if err := f.db.QueryRow(`SELECT COUNT(1) FROM service_cases WHERE order_id = ? AND case_type = 'refund'`, orderID).
		Scan(&cases); err != nil {
		t.Fatalf("count cases: %v", err)
	}
	if cases != 1 {
		t.Fatalf("cases = %d, want 1", cases)
	}
}

func TestProposeRejectedEligibilityCreatesNoAction(t *testing.T) {
	f := newFixture(t)
	shippedAt := fixedNow.Add(-24 * time.Hour)
	orderID, _ := f.insertOrderWithItem("ORD-40008", orderOpts{
		status: domain.OrderShipped, paidMinor: 129900, paidAt: timePtr(fixedNow.Add(-48 * time.Hour)), shippedAt: &shippedAt,
	})
	f.insertShipment(orderID, domain.ShipmentInTransit, "SF40008", fixedNow.Add(-20*time.Hour))

	proposal, err := f.svc.Actions.Propose(f.ctx, service.ProposalRequest{
		CustomerID: f.custID, ConversationID: f.convID, RequestID: "req-16",
		ActionType: domain.ActionCancelOrder, OrderNo: "ORD-40008",
	})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if proposal.Action != nil {
		t.Fatal("ineligible action must not be created")
	}
	var actions int
	if err := f.db.QueryRow(`SELECT COUNT(1) FROM pending_actions`).Scan(&actions); err != nil {
		t.Fatalf("count actions: %v", err)
	}
	if actions != 0 {
		t.Fatalf("pending actions = %d, want 0", actions)
	}
}
