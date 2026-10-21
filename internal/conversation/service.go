// Package conversation owns chat session lifecycle: creating sessions, persisting
// messages, de-duplicating client retries and delegating one turn to the AI
// orchestrator.
package conversation

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/example/ecommerce-support/internal/ai"
	"github.com/example/ecommerce-support/internal/domain"
	"github.com/example/ecommerce-support/internal/repository"
	"github.com/example/ecommerce-support/internal/service"
)

// Options configures the conversation service.
type Options struct {
	IdempotencyTTL  time.Duration
	MaxMessageChars int
	HistoryLimit    int
	Logger          *slog.Logger
}

// Service implements session and message handling.
type Service struct {
	store        repository.Store
	orchestrator *ai.Orchestrator
	actions      *service.Actions
	opts         Options
	logger       *slog.Logger
	now          func() time.Time
}

// New builds the conversation service.
func New(store repository.Store, orchestrator *ai.Orchestrator, actions *service.Actions, opts Options) *Service {
	if opts.IdempotencyTTL <= 0 {
		opts.IdempotencyTTL = 24 * time.Hour
	}
	if opts.MaxMessageChars <= 0 {
		opts.MaxMessageChars = 2000
	}
	if opts.HistoryLimit <= 0 {
		opts.HistoryLimit = 40
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{store: store, orchestrator: orchestrator, actions: actions, opts: opts,
		logger: logger, now: func() time.Time { return time.Now().UTC() }}
}

// MessageView is the API-safe projection of a stored message.
type MessageView struct {
	ID        string `json:"id"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	ToolName  string `json:"tool_name,omitempty"`
	Reason    string `json:"reason,omitempty"`
	CreatedAt string `json:"created_at"`
}

// PendingActionView is what the frontend renders as a confirmation card.
type PendingActionView struct {
	ActionID   string         `json:"action_id"`
	ActionNo   string         `json:"action_no"`
	ActionType string         `json:"action_type"`
	Summary    string         `json:"summary"`
	Preview    map[string]any `json:"preview"`
	ExpiresAt  string         `json:"expires_at"`
	Status     string         `json:"status"`
}

// Turn is the HTTP response of one message exchange.
type Turn struct {
	ConversationID   string              `json:"conversation_id"`
	UserMessage      *MessageView        `json:"user_message,omitempty"`
	AssistantMessage *MessageView        `json:"assistant_message,omitempty"`
	ToolExecutions   []ai.ToolTrace      `json:"tool_executions"`
	Reasons          []ai.TurnReason     `json:"reasons"`
	PendingActions   []PendingActionView `json:"pending_actions"`
	Degraded         bool                `json:"degraded,omitempty"`
	Idempotent       bool                `json:"idempotent,omitempty"`
}

// Create opens a new conversation for the customer.
func (s *Service) Create(ctx context.Context, customerID, channel string) (*domain.Conversation, error) {
	if customerID == "" {
		return nil, domain.NewError(domain.CodeUnauthorized, "未识别客户身份")
	}
	if _, err := s.store.GetCustomer(ctx, customerID); err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.NotFoundError("客户不存在")
		}
		return nil, domain.AsAppError(err)
	}
	channel = strings.TrimSpace(channel)
	if channel == "" {
		channel = "web_demo"
	}
	now := s.now().Truncate(time.Second)
	conv := &domain.Conversation{
		ID:             domain.NewID(),
		ConversationNo: domain.HumanNo("CONV", now),
		CustomerID:     customerID,
		Status:         domain.ConversationOpen,
		Channel:        channel,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if err := s.store.CreateConversation(ctx, conv); err != nil {
		return nil, domain.AsAppError(err)
	}
	return conv, nil
}

// Get returns a conversation after verifying ownership.
func (s *Service) Get(ctx context.Context, customerID, conversationID string) (*domain.Conversation, error) {
	return s.owned(ctx, customerID, conversationID)
}

// List returns the customer's conversations.
func (s *Service) List(ctx context.Context, customerID string, limit int, cursor *repository.Cursor) ([]domain.Conversation, error) {
	if customerID == "" {
		return nil, domain.NewError(domain.CodeUnauthorized, "未识别客户身份")
	}
	convs, err := s.store.ListConversations(ctx, customerID, limit, cursor)
	if err != nil {
		return nil, domain.AsAppError(err)
	}
	return convs, nil
}

// Close marks a conversation as closed.
func (s *Service) Close(ctx context.Context, customerID, conversationID string) (*domain.Conversation, error) {
	conv, err := s.owned(ctx, customerID, conversationID)
	if err != nil {
		return nil, err
	}
	if conv.Status == domain.ConversationClosed {
		return conv, nil
	}
	if err := s.store.SetConversationStatus(ctx, conv.ID, domain.ConversationClosed, s.now().UTC()); err != nil {
		return nil, domain.AsAppError(err)
	}
	conv.Status = domain.ConversationClosed
	return conv, nil
}

// Messages returns a page of chat history for the UI, hiding internal tool rows.
func (s *Service) Messages(ctx context.Context, customerID, conversationID string, limit int, before *repository.Cursor) ([]MessageView, error) {
	if _, err := s.owned(ctx, customerID, conversationID); err != nil {
		return nil, err
	}
	rows, err := s.store.ListMessages(ctx, repository.MessageQuery{
		ConversationID: conversationID, Limit: limit, Before: before,
	})
	if err != nil {
		return nil, domain.AsAppError(err)
	}
	out := make([]MessageView, 0, len(rows))
	for _, m := range rows {
		if m.Role == domain.RoleTool {
			continue
		}
		// Assistant rows that only hold tool-call arguments are internal.
		if m.Role == domain.RoleAssistant && m.Content == "" {
			continue
		}
		out = append(out, toMessageView(&m))
	}
	return out, nil
}

// SendInput describes one user message.
type SendInput struct {
	CustomerID      string
	ConversationID  string
	Text            string
	ClientMessageID string
	RequestID       string
}

// SendMessage persists the user message, runs the agent loop and returns the turn.
//
// A client_message_id makes retries safe: a retry replays the stored response, and
// a retry after an LLM failure reuses the already persisted user message instead
// of inserting a duplicate.
func (s *Service) SendMessage(ctx context.Context, in SendInput) (*Turn, error) {
	text := strings.TrimSpace(in.Text)
	if text == "" {
		return nil, domain.ValidationError("message 不能为空")
	}
	if len([]rune(text)) > s.opts.MaxMessageChars {
		return nil, domain.ValidationError("消息过长，请控制在 2000 字以内")
	}
	conv, err := s.owned(ctx, in.CustomerID, in.ConversationID)
	if err != nil {
		return nil, err
	}
	if conv.Status == domain.ConversationClosed {
		return nil, domain.ConflictError("会话已关闭，请新建会话")
	}

	idemKey := ""
	var userMsg *domain.Message
	if cmid := strings.TrimSpace(in.ClientMessageID); cmid != "" {
		idemKey = "conv:" + conv.ID + ":client:" + cmid
		record, err := s.store.GetIdempotency(ctx, idemKey)
		if err == nil && record != nil && record.ExpiresAt.After(s.now()) {
			var cached Turn
			if record.ResponseJSON != "" {
				if err := json.Unmarshal([]byte(record.ResponseJSON), &cached); err == nil && cached.AssistantMessage != nil {
					cached.Idempotent = true
					return &cached, nil
				}
			}
			var partial struct {
				Pending   bool   `json:"pending"`
				UserMsgID string `json:"user_message_id"`
			}
			if err := json.Unmarshal([]byte(record.ResponseJSON), &partial); err == nil && partial.UserMsgID != "" {
				if msg, err := s.store.GetMessage(ctx, partial.UserMsgID); err == nil {
					userMsg = msg
				}
			}
		}
	}

	if userMsg == nil {
		userMsg = &domain.Message{
			ID:             domain.NewID(),
			ConversationID: conv.ID,
			Role:           domain.RoleUser,
			Content:        text,
			MetadataJSON:   "{}",
			CreatedAt:      s.now().Truncate(time.Second),
		}
		if err := s.store.AppendMessage(ctx, userMsg); err != nil {
			return nil, domain.AsAppError(err)
		}
	}

	// Expire stale proposals so the model never sees an action that can no longer run.
	if _, err := s.actions.ExpireStaleActions(ctx); err != nil {
		s.logger.Warn("expire pending actions failed", "conversation_id", conv.ID, "error", err.Error())
	}

	if err := s.store.TouchConversation(ctx, conv.ID, userMsg.CreatedAt); err != nil {
		s.logger.Warn("touch conversation failed", "conversation_id", conv.ID, "error", err.Error())
	}

	if idemKey != "" {
		partial, _ := json.Marshal(map[string]any{"pending": true, "user_message_id": userMsg.ID})
		if err := s.store.PutIdempotency(ctx, &domain.IdempotencyRecord{
			Key:          idemKey,
			Operation:    "send_message",
			RequestHash:  hashText(text),
			ResponseJSON: string(partial),
			CreatedAt:    s.now(),
			ExpiresAt:    s.now().Add(s.opts.IdempotencyTTL),
		}); err != nil {
			s.logger.Warn("store idempotency record failed", "conversation_id", conv.ID, "error", err.Error())
		}
	}

	history, err := s.store.ListRecentMessages(ctx, conv.ID, s.opts.HistoryLimit)
	if err != nil {
		return nil, domain.AsAppError(err)
	}
	// The conversation row carries the active task state used for context.
	if fresh, err := s.store.GetConversation(ctx, conv.ID); err == nil {
		conv = fresh
	}

	run, err := s.orchestrator.Run(ctx, ai.TurnInput{
		Conversation: conv,
		CustomerID:   in.CustomerID,
		RequestID:    in.RequestID,
		History:      history,
	})
	if err != nil {
		return nil, err
	}

	turn := &Turn{
		ConversationID: conv.ID,
		UserMessage:    viewPtr(toMessageView(userMsg)),
		ToolExecutions: run.ToolTraces,
		Reasons:        run.Reasons,
		PendingActions: pendingViews(run.PendingActions),
		Degraded:       run.Degraded,
	}
	if turn.ToolExecutions == nil {
		turn.ToolExecutions = []ai.ToolTrace{}
	}
	if turn.Reasons == nil {
		turn.Reasons = []ai.TurnReason{}
	}
	if run.Assistant != nil {
		turn.AssistantMessage = viewPtr(toMessageView(run.Assistant))
	}

	if idemKey != "" {
		if raw, err := json.Marshal(turn); err == nil {
			if err := s.store.PutIdempotency(ctx, &domain.IdempotencyRecord{
				Key:          idemKey,
				Operation:    "send_message",
				RequestHash:  hashText(text),
				ResponseJSON: string(raw),
				CreatedAt:    s.now(),
				ExpiresAt:    s.now().Add(s.opts.IdempotencyTTL),
			}); err != nil {
				s.logger.Warn("store idempotency result failed", "conversation_id", conv.ID, "error", err.Error())
			}
		}
	}
	return turn, nil
}

func (s *Service) owned(ctx context.Context, customerID, conversationID string) (*domain.Conversation, error) {
	if customerID == "" {
		return nil, domain.NewError(domain.CodeUnauthorized, "未识别客户身份")
	}
	if conversationID == "" {
		return nil, domain.ValidationError("conversation_id 不能为空")
	}
	conv, err := s.store.GetConversation(ctx, conversationID)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, domain.NotFoundError("未找到该会话")
		}
		return nil, domain.AsAppError(err)
	}
	if conv.CustomerID != customerID {
		return nil, domain.ForbiddenError("该会话不属于当前客户")
	}
	return conv, nil
}

func toMessageView(m *domain.Message) MessageView {
	return MessageView{
		ID:        m.ID,
		Role:      m.Role,
		Content:   m.Content,
		ToolName:  m.ToolName,
		Reason:    reasonOf(m),
		CreatedAt: m.CreatedAt.Format(time.RFC3339),
	}
}

// reasonOf reads the note persisted in metadata. Rebuilt history therefore shows
// the same note the live turn returned.
func reasonOf(m *domain.Message) string {
	if m.MetadataJSON == "" {
		return ""
	}
	var meta struct {
		Reason string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(m.MetadataJSON), &meta); err != nil {
		return ""
	}
	return meta.Reason
}

func viewPtr(v MessageView) *MessageView { return &v }

// pendingViews decodes the stored preview so the frontend can render a card.
func pendingViews(actions []domain.PendingAction) []PendingActionView {
	out := make([]PendingActionView, 0, len(actions))
	for _, a := range actions {
		preview := map[string]any{}
		if a.PreviewJSON != "" {
			_ = json.Unmarshal([]byte(a.PreviewJSON), &preview)
		}
		summary, _ := preview["summary"].(string)
		if summary == "" {
			summary = a.ActionNo
		}
		out = append(out, PendingActionView{
			ActionID:   a.ID,
			ActionNo:   a.ActionNo,
			ActionType: actionDisplayType(a),
			Summary:    summary,
			Preview:    preview,
			ExpiresAt:  a.ExpiresAt.Format(time.RFC3339),
			Status:     a.Status,
		})
	}
	return out
}

// actionDisplayType reports the concrete case type instead of the generic
// submit_service_request action type.
func actionDisplayType(a domain.PendingAction) string {
	if a.ActionType != domain.ActionSubmitServiceRequest {
		return a.ActionType
	}
	var payload struct {
		CaseType string `json:"case_type"`
	}
	if err := json.Unmarshal([]byte(a.PayloadJSON), &payload); err == nil && payload.CaseType != "" {
		return payload.CaseType
	}
	return a.ActionType
}

// hashText is a stable non-cryptographic fingerprint of the client message text.
func hashText(s string) string {
	var h uint32 = 2166136261
	for _, r := range s {
		h ^= uint32(r)
		h *= 16777619
	}
	return strings.ToUpper(strconvFormat(h))
}

func strconvFormat(v uint32) string {
	const digits = "0123456789abcdef"
	if v == 0 {
		return "0"
	}
	buf := make([]byte, 0, 8)
	for v > 0 {
		buf = append(buf, digits[v&0xf])
		v >>= 4
	}
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}

// PendingActions returns the pending proposals of a conversation, newest first.
func (s *Service) PendingActions(ctx context.Context, customerID, conversationID string) ([]PendingActionView, error) {
	if _, err := s.owned(ctx, customerID, conversationID); err != nil {
		return nil, err
	}
	actions, err := s.store.ListPendingActions(ctx, conversationID, domain.ActionPending, 10)
	if err != nil {
		return nil, domain.AsAppError(err)
	}
	return pendingViews(actions), nil
}
