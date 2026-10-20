package httpapi

import (
	"net/http"

	"github.com/example/ecommerce-support/internal/domain"
)

// handleConversationTrace exposes tool calls and audit records of one conversation.
// Registered only when APP_ENV is dev/test AND DEV_API_ENABLED=true.
func (s *Server) handleConversationTrace(w http.ResponseWriter, r *http.Request) {
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
	executions, err := s.store.ListToolExecutions(r.Context(), convID, 100)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	audits, err := s.store.ListAuditLogs(r.Context(), convID, 50)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	trace := make([]map[string]any, 0, len(executions))
	for _, e := range executions {
		trace = append(trace, map[string]any{
			"tool_name":     e.ToolName,
			"tool_call_id":  e.ToolCallID,
			"status":        e.Status,
			"input":         e.InputJSON,
			"output":        e.OutputJSON,
			"error_code":    e.ErrorCode,
			"error_message": e.ErrorMessageSafe,
			"latency_ms":    e.LatencyMS,
			"created_at":    tstr(e.CreatedAt),
			"finished_at":   tptr(e.FinishedAt),
		})
	}
	auditViews := make([]map[string]any, 0, len(audits))
	for _, a := range audits {
		auditViews = append(auditViews, map[string]any{
			"action_type": a.ActionType,
			"target_type": a.TargetType,
			"target_id":   a.TargetID,
			"actor_type":  a.ActorType,
			"result":      a.Result,
			"reason":      a.Reason,
			"before":      a.BeforeJSON,
			"after":       a.AfterJSON,
			"created_at":  tstr(a.CreatedAt),
		})
	}
	writeData(w, r, map[string]any{
		"conversation":    s.conversationDTO(r, conv, nil),
		"tool_executions": trace,
		"audit_logs":      auditViews,
	}, nil)
}

// handleToolExecutions lists tool traces of one conversation (dev only).
func (s *Server) handleToolExecutions(w http.ResponseWriter, r *http.Request) {
	id, ok := s.requireIdentity(w, r)
	if !ok {
		return
	}
	convID := r.URL.Query().Get("conversation_id")
	if convID == "" {
		writeError(w, r, s.logger, domain.ValidationError("conversation_id 查询参数必填"))
		return
	}
	if _, err := s.conv.Get(r.Context(), id.CustomerID, convID); err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	limit, err := parseLimit(r, 50, 200)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	executions, err := s.store.ListToolExecutions(r.Context(), convID, limit)
	if err != nil {
		writeError(w, r, s.logger, err)
		return
	}
	writeData(w, r, executions, map[string]any{"limit": limit, "count": len(executions)})
}
