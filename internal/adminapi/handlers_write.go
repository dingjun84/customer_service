package adminapi

import (
	"net/http"

	"github.com/example/ecommerce-support/internal/admin"
	"github.com/example/ecommerce-support/internal/domain"
)

type voidActionRequest struct {
	Reason string `json:"reason"`
}

type knowledgeStatusRequest struct {
	Status string `json:"status"`
}

type noteRequest struct {
	Note string `json:"note"`
}

// handleUpdateTicket moves a ticket through its lifecycle and/or reassigns it.
func (s *Server) handleUpdateTicket(w http.ResponseWriter, r *http.Request) {
	var in admin.TicketUpdate
	if err := decodeJSON(w, r, &in); err != nil {
		s.writeError(w, r, err)
		return
	}
	session := mustSession(r)
	ticket, err := s.svc.UpdateTicket(r.Context(), r.PathValue("ticketNo"), in,
		session.User.ID, session.User.DisplayName)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeData(w, ticket, nil)
}

// handleAddTicketNote appends a handling note without touching the status.
func (s *Server) handleAddTicketNote(w http.ResponseWriter, r *http.Request) {
	var in noteRequest
	if err := decodeJSON(w, r, &in); err != nil {
		s.writeError(w, r, err)
		return
	}
	session := mustSession(r)
	if err := s.svc.AddTicketNote(r.Context(), r.PathValue("ticketNo"), in.Note,
		session.User.ID, session.User.DisplayName); err != nil {
		s.writeError(w, r, err)
		return
	}
	detail, err := s.svc.TicketDetail(r.Context(), r.PathValue("ticketNo"))
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeData(w, detail, nil)
}

// handleVoidPendingAction expires a proposal. There is deliberately no
// "confirm on behalf of the customer" endpoint.
func (s *Server) handleVoidPendingAction(w http.ResponseWriter, r *http.Request) {
	var in voidActionRequest
	if r.ContentLength > 0 {
		if err := decodeJSON(w, r, &in); err != nil {
			s.writeError(w, r, err)
			return
		}
	}
	session := mustSession(r)
	status, err := s.svc.VoidPendingAction(r.Context(), r.PathValue("actionID"),
		session.User.ID, session.User.DisplayName, in.Reason)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeData(w, map[string]any{"status": status}, nil)
}

// handleCreateKnowledge creates a draft or published article.
func (s *Server) handleCreateKnowledge(w http.ResponseWriter, r *http.Request) {
	var in admin.KnowledgeInput
	if err := decodeJSON(w, r, &in); err != nil {
		s.writeError(w, r, err)
		return
	}
	session := mustSession(r)
	detail, err := s.svc.CreateKnowledge(r.Context(), in, session.User.ID, session.User.DisplayName)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeData(w, detail, nil)
}

// handleUpdateKnowledge rewrites an article body and bumps its version.
func (s *Server) handleUpdateKnowledge(w http.ResponseWriter, r *http.Request) {
	var in admin.KnowledgeInput
	if err := decodeJSON(w, r, &in); err != nil {
		s.writeError(w, r, err)
		return
	}
	session := mustSession(r)
	detail, err := s.svc.UpdateKnowledge(r.Context(), r.PathValue("articleID"), in,
		session.User.ID, session.User.DisplayName)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeData(w, detail, nil)
}

// handleKnowledgeStatus publishes, archives or reverts an article.
func (s *Server) handleKnowledgeStatus(w http.ResponseWriter, r *http.Request) {
	var in knowledgeStatusRequest
	if err := decodeJSON(w, r, &in); err != nil {
		s.writeError(w, r, err)
		return
	}
	if in.Status == "" {
		s.writeError(w, r, domain.ValidationError("status 不能为空"))
		return
	}
	session := mustSession(r)
	detail, err := s.svc.SetKnowledgeStatus(r.Context(), r.PathValue("articleID"), in.Status,
		session.User.ID, session.User.DisplayName)
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	writeData(w, detail, nil)
}
