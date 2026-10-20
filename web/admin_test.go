package web_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/example/ecommerce-support/web"
)

// The console is embedded; a missing file only shows up as a 404 at runtime, so
// pin the document, its two assets and the redirect here.
func TestAdminHandlerServesDocumentAndAssets(t *testing.T) {
	handler := web.AdminHandler(http.NotFoundHandler())

	cases := []struct {
		path        string
		wantStatus  int
		wantType    string
		wantSnippet string
	}{
		{"/admin/", http.StatusOK, "text/html", "admin.js"},
		{"/admin", http.StatusMovedPermanently, "", ""},
		{"/admin/admin.js", http.StatusOK, "javascript", "renderOverview"},
		{"/admin/admin.css", http.StatusOK, "text/css", "--sidebar-w"},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, c.path, nil)
		handler.ServeHTTP(rec, req)

		if rec.Code != c.wantStatus {
			t.Fatalf("%s = %d, want %d", c.path, rec.Code, c.wantStatus)
		}
		if c.wantType != "" && !strings.Contains(rec.Header().Get("Content-Type"), c.wantType) {
			t.Fatalf("%s content-type = %q, want %q", c.path, rec.Header().Get("Content-Type"), c.wantType)
		}
		if c.wantSnippet != "" && !strings.Contains(rec.Body.String(), c.wantSnippet) {
			t.Fatalf("%s body does not contain %q", c.path, c.wantSnippet)
		}
	}
}

func TestAdminHandlerKeepsAPI404AsJSON(t *testing.T) {
	handler := web.AdminHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"not_found"}}`))
	}))

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/v1/nope", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "not_found") {
		t.Fatalf("body = %s, want the JSON envelope", rec.Body.String())
	}
}

func TestAdminHandlerRejectsWrites(t *testing.T) {
	handler := web.AdminHandler(http.NotFoundHandler())
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/admin/", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST /admin/ = %d, want 405", rec.Code)
	}
}
