package sqlite

import (
	"context"
	"strings"
	"time"

	"github.com/example/ecommerce-support/internal/domain"
)

const pendingActionColumns = `id, action_no, conversation_id, customer_id, action_type, target_type, target_id,
	payload_json, preview_json, status, expires_at, created_at, COALESCE(confirmed_at,''),
	COALESCE(executed_at,''), COALESCE(result_json,'')`

func scanPendingAction(scan func(dest ...any) error) (*domain.PendingAction, error) {
	var a domain.PendingAction
	var expiresAt, created, confirmedAt, executedAt string
	if err := scan(&a.ID, &a.ActionNo, &a.ConversationID, &a.CustomerID, &a.ActionType, &a.TargetType, &a.TargetID,
		&a.PayloadJSON, &a.PreviewJSON, &a.Status, &expiresAt, &created, &confirmedAt, &executedAt, &a.ResultJSON); err != nil {
		return nil, translateErr(err)
	}
	a.ExpiresAt = parseTS(expiresAt)
	a.CreatedAt = parseTS(created)
	a.ConfirmedAt = parseTSPtr(confirmedAt)
	a.ExecutedAt = parseTSPtr(executedAt)
	return &a, nil
}

// CreatePendingAction inserts a write proposal awaiting confirmation.
func (s *Store) CreatePendingAction(ctx context.Context, a *domain.PendingAction) error {
	_, err := s.conn().ExecContext(ctx, `
		INSERT INTO pending_actions (id, action_no, conversation_id, customer_id, action_type, target_type, target_id,
		                             payload_json, preview_json, status, expires_at, created_at, confirmed_at,
		                             executed_at, result_json)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.ActionNo, a.ConversationID, a.CustomerID, a.ActionType, a.TargetType, a.TargetID,
		defaultJSON(a.PayloadJSON), defaultJSON(a.PreviewJSON), a.Status, ts(a.ExpiresAt), ts(a.CreatedAt),
		tsPtr(a.ConfirmedAt), tsPtr(a.ExecutedAt), nullString(a.ResultJSON))
	return translateErr(err)
}

// GetPendingAction loads an action by ID. Ownership checks live in the service.
func (s *Store) GetPendingAction(ctx context.Context, id string) (*domain.PendingAction, error) {
	row := s.conn().QueryRowContext(ctx, `SELECT `+pendingActionColumns+` FROM pending_actions WHERE id = ?`, id)
	return scanPendingAction(row.Scan)
}

// ListPendingActions lists actions of a conversation, optionally by status.
func (s *Store) ListPendingActions(ctx context.Context, conversationID, status string, limit int) ([]domain.PendingAction, error) {
	limit = limitOrDefault(limit, 20, 100)
	where := []string{"conversation_id = ?"}
	args := []any{conversationID}
	if st := strings.TrimSpace(status); st != "" {
		where = append(where, "status = ?")
		args = append(args, st)
	}
	args = append(args, limit)

	rows, err := s.conn().QueryContext(ctx,
		`SELECT `+pendingActionColumns+` FROM pending_actions WHERE `+strings.Join(where, " AND ")+
			` ORDER BY created_at DESC, id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, translateErr(err)
	}
	defer rows.Close()

	out := []domain.PendingAction{}
	for rows.Next() {
		a, err := scanPendingAction(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, translateErr(rows.Err())
}

// ClaimPendingAction atomically transitions pending -> confirmed. Exactly one
// concurrent caller can win, which is what makes confirmations idempotent.
func (s *Store) ClaimPendingAction(ctx context.Context, id string, now time.Time) (bool, error) {
	res, err := s.conn().ExecContext(ctx,
		`UPDATE pending_actions SET status = 'confirmed', confirmed_at = ? WHERE id = ? AND status = 'pending'`,
		ts(now), id)
	if err != nil {
		return false, translateErr(err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, translateErr(err)
	}
	return affected == 1, nil
}

// RejectPendingAction atomically transitions pending -> rejected.
func (s *Store) RejectPendingAction(ctx context.Context, id string, now time.Time) (bool, error) {
	res, err := s.conn().ExecContext(ctx,
		`UPDATE pending_actions SET status = 'rejected', confirmed_at = ? WHERE id = ? AND status = 'pending'`,
		ts(now), id)
	if err != nil {
		return false, translateErr(err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, translateErr(err)
	}
	return affected == 1, nil
}

// FinishPendingAction stores the terminal status and result payload.
func (s *Store) FinishPendingAction(ctx context.Context, id, status, resultJSON string, executedAt *time.Time) error {
	_, err := s.conn().ExecContext(ctx,
		`UPDATE pending_actions SET status = ?, result_json = ?, executed_at = ? WHERE id = ?`,
		status, nullString(resultJSON), tsPtr(executedAt), id)
	return translateErr(err)
}

// ExpirePendingActions marks stale pending actions as expired.
func (s *Store) ExpirePendingActions(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.conn().ExecContext(ctx,
		`UPDATE pending_actions SET status = 'expired' WHERE status = 'pending' AND expires_at < ?`, ts(now))
	if err != nil {
		return 0, translateErr(err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, translateErr(err)
	}
	return affected, nil
}

const auditColumns = `id, COALESCE(request_id,''), COALESCE(customer_id,''), COALESCE(conversation_id,''),
	action_type, target_type, target_id, COALESCE(before_json,''), COALESCE(after_json,''),
	actor_type, COALESCE(actor_id,''), result, COALESCE(reason,''), created_at`

// AppendAuditLog writes an immutable audit record.
func (s *Store) AppendAuditLog(ctx context.Context, log *domain.ActionAuditLog) error {
	_, err := s.conn().ExecContext(ctx, `
		INSERT INTO action_audit_logs (id, request_id, customer_id, conversation_id, action_type, target_type,
		                               target_id, before_json, after_json, actor_type, actor_id, result, reason, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		log.ID, nullString(log.RequestID), nullString(log.CustomerID), nullString(log.ConversationID),
		log.ActionType, log.TargetType, log.TargetID, nullString(log.BeforeJSON), nullString(log.AfterJSON),
		log.ActorType, nullString(log.ActorID), log.Result, nullString(log.Reason), ts(log.CreatedAt))
	return translateErr(err)
}

// ListAuditLogs returns audit records for a conversation, newest first.
func (s *Store) ListAuditLogs(ctx context.Context, conversationID string, limit int) ([]domain.ActionAuditLog, error) {
	limit = limitOrDefault(limit, 50, 200)
	rows, err := s.conn().QueryContext(ctx,
		`SELECT `+auditColumns+` FROM action_audit_logs WHERE conversation_id = ?
		  ORDER BY created_at DESC, id DESC LIMIT ?`, conversationID, limit)
	if err != nil {
		return nil, translateErr(err)
	}
	defer rows.Close()

	out := []domain.ActionAuditLog{}
	for rows.Next() {
		var a domain.ActionAuditLog
		var created string
		if err := rows.Scan(&a.ID, &a.RequestID, &a.CustomerID, &a.ConversationID, &a.ActionType, &a.TargetType,
			&a.TargetID, &a.BeforeJSON, &a.AfterJSON, &a.ActorType, &a.ActorID, &a.Result, &a.Reason, &created); err != nil {
			return nil, translateErr(err)
		}
		a.CreatedAt = parseTS(created)
		out = append(out, a)
	}
	return out, translateErr(rows.Err())
}
