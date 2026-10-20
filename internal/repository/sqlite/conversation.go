package sqlite

import (
	"context"
	"strings"
	"time"

	"github.com/example/ecommerce-support/internal/domain"
	"github.com/example/ecommerce-support/internal/repository"
)

const conversationColumns = `id, conversation_no, customer_id, status, channel,
	COALESCE(active_task_state_json,''), COALESCE(summary,''), created_at, updated_at, COALESCE(last_message_at,'')`

func scanConversation(scan func(dest ...any) error) (*domain.Conversation, error) {
	var c domain.Conversation
	var created, updated, lastMessage string
	if err := scan(&c.ID, &c.ConversationNo, &c.CustomerID, &c.Status, &c.Channel,
		&c.ActiveTaskStateJSON, &c.Summary, &created, &updated, &lastMessage); err != nil {
		return nil, translateErr(err)
	}
	c.CreatedAt = parseTS(created)
	c.UpdatedAt = parseTS(updated)
	c.LastMessageAt = parseTSPtr(lastMessage)
	return &c, nil
}

// CreateConversation inserts a new conversation.
func (s *Store) CreateConversation(ctx context.Context, c *domain.Conversation) error {
	_, err := s.conn().ExecContext(ctx, `
		INSERT INTO conversations (id, conversation_no, customer_id, status, channel, active_task_state_json,
		                           summary, created_at, updated_at, last_message_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		c.ID, c.ConversationNo, c.CustomerID, c.Status, c.Channel, nullString(c.ActiveTaskStateJSON),
		nullString(c.Summary), ts(c.CreatedAt), ts(c.UpdatedAt), tsPtr(c.LastMessageAt))
	return translateErr(err)
}

// GetConversation loads a conversation by ID.
func (s *Store) GetConversation(ctx context.Context, id string) (*domain.Conversation, error) {
	row := s.conn().QueryRowContext(ctx, `SELECT `+conversationColumns+` FROM conversations WHERE id = ?`, id)
	return scanConversation(row.Scan)
}

// ListConversations lists one customer's conversations, newest activity first.
func (s *Store) ListConversations(ctx context.Context, customerID string, limit int, cursor *repository.Cursor) ([]domain.Conversation, error) {
	limit = limitOrDefault(limit, 20, 100)
	where := []string{"customer_id = ?"}
	args := []any{customerID}
	if cursor != nil {
		where = append(where, `(created_at < ? OR (created_at = ? AND id < ?))`)
		args = append(args, ts(cursor.CreatedAt), ts(cursor.CreatedAt), cursor.ID)
	}
	args = append(args, limit)

	rows, err := s.conn().QueryContext(ctx,
		`SELECT `+conversationColumns+` FROM conversations WHERE `+strings.Join(where, " AND ")+
			` ORDER BY created_at DESC, id DESC LIMIT ?`, args...)
	if err != nil {
		return nil, translateErr(err)
	}
	defer rows.Close()

	out := []domain.Conversation{}
	for rows.Next() {
		c, err := scanConversation(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, translateErr(rows.Err())
}

// SetConversationStatus updates the lifecycle status of a conversation.
func (s *Store) SetConversationStatus(ctx context.Context, id, status string, now time.Time) error {
	res, err := s.conn().ExecContext(ctx,
		`UPDATE conversations SET status = ?, updated_at = ? WHERE id = ?`, status, ts(now), id)
	if err != nil {
		return translateErr(err)
	}
	if affected, err := res.RowsAffected(); err != nil {
		return translateErr(err)
	} else if affected == 0 {
		return domain.ErrNotFound
	}
	return nil
}

// UpdateConversationState replaces the active task state and summary.
func (s *Store) UpdateConversationState(ctx context.Context, id, summary, taskStateJSON string, now time.Time) error {
	_, err := s.conn().ExecContext(ctx,
		`UPDATE conversations SET summary = ?, active_task_state_json = ?, updated_at = ? WHERE id = ?`,
		nullString(summary), nullString(taskStateJSON), ts(now), id)
	return translateErr(err)
}

// TouchConversation records message activity.
func (s *Store) TouchConversation(ctx context.Context, id string, at time.Time) error {
	_, err := s.conn().ExecContext(ctx,
		`UPDATE conversations SET last_message_at = ?, updated_at = ? WHERE id = ?`, ts(at), ts(at), id)
	return translateErr(err)
}

const messageColumns = `id, conversation_id, role, content, COALESCE(tool_name,''), COALESCE(tool_call_id,''),
	COALESCE(tool_arguments_json,''), COALESCE(tool_result_json,''), metadata_json, created_at`

func scanMessage(scan func(dest ...any) error) (*domain.Message, error) {
	var m domain.Message
	var created string
	if err := scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.ToolName, &m.ToolCallID,
		&m.ToolArgumentsJSON, &m.ToolResultJSON, &m.MetadataJSON, &created); err != nil {
		return nil, translateErr(err)
	}
	m.CreatedAt = parseTS(created)
	return &m, nil
}

// AppendMessage inserts one message row.
func (s *Store) AppendMessage(ctx context.Context, m *domain.Message) error {
	_, err := s.conn().ExecContext(ctx, `
		INSERT INTO messages (id, conversation_id, role, content, tool_name, tool_call_id,
		                      tool_arguments_json, tool_result_json, metadata_json, created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?)`,
		m.ID, m.ConversationID, m.Role, m.Content, nullString(m.ToolName), nullString(m.ToolCallID),
		nullString(m.ToolArgumentsJSON), nullString(m.ToolResultJSON), defaultJSON(m.MetadataJSON), ts(m.CreatedAt))
	return translateErr(err)
}

// GetMessage loads a single message by ID.
func (s *Store) GetMessage(ctx context.Context, id string) (*domain.Message, error) {
	row := s.conn().QueryRowContext(ctx, `SELECT `+messageColumns+` FROM messages WHERE id = ?`, id)
	return scanMessage(row.Scan)
}

// ListMessages returns one page of history in chronological order. The cursor
// points at the oldest message of the previous page (paging backwards).
//
// Ordering uses the implicit rowid (insertion order) rather than created_at:
// timestamps are second-precision, so several messages of one turn share the
// same second and an id-based tie-break would scramble the conversation.
func (s *Store) ListMessages(ctx context.Context, q repository.MessageQuery) ([]domain.Message, error) {
	limit := limitOrDefault(q.Limit, 50, 100)
	where := []string{"conversation_id = ?"}
	args := []any{q.ConversationID}
	if q.Before != nil {
		where = append(where, `rowid < (SELECT rowid FROM messages WHERE id = ?)`)
		args = append(args, q.Before.ID)
	}
	args = append(args, limit)

	rows, err := s.conn().QueryContext(ctx,
		`SELECT `+messageColumns+` FROM messages WHERE `+strings.Join(where, " AND ")+
			` ORDER BY rowid DESC LIMIT ?`, args...)
	if err != nil {
		return nil, translateErr(err)
	}
	defer rows.Close()

	out := make([]domain.Message, 0, limit)
	for rows.Next() {
		m, err := scanMessage(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	if err := rows.Err(); err != nil {
		return nil, translateErr(err)
	}
	reverseMessages(out)
	return out, nil
}

// ListRecentMessages returns the newest limit messages in chronological order.
// It uses insertion order for the same reason as ListMessages.
func (s *Store) ListRecentMessages(ctx context.Context, conversationID string, limit int) ([]domain.Message, error) {
	limit = limitOrDefault(limit, 40, 400)
	rows, err := s.conn().QueryContext(ctx,
		`SELECT `+messageColumns+` FROM messages WHERE conversation_id = ?
		  ORDER BY rowid DESC LIMIT ?`, conversationID, limit)
	if err != nil {
		return nil, translateErr(err)
	}
	defer rows.Close()

	out := make([]domain.Message, 0, limit)
	for rows.Next() {
		m, err := scanMessage(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	if err := rows.Err(); err != nil {
		return nil, translateErr(err)
	}
	reverseMessages(out)
	return out, nil
}

func reverseMessages(msgs []domain.Message) {
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
}

func defaultJSON(raw string) string {
	if strings.TrimSpace(raw) == "" {
		return "{}"
	}
	return raw
}

const toolExecutionColumns = `id, conversation_id, COALESCE(message_id,''), tool_call_id, tool_name,
	input_json, COALESCE(output_json,''), status, COALESCE(error_code,''), COALESCE(error_message_safe,''),
	latency_ms, created_at, COALESCE(finished_at,'')`

// CreateToolExecution records a tool call in the started state.
func (s *Store) CreateToolExecution(ctx context.Context, e *domain.ToolExecution) error {
	_, err := s.conn().ExecContext(ctx, `
		INSERT INTO tool_executions (id, conversation_id, message_id, tool_call_id, tool_name, input_json,
		                             output_json, status, error_code, error_message_safe, latency_ms, created_at, finished_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		e.ID, e.ConversationID, nullString(e.MessageID), e.ToolCallID, e.ToolName, defaultJSON(e.InputJSON),
		nullString(e.OutputJSON), e.Status, nullString(e.ErrorCode), nullString(e.ErrorMessageSafe),
		e.LatencyMS, ts(e.CreatedAt), tsPtr(e.FinishedAt))
	return translateErr(err)
}

// FinishToolExecution closes a tool call row.
func (s *Store) FinishToolExecution(ctx context.Context, id, status, outputJSON, errorCode, errorMessageSafe string, latencyMS int64, finishedAt time.Time) error {
	_, err := s.conn().ExecContext(ctx, `
		UPDATE tool_executions
		   SET status = ?, output_json = ?, error_code = ?, error_message_safe = ?, latency_ms = ?, finished_at = ?
		 WHERE id = ?`,
		status, nullString(outputJSON), nullString(errorCode), nullString(errorMessageSafe), latencyMS, ts(finishedAt), id)
	return translateErr(err)
}

// ListToolExecutions returns the newest tool calls of a conversation.
func (s *Store) ListToolExecutions(ctx context.Context, conversationID string, limit int) ([]domain.ToolExecution, error) {
	limit = limitOrDefault(limit, 50, 200)
	rows, err := s.conn().QueryContext(ctx,
		`SELECT `+toolExecutionColumns+` FROM tool_executions WHERE conversation_id = ?
		  ORDER BY rowid DESC LIMIT ?`, conversationID, limit)
	if err != nil {
		return nil, translateErr(err)
	}
	defer rows.Close()

	out := []domain.ToolExecution{}
	for rows.Next() {
		var e domain.ToolExecution
		var created, finished string
		if err := rows.Scan(&e.ID, &e.ConversationID, &e.MessageID, &e.ToolCallID, &e.ToolName,
			&e.InputJSON, &e.OutputJSON, &e.Status, &e.ErrorCode, &e.ErrorMessageSafe,
			&e.LatencyMS, &created, &finished); err != nil {
			return nil, translateErr(err)
		}
		e.CreatedAt = parseTS(created)
		e.FinishedAt = parseTSPtr(finished)
		out = append(out, e)
	}
	return out, translateErr(rows.Err())
}
