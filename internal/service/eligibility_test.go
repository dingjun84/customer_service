package service_test

import (
	"testing"
	"time"

	"github.com/example/ecommerce-support/internal/domain"
	"github.com/example/ecommerce-support/internal/service"
)

func TestEligibilityCancelUnshippedOrder(t *testing.T) {
	f := newFixture(t)
	paidAt := fixedNow.Add(-2 * time.Hour)
	orderID, _ := f.insertOrderWithItem("ORD-30001", orderOpts{status: domain.OrderPaid, paidMinor: 129900, paidAt: &paidAt})
	f.insertPayment(orderID, "PAY-30001", 129900)

	res, err := f.svc.Eligibility.Check(f.ctx, service.EligibilityRequest{
		CustomerID: f.custID, ActionType: domain.ActionCancelOrder, OrderNo: "ORD-30001",
	})
	if err != nil {
		t.Fatalf("check eligibility: %v", err)
	}
	if !res.Eligible {
		t.Fatalf("eligible = false, reason = %s / %s", res.ReasonCode, res.Message)
	}
	if res.EstimatedAmountMinor == nil || *res.EstimatedAmountMinor != 129900 {
		t.Fatalf("estimated refund = %v, want 129900", res.EstimatedAmountMinor)
	}
}

func TestEligibilityCancelShippedOrderIsRejected(t *testing.T) {
	f := newFixture(t)
	shippedAt := fixedNow.Add(-24 * time.Hour)
	orderID, _ := f.insertOrderWithItem("ORD-30002", orderOpts{
		status: domain.OrderShipped, paidMinor: 129900, paidAt: timePtr(fixedNow.Add(-48 * time.Hour)), shippedAt: &shippedAt,
	})
	f.insertShipment(orderID, domain.ShipmentInTransit, "SF30002", fixedNow.Add(-20*time.Hour))

	res, err := f.svc.Eligibility.Check(f.ctx, service.EligibilityRequest{
		CustomerID: f.custID, ActionType: domain.ActionCancelOrder, OrderNo: "ORD-30002",
	})
	if err != nil {
		t.Fatalf("check eligibility: %v", err)
	}
	if res.Eligible {
		t.Fatal("shipped order must not be cancellable")
	}
	if res.ReasonCode != service.ReasonAlreadyShipped {
		t.Fatalf("reason = %s, want %s", res.ReasonCode, service.ReasonAlreadyShipped)
	}
}

func TestEligibilityModifyUnshippedOrderBuildsMaskedPreview(t *testing.T) {
	f := newFixture(t)
	f.insertOrderWithItem("ORD-30003", orderOpts{status: domain.OrderPaid, paidMinor: 129900, address: "旧地址 1 号"})

	res, err := f.svc.Eligibility.Check(f.ctx, service.EligibilityRequest{
		CustomerID: f.custID, ActionType: domain.ActionModifyOrder, OrderNo: "ORD-30003",
		Changes: map[string]any{"shipping_address": "新地址 2 号", "receiver_phone": "13900001111"},
	})
	if err != nil {
		t.Fatalf("check eligibility: %v", err)
	}
	if !res.Eligible {
		t.Fatalf("eligible = false, reason = %s / %s", res.ReasonCode, res.Message)
	}
	changes, ok := res.Preview["changes"].([]map[string]any)
	if !ok || len(changes) != 2 {
		t.Fatalf("preview changes = %#v", res.Preview["changes"])
	}
	for _, c := range changes {
		if c["field"] == "receiver_phone" {
			if c["after"] != "139****1111" {
				t.Fatalf("masked phone = %v, want 139****1111", c["after"])
			}
		}
	}
}

func TestEligibilityModifyRejectsNonWhitelistedField(t *testing.T) {
	f := newFixture(t)
	f.insertOrderWithItem("ORD-30004", orderOpts{status: domain.OrderPaid, paidMinor: 129900})

	res, err := f.svc.Eligibility.Check(f.ctx, service.EligibilityRequest{
		CustomerID: f.custID, ActionType: domain.ActionModifyOrder, OrderNo: "ORD-30004",
		Changes: map[string]any{"paid_amount_minor": "1"},
	})
	if err != nil {
		t.Fatalf("check eligibility: %v", err)
	}
	if res.Eligible || res.ReasonCode != service.ReasonInvalidField {
		t.Fatalf("eligible = %v, reason = %s; want invalid_field", res.Eligible, res.ReasonCode)
	}
}

