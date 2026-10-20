package adminapi_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/example/ecommerce-support/internal/admin"
	"github.com/example/ecommerce-support/internal/adminapi"
	"github.com/example/ecommerce-support/internal/config"
	"github.com/example/ecommerce-support/internal/database"
	"github.com/example/ecommerce-support/internal/seed"
)

const testPassword = "test-password-1"

type harness struct {
	t      *testing.T
	db     *sql.DB
	server *httptest.Server
	client *http.Client
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()

	db, err := database.Open(ctx, filepath.Join(t.TempDir(), "admin.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if err := database.Migrate(ctx, db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := seed.New(db, nil).Run(ctx, false); err != nil {
		t.Fatalf("seed: %v", err)
	}

	auth := admin.NewAuth(db, time.Hour)
	if _, err := auth.BootstrapUser(ctx, "admin", testPassword, "系统管理员"); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}

	svc := admin.NewService(admin.NewStore(db), auth, admin.Metadata{
		SchemaVersion: 2, AppEnv: "test", LLMMode: "disabled",
		ExposeDBPath: true, SessionTTL: time.Hour,
	})
	cfg := config.Config{AppEnv: "test", AdminSessionTTL: time.Hour}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := adminapi.NewServer(svc, cfg, logger)
	mux := http.NewServeMux()
	srv.Register(mux)

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookie jar: %v", err)
	}
	return &harness{t: t, db: db, server: ts, client: &http.Client{Jar: jar, Timeout: 10 * time.Second}}
}

// do sends a request and decodes the envelope. body may be nil.
func (h *harness) do(method, path string, body any) (int, map[string]any) {
	h.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			h.t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, h.server.URL+path, reader)
	if err != nil {
		h.t.Fatalf("new request: %v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := h.client.Do(req)
	if err != nil {
		h.t.Fatalf("do request: %v", err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)

	var envelope map[string]any
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &envelope)
	}
	return res.StatusCode, envelope
}

func (h *harness) login() {
	h.t.Helper()
	code, env := h.do(http.MethodPost, "/api/admin/v1/auth/login",
		map[string]any{"username": "admin", "password": testPassword})
	if code != http.StatusOK {
		h.t.Fatalf("login = %d, body = %v", code, env)
	}
}

func dataOf(t *testing.T, env map[string]any) any {
	t.Helper()
	data, ok := env["data"]
	if !ok {
		t.Fatalf("envelope has no data: %v", env)
	}
	return data
}

func metaOf(t *testing.T, env map[string]any) map[string]any {
	t.Helper()
	meta, _ := env["meta"].(map[string]any)
	return meta
}

// ---------------------------------------------------------------- auth tests

