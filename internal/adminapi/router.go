// Package adminapi exposes the operations console over HTTP.
//
// It is mounted under /api/admin/v1 and is authenticated by its own cookie
// session. The customer API's X-Demo-Customer-ID header plays no part here: staff
// identity and customer identity must never share a mechanism.
package adminapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/example/ecommerce-support/internal/admin"
	"github.com/example/ecommerce-support/internal/config"
	"github.com/example/ecommerce-support/internal/domain"
)

// SessionCookie is the console's session cookie name.
const SessionCookie = "wb_admin_session"

type ctxKey string

const ctxKeySession ctxKey = "admin_session"

// Server holds the console's HTTP dependencies.
type Server struct {
	svc    *admin.Service
	cfg    config.Config
	logger *slog.Logger
}

// NewServer builds the console HTTP layer.
func NewServer(svc *admin.Service, cfg config.Config, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{svc: svc, cfg: cfg, logger: logger}
}

// Register mounts every console route on the shared mux.
//
// The routes are registered on the same mux as the customer API so the console
// needs no second port. Auth is per-group rather than global because the login
// endpoints must stay reachable while everything else requires a session.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/admin/v1/auth/login", s.handleLogin)
	mux.HandleFunc("POST /api/admin/v1/auth/logout", s.handleLogout)
	mux.HandleFunc("GET /api/admin/v1/auth/mode", s.handleAuthMode)
	mux.HandleFunc("GET /api/admin/v1/auth/me", s.requireSession(s.handleMe))

	mux.HandleFunc("GET /api/admin/v1/overview", s.requireSession(s.handleOverview))
	mux.HandleFunc("GET /api/admin/v1/health", s.requireSession(s.handleHealth))

	mux.HandleFunc("GET /api/admin/v1/conversations", s.requireSession(s.handleConversations))
	mux.HandleFunc("GET /api/admin/v1/conversations/{conversationID}", s.requireSession(s.handleConversation))

	mux.HandleFunc("GET /api/admin/v1/tickets", s.requireSession(s.handleTickets))
	mux.HandleFunc("GET /api/admin/v1/tickets/{ticketNo}", s.requireSession(s.handleTicket))
	mux.HandleFunc("POST /api/admin/v1/tickets/{ticketNo}", s.requireSession(s.handleUpdateTicket))
	mux.HandleFunc("POST /api/admin/v1/tickets/{ticketNo}/notes", s.requireSession(s.handleAddTicketNote))

	mux.HandleFunc("GET /api/admin/v1/pending-actions", s.requireSession(s.handlePendingActions))
	mux.HandleFunc("GET /api/admin/v1/pending-actions/{actionID}", s.requireSession(s.handlePendingAction))
	mux.HandleFunc("POST /api/admin/v1/pending-actions/{actionID}/void", s.requireSession(s.handleVoidPendingAction))

	mux.HandleFunc("GET /api/admin/v1/audit-logs", s.requireSession(s.handleAuditLogs))
	mux.HandleFunc("GET /api/admin/v1/tool-executions", s.requireSession(s.handleToolExecutions))
	mux.HandleFunc("GET /api/admin/v1/analytics/tools", s.requireSession(s.handleToolStats))
	mux.HandleFunc("GET /api/admin/v1/analytics/intents", s.requireSession(s.handleIntentStats))

	mux.HandleFunc("GET /api/admin/v1/knowledge", s.requireSession(s.handleKnowledge))
	mux.HandleFunc("GET /api/admin/v1/knowledge/categories", s.requireSession(s.handleKnowledgeCategories))
	mux.HandleFunc("GET /api/admin/v1/knowledge/{articleID}", s.requireSession(s.handleKnowledgeDetail))
	mux.HandleFunc("POST /api/admin/v1/knowledge", s.requireSession(s.handleCreateKnowledge))
	mux.HandleFunc("PUT /api/admin/v1/knowledge/{articleID}", s.requireSession(s.handleUpdateKnowledge))
	mux.HandleFunc("POST /api/admin/v1/knowledge/{articleID}/status", s.requireSession(s.handleKnowledgeStatus))

	mux.HandleFunc("GET /api/admin/v1/customers", s.requireSession(s.handleCustomers))
	mux.HandleFunc("GET /api/admin/v1/customers/{customerNo}", s.requireSession(s.handleCustomerDetail))
	mux.HandleFunc("GET /api/admin/v1/orders", s.requireSession(s.handleOrders))
	mux.HandleFunc("GET /api/admin/v1/orders/{orderNo}", s.requireSession(s.handleOrderDetail))
	mux.HandleFunc("GET /api/admin/v1/products", s.requireSession(s.handleProducts))
	mux.HandleFunc("GET /api/admin/v1/products/{productID}", s.requireSession(s.handleProductDetail))
	mux.HandleFunc("GET /api/admin/v1/service-cases", s.requireSession(s.handleServiceCases))
	mux.HandleFunc("GET /api/admin/v1/refunds", s.requireSession(s.handleRefunds))
	mux.HandleFunc("GET /api/admin/v1/invoices", s.requireSession(s.handleInvoices))
	mux.HandleFunc("GET /api/admin/v1/promotions", s.requireSession(s.handlePromotions))
}