func TestEligibilityReturnOutsideWindow(t *testing.T) {
	f := newFixture(t)
	deliveredAt := fixedNow.Add(-20 * 24 * time.Hour)
	f.insertOrderWithItem("ORD-30005", orderOpts{
		status: domain.OrderCompleted, paidMinor: 129900, paidAt: timePtr(fixedNow.Add(-30 * 24 * time.Hour)),
		shippedAt: timePtr(fixedNow.Add(-25 * 24 * time.Hour)), deliveredAt: &deliveredAt,
	})

	res, err := f.svc.Eligibility.Check(f.ctx, service.EligibilityRequest{
		CustomerID: f.custID, ActionType: domain.CaseTypeReturn, OrderNo: "ORD-30005", Quantity: 1,
	})
	if err != nil {
		t.Fatalf("check eligibility: %v", err)
	}
	if res.Eligible || res.ReasonCode != service.ReasonOutOfWindow {
		t.Fatalf("eligible = %v, reason = %s; want out_of_window", res.Eligible, res.ReasonCode)
	}
}

func TestEligibilityPartialRefundUsesProratedAmount(t *testing.T) {
	f := newFixture(t)
	deliveredAt := fixedNow.Add(-2 * 24 * time.Hour)
	f.insertOrderWithItem("ORD-30006", orderOpts{
		status: domain.OrderCompleted, paidMinor: 259800, paidAt: timePtr(fixedNow.Add(-6 * 24 * time.Hour)),
		shippedAt: timePtr(fixedNow.Add(-4 * 24 * time.Hour)), deliveredAt: &deliveredAt,
		quantity: 2, unitPrice: 129900,
	})

	res, err := f.svc.Eligibility.Check(f.ctx, service.EligibilityRequest{
		CustomerID: f.custID, ActionType: domain.CaseTypeRefund, OrderNo: "ORD-30006", Quantity: 1,
	})
	if err != nil {
		t.Fatalf("check eligibility: %v", err)
	}
	if !res.Eligible {
		t.Fatalf("eligible = false: %s / %s", res.ReasonCode, res.Message)
	}
	if res.EstimatedAmountMinor == nil || *res.EstimatedAmountMinor != 129900 {
		t.Fatalf("estimated refund = %v, want 129900", res.EstimatedAmountMinor)
	}

	over, err := f.svc.Eligibility.Check(f.ctx, service.EligibilityRequest{
		CustomerID: f.custID, ActionType: domain.CaseTypeRefund, OrderNo: "ORD-30006", Quantity: 5,
	})
	if err != nil {
		t.Fatalf("check eligibility (over qty): %v", err)
	}
	if over.Eligible || over.ReasonCode != service.ReasonQuantityExceeded {
		t.Fatalf("over-quantity eligible = %v, reason = %s", over.Eligible, over.ReasonCode)
	}
}

func TestEligibilityForbidsOtherCustomersOrder(t *testing.T) {
	f := newFixture(t)
	f.insertOrderWithItem("ORD-30007", orderOpts{status: domain.OrderPaid, paidMinor: 129900})
	other := f.insertCustomer("CUST-2002")

	_, err := f.svc.Eligibility.Check(f.ctx, service.EligibilityRequest{
		CustomerID: other, ActionType: domain.ActionCancelOrder, OrderNo: "ORD-30007",
	})
	if err == nil {
		t.Fatal("expected forbidden error for foreign order")
	}
	if appErr := domain.AsAppError(err); appErr.Code != domain.CodeForbidden {
		t.Fatalf("code = %s, want forbidden", appErr.Code)
	}
}

func TestEligibilityUnknownActionType(t *testing.T) {
	f := newFixture(t)
	f.insertOrderWithItem("ORD-30008", orderOpts{status: domain.OrderPaid, paidMinor: 129900})

	res, err := f.svc.Eligibility.Check(f.ctx, service.EligibilityRequest{
		CustomerID: f.custID, ActionType: "delete_order", OrderNo: "ORD-30008",
	})
	if err != nil {
		t.Fatalf("check eligibility: %v", err)
	}
	if res.Eligible || res.ReasonCode != service.ReasonUnsupported {
		t.Fatalf("eligible = %v, reason = %s", res.Eligible, res.ReasonCode)
	}
}
