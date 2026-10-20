package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/example/ecommerce-support/internal/conversation"
	"github.com/example/ecommerce-support/internal/domain"
)

type createConversationRequest struct {
	Channel string `json:"channel"`
}

type sendMessageRequest struct {
	Message         string `json:"message"`
	ClientMessageID string `json:"client_message_id"`
}

func (s *Server) handleCreateConversation(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	var req createConversationRequest
	if r.ContentLength != 0 {
		if err := decodeJSON(w, r, &req); err != nil {
			writeError(w, r, s.logger, err)
			return
		}
	}
	conv, err := s.conv.Create(r.Context(), id.CustomerID, req.Channel)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	writeCreated(w, r, s.conversationDTO(r, conv, nil))
}

func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	limit, err := parseLimit(r, 20, 100)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	cursor, err := decodeCursor(r.URL.Query().Get("cursor"))
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	convs, err := s.conv.List(r.Context(), id.CustomerID, limit, cursor)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	views := make([]conversationView, 0, len(convs))
	for i := range convs {
		views = append(views, s.conversationDTO(r, &convs[i], nil))
	}
	next := ""
	if len(convs) == limit && len(convs) > 0 {
		last := convs[len(convs)-1]
		next = encodeCursor(last.CreatedAt, last.ID)
	}
	writeData(w, r, views, listMeta(limit, next))
}

func (s *Server) handleGetConversation(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	convID := r.PathValue("conversationID")
	conv, err := s.conv.Get(r.Context(), id.CustomerID, convID)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	pending, err := s.conv.PendingActions(r.Context(), id.CustomerID, convID)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	writeData(w, r, s.conversationDTO(r, conv, pending), nil)
}

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	limit, err := parseLimit(r, 50, 100)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	before, err := decodeCursor(r.URL.Query().Get("before"))
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	msgs, err := s.conv.Messages(r.Context(), id.CustomerID, r.PathValue("conversationID"), limit, before)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	next := ""
	if len(msgs) == limit && len(msgs) > 0 {
		oldest := msgs[0]
		next = encodeCursor(parseRFC3339(oldest.CreatedAt), oldest.ID)
	}
	writeData(w, r, msgs, listMeta(limit, next))
}

func (s *Server) handleSendMessage(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	var req sendMessageRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	turn, err := s.conv.SendMessage(r.Context(), conversation.SendInput{
		CustomerID:      id.CustomerID,
		ConversationID:  r.PathValue("conversationID"),
		Text:            req.Message,
		ClientMessageID: req.ClientMessageID,
		RequestID:       requestIDFrom(r.Context()),
	})
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	writeData(w, r, turn, nil)
}

func (s *Server) handleCloseConversation(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	conv, err := s.conv.Close(r.Context(), id.CustomerID, r.PathValue("conversationID"))
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	writeData(w, r, s.conversationDTO(r, conv, nil), nil)
}

// conversationDTO maps a conversation plus its pending actions.
func (s *Server) conversationDTO(r *http.Request, conv *domain.Conversation, pending []conversation.PendingActionView) conversationView {
	view := conversationView{
		ID:             conv.ID,
		ConversationNo: conv.ConversationNo,
		Status:         conv.Status,
		Channel:        conv.Channel,
		Summary:        conv.Summary,
		CreatedAt:      tstr(conv.CreatedAt),
		UpdatedAt:      tstr(conv.UpdatedAt),
		LastMessageAt:  tptr(conv.LastMessageAt),
	}
	if conv.ActiveTaskStateJSON != "" {
		state := map[string]any{}
		if err := json.Unmarshal([]byte(conv.ActiveTaskStateJSON), &state); err == nil {
			view.ActiveTask = state
		}
	}
	for _, p := range pending {
		view.PendingActions = append(view.PendingActions, pendingActionView{
			ActionID:   p.ActionID,
			ActionNo:   p.ActionNo,
			ActionType: p.ActionType,
			Status:     p.Status,
			Summary:    p.Summary,
			Preview:    p.Preview,
			ExpiresAt:  p.ExpiresAt,
		})
	}
	return view
}