// ---------------------------------------------------------------------------
// Envelope
// ---------------------------------------------------------------------------

type successEnvelope struct {
	Data any `json:"data"`
	Meta any `json:"meta,omitempty"`
}

type errorEnvelope struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	RequestID string         `json:"request_id"`
	Details   map[string]any `json:"details,omitempty"`
}

func writeData(w http.ResponseWriter, data any, meta any) {
	writeJSON(w, http.StatusOK, successEnvelope{Data: data, Meta: meta})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(body)
}

// writeError maps a domain error onto the shared envelope. Console errors carry
// the request id so an operator can paste it into the server log.
func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	appErr := domain.AsAppError(err)
	status := domain.HTTPStatus(appErr.Code)
	if status >= 500 {
		s.logger.Error("admin request failed",
			"request_id", requestIDFrom(r),
			"method", r.Method, "path", r.URL.Path,
			"code", string(appErr.Code), "error", appErr.Error())
	}
	writeJSON(w, status, errorEnvelope{Error: errorDetail{
		Code:      string(appErr.Code),
		Message:   appErr.Message,
		RequestID: requestIDFrom(r),
		Details:   appErr.Details,
	}})
}

// ---------------------------------------------------------------------------
// Input helpers
// ---------------------------------------------------------------------------

func decodeJSON(w http.ResponseWriter, r *http.Request, dest any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		if strings.Contains(err.Error(), "unknown field") {
			return domain.ValidationError("请求体包含未知字段")
		}
		return domain.NewError(domain.CodeBadRequest, "请求体不是合法 JSON")
	}
	if dec.More() {
		return domain.NewError(domain.CodeBadRequest, "请求体包含多余内容")
	}
	return nil
}

// pageFrom reads page and page_size, rejecting values outside the supported
// window instead of silently clamping them: a console that quietly shows page 1
// when you asked for page 900 is worse than an error.
func pageFrom(r *http.Request) (admin.Page, error) {
	q := r.URL.Query()
	p := admin.Page{Page: 1, PageSize: 20}
	if raw := strings.TrimSpace(q.Get("page")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 {
			return p, domain.ValidationError("page 必须是正整数")
		}
		if v > 10000 {
			return p, domain.ValidationError("page 过大")
		}
		p.Page = v
	}
	if raw := strings.TrimSpace(q.Get("page_size")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 1 {
			return p, domain.ValidationError("page_size 必须是正整数")
		}
		if v > 100 {
			return p, domain.ValidationError("page_size 不能超过 100")
		}
		p.PageSize = v
	}
	return p, nil
}

func query(r *http.Request, key string) string {
	return strings.TrimSpace(r.URL.Query().Get(key))
}

// requestIDFrom reads the id the outer middleware normalised onto the request
// header. The console does not keep a private context key for it.
func requestIDFrom(r *http.Request) string {
	return r.Header.Get("X-Request-ID")
}
