// Package httpapi exposes the HTTP contract: routing, middleware, DTO mapping and
// a single error envelope. Handlers only parse input and call services.
package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/example/ecommerce-support/internal/domain"
)

type ctxKey string

const (
	ctxKeyRequestID ctxKey = "request_id"
	ctxKeyIdentity  ctxKey = "identity"
)

// Identity is the authenticated caller. In the MVP it is injected by the dev
// identity middleware; in production a real auth middleware must fill it in.
type Identity struct {
	CustomerID string
	CustomerNo string
}

// requestIDFrom returns the request id stored by the middleware.
func requestIDFrom(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKeyRequestID).(string); ok {
		return v
	}
	return ""
}

// identityFrom returns the authenticated identity, if any.
func identityFrom(ctx context.Context) (Identity, bool) {
	v, ok := ctx.Value(ctxKeyIdentity).(Identity)
	return v, ok
}

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

// writeData writes a successful envelope.
func writeData(w http.ResponseWriter, r *http.Request, data any, meta any) {
	writeJSON(w, http.StatusOK, successEnvelope{Data: data, Meta: meta})
}

// writeCreated writes a 201 envelope.
func writeCreated(w http.ResponseWriter, r *http.Request, data any) {
	writeJSON(w, http.StatusCreated, successEnvelope{Data: data})
}

// writeError maps a domain error onto the stable error envelope. Internal causes
// are logged, never returned.
func writeError(w http.ResponseWriter, r *http.Request, logger *slog.Logger, err error) {
	appErr := domain.AsAppError(err)
	status := domain.HTTPStatus(appErr.Code)
	if status >= 500 && logger != nil {
		logger.Error("request failed",
			"request_id", requestIDFrom(r.Context()),
			"method", r.Method,
			"path", r.URL.Path,
			"code", string(appErr.Code),
			"error", appErr.Error())
	}
	writeJSON(w, status, errorEnvelope{Error: errorDetail{
		Code:      string(appErr.Code),
		Message:   appErr.Message,
		RequestID: requestIDFrom(r.Context()),
		Details:   appErr.Details,
	}})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(body)
}

// decodeJSON enforces a body size limit and rejects unknown fields so typos never
// silently change behaviour.
func decodeJSON(w http.ResponseWriter, r *http.Request, dest any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return domain.ValidationError("请求体读取超时")
		}
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			return domain.ValidationError("请求体过大")
		}
		if strings.Contains(err.Error(), "unknown field") {
			return domain.ValidationError("请求体包含未知字段")
		}
		return domain.NewError(domain.CodeBadRequest, "请求体不是合法 JSON")
	}
	// Only one JSON value is allowed.
	if dec.More() {
		return domain.NewError(domain.CodeBadRequest, "请求体包含多余内容")
	}
	return nil
}

func parseLimit(r *http.Request, def, max int) (int, error) {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return def, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return 0, domain.ValidationError("limit 必须是正整数")
	}
	if value > max {
		return 0, domain.ValidationError("limit 不能超过 " + strconv.Itoa(max))
	}
	return value, nil
}
