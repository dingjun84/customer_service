package sqlite

import (
	"context"

	"github.com/example/ecommerce-support/internal/domain"
)

const paymentColumns = `id, payment_no, order_id, payment_method, payment_status, amount_minor,
	refunded_amount_minor, COALESCE(provider_transaction_ref,''), COALESCE(paid_at,''), created_at, updated_at`

// ListPayments returns payment records of an order, newest first.
func (s *Store) ListPayments(ctx context.Context, orderID string) ([]domain.Payment, error) {
	rows, err := s.conn().QueryContext(ctx,
		`SELECT `+paymentColumns+` FROM payments WHERE order_id = ? ORDER BY created_at DESC, id DESC`, orderID)
	if err != nil {
		return nil, translateErr(err)
	}
	defer rows.Close()

	out := []domain.Payment{}
	for rows.Next() {
		var p domain.Payment
		var paidAt, created, updated string
		if err := rows.Scan(&p.ID, &p.PaymentNo, &p.OrderID, &p.PaymentMethod, &p.PaymentStatus, &p.AmountMinor,
			&p.RefundedAmountMinor, &p.ProviderTransactionRef, &paidAt, &created, &updated); err != nil {
			return nil, translateErr(err)
		}
		p.PaidAt = parseTSPtr(paidAt)
		p.CreatedAt = parseTS(created)
		p.UpdatedAt = parseTS(updated)
		out = append(out, p)
	}
	return out, translateErr(rows.Err())
}

const refundColumns = `id, refund_no, order_id, COALESCE(payment_id,''), COALESCE(service_case_id,''),
	refund_status, COALESCE(refund_channel,''), amount_minor, COALESCE(reason,''), requested_at,
	COALESCE(completed_at,''), COALESCE(failure_reason,''), created_at, updated_at`

func scanRefund(scan func(dest ...any) error) (*domain.Refund, error) {
	var r domain.Refund
	var requestedAt, completedAt, created, updated string
	if err := scan(&r.ID, &r.RefundNo, &r.OrderID, &r.PaymentID, &r.ServiceCaseID,
		&r.RefundStatus, &r.RefundChannel, &r.AmountMinor, &r.Reason, &requestedAt,
		&completedAt, &r.FailureReason, &created, &updated); err != nil {
		return nil, translateErr(err)
	}
	r.RequestedAt = parseTS(requestedAt)
	r.CompletedAt = parseTSPtr(completedAt)
	r.CreatedAt = parseTS(created)
	r.UpdatedAt = parseTS(updated)
	return &r, nil
}

// ListRefunds returns refund records of an order, newest first.
func (s *Store) ListRefunds(ctx context.Context, orderID string) ([]domain.Refund, error) {
	rows, err := s.conn().QueryContext(ctx,
		`SELECT `+refundColumns+` FROM refunds WHERE order_id = ? ORDER BY requested_at DESC, id DESC`, orderID)
	if err != nil {
		return nil, translateErr(err)
	}
	defer rows.Close()

	out := []domain.Refund{}
	for rows.Next() {
		r, err := scanRefund(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, translateErr(rows.Err())
}

// GetRefundByNo loads a refund by business number.
func (s *Store) GetRefundByNo(ctx context.Context, refundNo string) (*domain.Refund, error) {
	row := s.conn().QueryRowContext(ctx, `SELECT `+refundColumns+` FROM refunds WHERE refund_no = ?`, refundNo)
	return scanRefund(row.Scan)
}

// CreateRefund inserts a refund record.
func (s *Store) CreateRefund(ctx context.Context, r *domain.Refund) error {
	_, err := s.conn().ExecContext(ctx, `
		INSERT INTO refunds (id, refund_no, order_id, payment_id, service_case_id, refund_status, refund_channel,
		                     amount_minor, reason, requested_at, completed_at, failure_reason, created_at, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		r.ID, r.RefundNo, r.OrderID, nullString(r.PaymentID), nullString(r.ServiceCaseID), r.RefundStatus,
		nullString(r.RefundChannel), r.AmountMinor, nullString(r.Reason), ts(r.RequestedAt),
		tsPtr(r.CompletedAt), nullString(r.FailureReason), ts(r.CreatedAt), ts(r.UpdatedAt))
	return translateErr(err)
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
