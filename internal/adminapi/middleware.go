package adminapi

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/example/ecommerce-support/internal/admin"
	"github.com/example/ecommerce-support/internal/domain"
)

// requireSession gates every console endpoint except login.
//
// The cookie is HttpOnly and SameSite=Lax, and state-changing requests carrying
// an Origin header must match the host they arrived on. Together those two checks
// cover the CSRF surface a same-origin console has without needing a token dance.
func (s *Server) requireSession(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if !sameOrigin(r) {
				s.writeError(w, r, domain.ForbiddenError("跨站请求已被拒绝"))
				return
			}
		}

		cookie, err := r.Cookie(SessionCookie)
		if err != nil {
			s.writeError(w, r, domain.NewError(domain.CodeUnauthorized, "请先登录"))
			return
		}
		session, err := s.svc.Auth.ResolveSession(r.Context(), cookie.Value)
		if err != nil {
			s.clearCookie(w)
			s.writeError(w, r, err)
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeySession, session)
		next(w, r.WithContext(ctx))
	}
}

// sessionFrom returns the session injected by requireSession.
func sessionFrom(ctx context.Context) (admin.Session, bool) {
	v, ok := ctx.Value(ctxKeySession).(admin.Session)
	return v, ok
}

// mustSession is used inside gated handlers, where the middleware has already
// proven a session exists.
func mustSession(r *http.Request) admin.Session {
	session, _ := sessionFrom(r.Context())
	return session
}

func sameOrigin(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		// Non-browser clients (curl, the smoke test) send no Origin. They also
		// cannot be tricked into attaching a victim's cookie.
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

func (s *Server) setCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   !s.cfg.IsDev(),
		MaxAge:   maxAge,
	})
}

func (s *Server) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   !s.cfg.IsDev(),
		MaxAge:   -1,
	})
}
