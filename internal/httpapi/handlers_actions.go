package httpapi

import (
	"net/http"
)

// handleConfirmAction executes a pending action after re-validating everything.
// The response reports the real business result, never "confirmation received".
func (s *Server) handleConfirmAction(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	result, err := s.svc.Actions.Confirm(r.Context(), id.CustomerID,
		r.PathValue("conversationID"), r.PathValue("actionID"), requestIDFrom(r.Context()))
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	writeData(w, r, result, nil)
}

// handleRejectAction records an explicit rejection; business data stays untouched.
func (s *Server) handleRejectAction(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	result, err := s.svc.Actions.Reject(r.Context(), id.CustomerID,
		r.PathValue("conversationID"), r.PathValue("actionID"))
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	writeData(w, r, result, nil)
}
