package adminapi

import (
	"net"
	"net/http"
	"time"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	User            any    `json:"user"`
	ExpiresAt       string `json:"expires_at"`
	SessionTTLHours int    `json:"session_ttl_hours"`
}

// handleLogin exchanges credentials for a session cookie.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in loginRequest
	if err := decodeJSON(w, r, &in); err != nil {
		s.writeError(w, r, err)
		return
	}
	user, token, expires, err := s.svc.Auth.Login(r.Context(), in.Username, in.Password,
		clientIP(r), r.UserAgent())
	if err != nil {
		s.writeError(w, r, err)
		return
	}
	ttl := int(time.Until(expires).Seconds())
	s.setCookie(w, token, ttl)
	writeData(w, loginResponse{
		User:            user,
		ExpiresAt:       expires.UTC().Format(time.RFC3339),
		SessionTTLHours: int(expires.Sub(time.Now()).Hours()),
	}, nil)
}

// handleLogout revokes the session and clears the cookie.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(SessionCookie); err == nil {
		if err := s.svc.Auth.Logout(r.Context(), cookie.Value); err != nil {
			s.writeError(w, r, err)
			return
		}
	}
	s.clearCookie(w)
	writeData(w, map[string]any{"status": "logged_out"}, nil)
}

// handleAuthMode is the one unauthenticated endpoint besides login. The console
// needs it before it has a session, to decide whether to show the built-in
// development credentials hint. It exposes nothing but the environment name.
func (s *Server) handleAuthMode(w http.ResponseWriter, r *http.Request) {
	writeData(w, map[string]any{
		"app_env":    s.cfg.AppEnv,
		"dev":        s.cfg.IsDev(),
		"login_path": "/admin/",
	}, nil)
}

// handleMe returns the caller and the facts the console header displays.
func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	session := mustSession(r)
	meta := s.svc.Metadata()
	writeData(w, map[string]any{
		"user":               session.User,
		"session_expires_at": session.ExpiresAt.UTC().Format(time.RFC3339),
		"meta": map[string]any{
			"app_env":             meta.AppEnv,
			"llm_mode":            meta.LLMMode,
			"schema_version":      meta.SchemaVersion,
			"session_ttl_seconds": int(meta.SessionTTL.Seconds()),
			"dev":                 s.cfg.IsDev(),
		},
	}, nil)
}

// clientIP strips the port so the throttle key is stable across connections.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
