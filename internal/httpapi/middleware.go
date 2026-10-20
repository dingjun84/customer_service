package httpapi

import (
	"context"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/example/ecommerce-support/internal/domain"
)

// withRecovery converts a panic into a 500 envelope without leaking a stack trace.
func (s *Server) withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.logger.Error("panic recovered",
					"request_id", requestIDFrom(r.Context()),
					"path", r.URL.Path,
					"stack", string(debug.Stack()))
				writeJSON(w, http.StatusInternalServerError, errorEnvelope{Error: errorDetail{
					Code:      string(domain.CodeInternal),
					Message:   "服务内部错误",
					RequestID: requestIDFrom(r.Context()),
				}})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// withRequestID propagates or generates X-Request-ID.
func (s *Server) withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if id == "" || len([]rune(id)) > 100 {
			id = "req_" + domain.NewID()
		}
		w.Header().Set("X-Request-ID", id)
		// Downstream handlers in other packages (the admin console) read the
		// normalised id from the request header rather than a private ctx key.
		r.Header.Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), ctxKeyRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// withAccessLog logs method, path, status and latency (never the body).
func (s *Server) withAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.logger.Info("http request",
			"request_id", requestIDFrom(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"latency_ms", time.Since(started).Milliseconds())
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

// withIdentity injects the customer identity for /api/v1 routes.
//
// The MVP only accepts X-Demo-Customer-ID in dev/test. In staging/prod this
// middleware refuses every request until a real authentication layer is added,
// which is deliberately fail-closed.
func (s *Server) withIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/v1/") {
			next.ServeHTTP(w, r)
			return
		}

		customerNo := strings.TrimSpace(r.Header.Get("X-Demo-Customer-ID"))
		if !s.cfg.IsDev() {
			writeError(w, r, s.logger, domain.NewError(domain.CodeUnauthorized, "未配置认证方式，无法识别客户身份"))
			return
		}
		if customerNo == "" {
			writeError(w, r, s.logger, domain.NewError(domain.CodeUnauthorized, "缺少 X-Demo-Customer-ID 请求头（开发环境身份）"))
			return
		}
		customer, err := s.store.GetCustomerByNo(r.Context(), customerNo)
		if err != nil {
			writeError(w, r, s.logger, domain.NewError(domain.CodeUnauthorized, "无法识别客户身份").Wrap(err))
			return
		}
		if customer.Status != "active" {
			writeError(w, r, s.logger, domain.NewError(domain.CodeForbidden, "客户账号不可用"))
			return
		}

		ctx := context.WithValue(r.Context(), ctxKeyIdentity, Identity{
			CustomerID: customer.ID,
			CustomerNo: customer.CustomerNo,
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// requireIdentity returns the identity or writes a 401 and reports false.
func (s *Server) requireIdentity(w http.ResponseWriter, r *http.Request) (Identity, bool) {
	id, ok := identityFrom(r.Context())
	if !ok || id.CustomerID == "" {
		writeError(w, r, s.logger, domain.NewError(domain.CodeUnauthorized, "未识别客户身份"))
		return Identity{}, false
	}
	return id, true
}
