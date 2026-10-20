package sqlite

import (
	"context"

	"github.com/example/ecommerce-support/internal/domain"
)

// CreateTicket inserts a human-transfer/complaint/quality ticket.
func (s *Store) CreateTicket(ctx context.Context, t *domain.ServiceTicket) error {
	_, err := s.conn().ExecContext(ctx, `
		INSERT INTO service_tickets (id, ticket_no, customer_id, conversation_id, order_id, ticket_type, status,
		                             priority, category, summary, details, assigned_to, created_at, updated_at, closed_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.TicketNo, t.CustomerID, nullString(t.ConversationID), nullString(t.OrderID), t.TicketType, t.Status,
		t.Priority, nullString(t.Category), t.Summary, nullString(t.Details), nullString(t.AssignedTo),
		ts(t.CreatedAt), ts(t.UpdatedAt), tsPtr(t.ClosedAt))
	return translateErr(err)
}

// GetIdempotency loads a stored idempotency record.
func (s *Store) GetIdempotency(ctx context.Context, key string) (*domain.IdempotencyRecord, error) {
	row := s.conn().QueryRowContext(ctx,
		`SELECT idempotency_key, operation, request_hash, COALESCE(response_json,''), created_at, expires_at
		   FROM idempotency_keys WHERE idempotency_key = ?`, key)
	var rec domain.IdempotencyRecord
	var created, expires string
	if err := row.Scan(&rec.Key, &rec.Operation, &rec.RequestHash, &rec.ResponseJSON, &created, &expires); err != nil {
		return nil, translateErr(err)
	}
	rec.CreatedAt = parseTS(created)
	rec.ExpiresAt = parseTS(expires)
	return &rec, nil
}

// PutIdempotency stores (or refreshes) an idempotency record.
func (s *Store) PutIdempotency(ctx context.Context, rec *domain.IdempotencyRecord) error {
	_, err := s.conn().ExecContext(ctx, `
		INSERT INTO idempotency_keys (idempotency_key, operation, request_hash, response_json, created_at, expires_at)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(idempotency_key) DO UPDATE SET
			operation = excluded.operation,
			request_hash = excluded.request_hash,
			response_json = excluded.response_json,
			expires_at = excluded.expires_at`,
		rec.Key, rec.Operation, rec.RequestHash, nullString(rec.ResponseJSON), ts(rec.CreatedAt), ts(rec.ExpiresAt))
	return translateErr(err)
}
