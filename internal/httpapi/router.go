package httpapi

import (
	"context"
	"database/sql"
	"encoding/base64"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/example/ecommerce-support/internal/config"
	"github.com/example/ecommerce-support/internal/conversation"
	"github.com/example/ecommerce-support/internal/database"
	"github.com/example/ecommerce-support/internal/domain"
	"github.com/example/ecommerce-support/internal/repository"
	"github.com/example/ecommerce-support/internal/service"
	"github.com/example/ecommerce-support/web"
)

// Server holds the HTTP dependencies.
type Server struct {
	cfg       config.Config
	db        *sql.DB
	store     repository.Store
	svc       *service.Services
	conv      *conversation.Service
	logger    *slog.Logger
	startedAt time.Time
}

// NewServer builds the HTTP server.
func NewServer(cfg config.Config, db *sql.DB, store repository.Store, svc *service.Services,
	conv *conversation.Service, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	return &Server{
		cfg: cfg, db: db, store: store, svc: svc, conv: conv,
		logger: logger, startedAt: time.Now(),
	}
}

// Handler builds the middleware chain and the route table.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)

	// Conversations and messages.
	mux.HandleFunc("POST /api/v1/conversations", s.handleCreateConversation)
	mux.HandleFunc("GET /api/v1/conversations", s.handleListConversations)
	mux.HandleFunc("GET /api/v1/conversations/{conversationID}", s.handleGetConversation)
	mux.HandleFunc("GET /api/v1/conversations/{conversationID}/messages", s.handleListMessages)
	mux.HandleFunc("POST /api/v1/conversations/{conversationID}/messages", s.handleSendMessage)
	mux.HandleFunc("POST /api/v1/conversations/{conversationID}/close", s.handleCloseConversation)
	mux.HandleFunc("POST /api/v1/conversations/{conversationID}/actions/{actionID}/confirm", s.handleConfirmAction)
	mux.HandleFunc("POST /api/v1/conversations/{conversationID}/actions/{actionID}/reject", s.handleRejectAction)

	// Read-only business APIs.
	mux.HandleFunc("GET /api/v1/products", s.handleListProducts)
	mux.HandleFunc("GET /api/v1/products/{productID}", s.handleGetProduct)
	mux.HandleFunc("GET /api/v1/orders", s.handleListOrders)
	mux.HandleFunc("GET /api/v1/orders/{orderNo}", s.handleGetOrder)
	mux.HandleFunc("GET /api/v1/orders/{orderNo}/shipments", s.handleOrderShipments)
	mux.HandleFunc("GET /api/v1/orders/{orderNo}/service-cases", s.handleOrderServiceCases)
	mux.HandleFunc("GET /api/v1/service-cases/{caseNo}", s.handleGetServiceCase)
	mux.HandleFunc("GET /api/v1/invoices", s.handleListInvoices)

	// Dev-only diagnostics: never registered outside dev/test.
	if s.cfg.DevAPIAllowed() {
		mux.HandleFunc("GET /api/v1/dev/conversations/{conversationID}/trace", s.handleConversationTrace)
		mux.HandleFunc("GET /api/v1/dev/tool-executions", s.handleToolExecutions)
	}

	// Everything else: the browser console on the same origin, with JSON 404s
	// preserved for unknown /api paths.
	notFound := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeError(w, r, s.logger, domain.NotFoundError("接口不存在"))
	})
	mux.Handle("/", web.Handler(notFound))

	return s.withRecovery(s.withRequestID(s.withAccessLog(s.withIdentity(mux))))
}

// ---- health ----

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeData(w, r, map[string]any{
		"status":         "ok",
		"app_env":        s.cfg.AppEnv,
		"uptime_seconds": int64(time.Since(s.startedAt).Seconds()),
	}, nil)
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	if err := s.store.Ping(ctx); err != nil {
		writeReadyFailure(w, r, "database_unavailable")
		return
	}
	version, err := database.SchemaVersion(ctx, s.db)
	if err != nil {
		writeReadyFailure(w, r, "migration_status_unknown")
		return
	}
	writeData(w, r, map[string]any{
		"status":         "ready",
		"schema_version": version,
		"llm_mode":       s.cfg.LLMMode(),
		"llm_enabled":    s.cfg.LLMEnabled,
		"llm_configured": s.cfg.LLMEnabled && s.cfg.LLMBaseURL != "" && s.cfg.LLMModel != "",
		"dev_api":        s.cfg.DevAPIAllowed(),
	}, nil)
}

func writeReadyFailure(w http.ResponseWriter, r *http.Request, reason string) {
	writeJSON(w, http.StatusServiceUnavailable, errorEnvelope{Error: errorDetail{
		Code:      "not_ready",
		Message:   "服务尚未就绪",
		RequestID: requestIDFrom(r.Context()),
		Details:   map[string]any{"reason": reason},
	}})
}

// ---- cursor helpers ----

func encodeCursor(t time.Time, id string) string {
	raw := t.UTC().Format(time.RFC3339) + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(raw string) (*repository.Cursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, domain.ValidationError("cursor 不合法")
	}
	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 {
		return nil, domain.ValidationError("cursor 不合法")
	}
	t, err := time.Parse(time.RFC3339, parts[0])
	if err != nil {
		return nil, domain.ValidationError("cursor 不合法")
	}
	return &repository.Cursor{CreatedAt: t.UTC(), ID: parts[1]}, nil
}

func listMeta(limit int, next string) map[string]any {
	meta := map[string]any{"limit": limit}
	if next != "" {
		meta["next_cursor"] = next
	}
	return meta
}