func TestUnauthenticatedRequestsAreRejected(t *testing.T) {
	h := newHarness(t)
	for _, path := range []string{"/api/admin/v1/overview", "/api/admin/v1/conversations", "/api/admin/v1/health"} {
		code, env := h.do(http.MethodGet, path, nil)
		if code != http.StatusUnauthorized {
			t.Fatalf("%s = %d, want 401", path, code)
		}
		errObj, _ := env["error"].(map[string]any)
		if errObj["code"] != "unauthorized" {
			t.Fatalf("%s error code = %v", path, errObj["code"])
		}
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	h := newHarness(t)
	code, env := h.do(http.MethodPost, "/api/admin/v1/auth/login",
		map[string]any{"username": "admin", "password": "wrong-password"})
	if code != http.StatusUnauthorized {
		t.Fatalf("login = %d, want 401, body = %v", code, env)
	}
}

func TestLoginThrottlesRepeatedFailures(t *testing.T) {
	h := newHarness(t)
	var last int
	for i := 0; i < 6; i++ {
		last, _ = h.do(http.MethodPost, "/api/admin/v1/auth/login",
			map[string]any{"username": "admin", "password": "nope-nope-nope"})
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("6th attempt = %d, want 429", last)
	}
}

func TestLoginAndLogoutLifecycle(t *testing.T) {
	h := newHarness(t)
	code, env := h.do(http.MethodGet, "/api/admin/v1/auth/mode", nil)
	if code != http.StatusOK {
		t.Fatalf("mode = %d", code)
	}
	mode := dataOf(t, env).(map[string]any)
	if mode["dev"] != true {
		t.Fatalf("mode.dev = %v, want true in test env", mode["dev"])
	}

	h.login()

	code, env = h.do(http.MethodGet, "/api/admin/v1/auth/me", nil)
	if code != http.StatusOK {
		t.Fatalf("me = %d", code)
	}
	me := dataOf(t, env).(map[string]any)
	user := me["user"].(map[string]any)
	if user["username"] != "admin" || user["role"] != "admin" {
		t.Fatalf("me.user = %v", user)
	}

	if code, _ := h.do(http.MethodPost, "/api/admin/v1/auth/logout", map[string]any{}); code != http.StatusOK {
		t.Fatalf("logout = %d", code)
	}
	if code, _ := h.do(http.MethodGet, "/api/admin/v1/auth/me", nil); code != http.StatusUnauthorized {
		t.Fatalf("me after logout = %d, want 401", code)
	}
}

// ---------------------------------------------------------------- read tests

func TestEveryReadEndpointResponds(t *testing.T) {
	h := newHarness(t)
	h.login()

	cases := []string{
		"/api/admin/v1/overview",
		"/api/admin/v1/health",
		"/api/admin/v1/conversations",
		"/api/admin/v1/tickets",
		"/api/admin/v1/pending-actions",
		"/api/admin/v1/audit-logs",
		"/api/admin/v1/tool-executions",
		"/api/admin/v1/analytics/tools",
		"/api/admin/v1/analytics/intents",
		"/api/admin/v1/knowledge",
		"/api/admin/v1/knowledge/categories",
		"/api/admin/v1/customers",
		"/api/admin/v1/orders",
		"/api/admin/v1/products",
		"/api/admin/v1/service-cases",
		"/api/admin/v1/refunds",
		"/api/admin/v1/invoices",
		"/api/admin/v1/promotions",
	}
	for _, path := range cases {
		code, env := h.do(http.MethodGet, path, nil)
		if code != http.StatusOK {
			t.Fatalf("%s = %d, body = %v", path, code, env)
		}
		if env["data"] == nil {
			t.Fatalf("%s returned no data", path)
		}
	}
}

func TestOverviewCarriesMetricsAndHealth(t *testing.T) {
	h := newHarness(t)
	h.login()

	_, env := h.do(http.MethodGet, "/api/admin/v1/overview", nil)
	ov := dataOf(t, env).(map[string]any)

	metrics, _ := ov["metrics"].([]any)
	if len(metrics) == 0 {
		t.Fatal("overview has no metrics")
	}
	health, _ := ov["health"].(map[string]any)
	if health == nil {
		t.Fatal("overview has no health block")
	}
	tables, _ := health["tables"].([]any)
	if len(tables) < 20 {
		t.Fatalf("health tables = %d, want at least 20", len(tables))
	}
	activity, _ := ov["activity"].([]any)
	if len(activity) != 14 {
		t.Fatalf("activity points = %d, want 14", len(activity))
	}
}

func TestConversationDetailIncludesToolTrail(t *testing.T) {
	h := newHarness(t)
	h.login()

	_, env := h.do(http.MethodGet, "/api/admin/v1/conversations?page_size=1", nil)
	items := dataOf(t, env).([]any)
	if len(items) == 0 {
		t.Skip("seed produced no conversations")
	}
	id := items[0].(map[string]any)["id"].(string)

	code, env := h.do(http.MethodGet, "/api/admin/v1/conversations/"+url.PathEscape(id), nil)
	if code != http.StatusOK {
		t.Fatalf("detail = %d", code)
	}
	detail := dataOf(t, env).(map[string]any)
	if detail["conversation"] == nil {
		t.Fatal("detail missing conversation")
	}
	msgs, _ := detail["messages"].([]any)
	if len(msgs) == 0 {
		t.Fatal("detail has no messages")
	}
}

func TestPaginationRejectsOutOfRangePageSize(t *testing.T) {
	h := newHarness(t)
	h.login()
	code, env := h.do(http.MethodGet, "/api/admin/v1/conversations?page_size=101", nil)
	if code != http.StatusBadRequest {
		t.Fatalf("page_size=101 = %d, want 400, body = %v", code, env)
	}
}

// -------------------------------------------------------------- write tests

func TestTicketStatusTransitionIsValidated(t *testing.T) {
	h := newHarness(t)
	h.login()

	_, env := h.do(http.MethodGet, "/api/admin/v1/tickets?page_size=50", nil)
	items := dataOf(t, env).([]any)

	var openTicket string
	for _, raw := range items {
		tk := raw.(map[string]any)
		if tk["status"] == "open" {
			openTicket = tk["ticket_no"].(string)
			break
		}
	}
	if openTicket == "" {
		t.Skip("seed has no open ticket")
	}

	code, env := h.do(http.MethodPost, "/api/admin/v1/tickets/"+url.PathEscape(openTicket),
		map[string]any{"status": "assigned", "note": "已接管，正在核实物流"})
	if code != http.StatusOK {
		t.Fatalf("assign = %d, body = %v", code, env)
	}
	updated := dataOf(t, env).(map[string]any)
	if updated["status"] != "assigned" {
		t.Fatalf("status = %v, want assigned", updated["status"])
	}
	if updated["assigned_to"] == "" {
		t.Fatal("assigning a ticket must record the operator")
	}

	// "submitted" is not a ticket status at all: that is a validation failure.
	code, _ = h.do(http.MethodPost, "/api/admin/v1/tickets/"+url.PathEscape(openTicket),
		map[string]any{"status": "submitted"})
	if code != http.StatusBadRequest {
		t.Fatalf("unknown status = %d, want 400", code)
	}

	// Closing then reopening is allowed, but closed -> resolved is not.
	if code, env := h.do(http.MethodPost, "/api/admin/v1/tickets/"+url.PathEscape(openTicket),
		map[string]any{"status": "closed", "note": "客户确认解决"}); code != http.StatusOK {
		t.Fatalf("close = %d, body = %v", code, env)
	}
	code, _ = h.do(http.MethodPost, "/api/admin/v1/tickets/"+url.PathEscape(openTicket),
		map[string]any{"status": "resolved"})
	if code != http.StatusConflict {
		t.Fatalf("closed -> resolved = %d, want 409", code)
	}

	// The note must show up in the ticket detail trail.
	code, env = h.do(http.MethodGet, "/api/admin/v1/tickets/"+url.PathEscape(openTicket), nil)
	if code != http.StatusOK {
		t.Fatalf("detail = %d", code)
	}
	detail := dataOf(t, env).(map[string]any)
	notes, _ := detail["notes"].([]any)
	if len(notes) == 0 {
		t.Fatal("status change did not write a note")
	}
}

func TestTicketUpdateWritesAuditLog(t *testing.T) {
	h := newHarness(t)
	h.login()

	_, env := h.do(http.MethodGet, "/api/admin/v1/tickets?status=open&page_size=1", nil)
	items := dataOf(t, env).([]any)
	if len(items) == 0 {
		t.Skip("seed has no open ticket")
	}
	ticket := items[0].(map[string]any)
	ticketID := ticket["id"].(string)

	if code, env := h.do(http.MethodPost, "/api/admin/v1/tickets/"+url.PathEscape(ticket["ticket_no"].(string)),
		map[string]any{"priority": "urgent", "note": "客户升级投诉"}); code != http.StatusOK {
		t.Fatalf("update = %d, body = %v", code, env)
	}

	var count int
	err := h.db.QueryRow(`SELECT COUNT(*) FROM action_audit_logs
		WHERE target_type = 'service_ticket' AND target_id = ? AND actor_type = 'admin'`, ticketID).Scan(&count)
	if err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if count == 0 {
		t.Fatal("ticket update left no admin audit record")
	}
}

func TestKnowledgeCRUD(t *testing.T) {
	h := newHarness(t)
	h.login()

	code, env := h.do(http.MethodPost, "/api/admin/v1/knowledge", map[string]any{
		"category": "logistics",
		"title":    "偏远地区配送时效说明",
		"content":  "偏远地区默认 5-7 天送达，节假日顺延。",
		"keywords": []string{"偏远", "时效", "配送"},
		"source":   "admin_console",
		"status":   "draft",
	})
	if code != http.StatusOK {
		t.Fatalf("create = %d, body = %v", code, env)
	}
	created := dataOf(t, env).(map[string]any)
	id := created["id"].(string)
	if created["article_code"] == "" || created["version"] != float64(1) {
		t.Fatalf("created = %v", created)
	}

	code, env = h.do(http.MethodPut, "/api/admin/v1/knowledge/"+url.PathEscape(id), map[string]any{
		"category": "logistics",
		"title":    "偏远地区配送时效说明（修订）",
		"content":  "偏远地区默认 5-7 个工作日送达，节假日顺延。",
		"keywords": []string{"偏远", "时效"},
		"source":   "admin_console",
		"status":   "draft",
	})
	if code != http.StatusOK {
		t.Fatalf("update = %d, body = %v", code, env)
	}
	updated := dataOf(t, env).(map[string]any)
	if updated["version"] != float64(2) {
		t.Fatalf("version = %v, want 2", updated["version"])
	}

	if code, env := h.do(http.MethodPost, "/api/admin/v1/knowledge/"+url.PathEscape(id)+"/status",
		map[string]any{"status": "published"}); code != http.StatusOK {
		t.Fatalf("publish = %d, body = %v", code, env)
	}

	code, env = h.do(http.MethodGet, "/api/admin/v1/knowledge?q=偏远地区", nil)
	if code != http.StatusOK {
		t.Fatalf("search = %d", code)
	}
	if len(dataOf(t, env).([]any)) == 0 {
		t.Fatal("published article is not searchable")
	}

	// Validation: an empty title must be rejected, not silently saved.
	if code, _ := h.do(http.MethodPost, "/api/admin/v1/knowledge", map[string]any{
		"category": "logistics", "title": "  ", "content": "x", "keywords": []string{},
	}); code != http.StatusBadRequest {
		t.Fatalf("empty title = %d, want 400", code)
	}
}

func TestVoidPendingActionNeverConfirms(t *testing.T) {
	h := newHarness(t)
	h.login()

	_, env := h.do(http.MethodGet, "/api/admin/v1/pending-actions?status=pending&page_size=1", nil)
	items := dataOf(t, env).([]any)
	if len(items) == 0 {
		t.Skip("seed has no pending action")
	}
	action := items[0].(map[string]any)
	id := action["id"].(string)

	code, env := h.do(http.MethodPost, "/api/admin/v1/pending-actions/"+url.PathEscape(id)+"/void",
		map[string]any{"reason": "重复提交"})
	if code != http.StatusOK {
		t.Fatalf("void = %d, body = %v", code, env)
	}
	result := dataOf(t, env).(map[string]any)
	if result["status"] != "expired" {
		t.Fatalf("status = %v, want expired", result["status"])
	}

	var status string
	if err := h.db.QueryRow(`SELECT status FROM pending_actions WHERE id = ?`, id).Scan(&status); err != nil {
		t.Fatalf("read status: %v", err)
	}
	if status != "expired" {
		t.Fatalf("stored status = %s, want expired", status)
	}

	// Voiding again is a no-op, not an error.
	if code, _ := h.do(http.MethodPost, "/api/admin/v1/pending-actions/"+url.PathEscape(id)+"/void",
		map[string]any{}); code != http.StatusOK {
		t.Fatalf("second void = %d, want 200", code)
	}
}

func TestUnknownTicketReturns404(t *testing.T) {
	h := newHarness(t)
	h.login()
	code, env := h.do(http.MethodGet, "/api/admin/v1/tickets/TKT-00000000-ZZZZZZ", nil)
	if code != http.StatusNotFound {
		t.Fatalf("unknown ticket = %d, want 404, body = %v", code, env)
	}
}

func TestCrossSiteWriteIsRejected(t *testing.T) {
	h := newHarness(t)
	h.login()

	req, _ := http.NewRequest(http.MethodPost, h.server.URL+"/api/admin/v1/tickets/X", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example.com")
	res, err := h.client.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-site write = %d, want 403", res.StatusCode)
	}
}
