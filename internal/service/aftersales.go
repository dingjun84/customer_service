package service

import (
	"context"
	"strings"

	"github.com/example/ecommerce-support/internal/domain"
)

// Refunds serves payment and refund status reads.
type Refunds struct{ core }

// RefundStatus is the authoritative answer to "where is my money".
type RefundStatus struct {
	OrderNo               string           `json:"order_no"`
	OrderStatus           string           `json:"order_status"`
	PaidAmountMinor       int64            `json:"paid_amount_minor"`
	RefundedAmountMinor   int64            `json:"refunded_amount_minor"`
	RefundableAmountMinor int64            `json:"refundable_amount_minor"`
	Payments              []domain.Payment `json:"payments"`
	Refunds               []domain.Refund  `json:"refunds"`
}

// Status returns payment/refund facts for one of the customer's orders.
func (s *Refunds) Status(ctx context.Context, customerID, orderNo, refundNo string) (*RefundStatus, error) {
	order, err := s.ownedOrder(ctx, customerID, orderNo)
	if err != nil {
		return nil, err
	}
	payments, err := s.store.ListPayments(ctx, order.ID)
	if err != nil {
		return nil, domain.AsAppError(err)
	}
	refunds, err := s.store.ListRefunds(ctx, order.ID)
	if err != nil {
		return nil, domain.AsAppError(err)
	}
	if rn := strings.TrimSpace(refundNo); rn != "" {
		filtered := refunds[:0]
		for _, r := range refunds {
			if r.RefundNo == rn {
				filtered = append(filtered, r)
			}
		}
		refunds = filtered
	}
	refundable := order.PaidAmountMinor - order.RefundedAmountMinor
	if refundable < 0 {
		refundable = 0
	}
	return &RefundStatus{
		OrderNo:               order.OrderNo,
		OrderStatus:           order.OrderStatus,
		PaidAmountMinor:       order.PaidAmountMinor,
		RefundedAmountMinor:   order.RefundedAmountMinor,
		RefundableAmountMinor: refundable,
		Payments:              payments,
		Refunds:               refunds,
	}, nil
}

// AfterSales serves after-sales case reads.
type AfterSales struct{ core }

// ListByOrder returns the after-sales cases of an order.
func (s *AfterSales) ListByOrder(ctx context.Context, customerID, orderNo string) ([]domain.ServiceCase, error) {
	order, err := s.ownedOrder(ctx, customerID, orderNo)
	if err != nil {
		return nil, err
	}
	cases, err := s.store.ListServiceCasesByOrder(ctx, order.ID)
	if err != nil {
		return nil, domain.AsAppError(err)
	}
	return cases, nil
}

// Get returns one case after verifying ownership.
func (s *AfterSales) Get(ctx context.Context, customerID, caseNo string) (*domain.ServiceCase, error) {
	return s.ownedCase(ctx, customerID, caseNo)
}

// Invoices serves invoice status reads.
type Invoices struct{ core }

// ListByOrder returns invoice records of one of the customer's orders.
func (s *Invoices) ListByOrder(ctx context.Context, customerID, orderNo string) ([]domain.Invoice, error) {
	order, err := s.ownedOrder(ctx, customerID, orderNo)
	if err != nil {
		return nil, err
	}
	invoices, err := s.store.ListInvoicesByOrder(ctx, order.ID)
	if err != nil {
		return nil, domain.AsAppError(err)
	}
	return invoices, nil
}

// Promotions serves campaign rules and customer benefits.
type Promotions struct{ core }

// List returns active promotions, optionally filtered by code or free text.
func (s *Promotions) List(ctx context.Context, query, code string, limit int) ([]domain.Promotion, error) {
	promos, err := s.store.ListPromotions(ctx, query, code, limit)
	if err != nil {
		return nil, domain.AsAppError(err)
	}
	return promos, nil
}

// Benefits returns the customer's coupons/points/insurance mock status.
func (s *Promotions) Benefits(ctx context.Context, customerID, benefitType string, limit int) ([]domain.CustomerBenefit, error) {
	if customerID == "" {
		return nil, domain.NewError(domain.CodeUnauthorized, "未识别客户身份")
	}
	benefits, err := s.store.ListBenefits(ctx, customerID, benefitType, limit)
	if err != nil {
		return nil, domain.AsAppError(err)
	}
	return benefits, nil
}
