package api

import (
	"log/slog"
	"net/http"
	"strings"

	"businessautomation/internal/core"
)

type authenticatedHandler func(http.ResponseWriter, *http.Request, core.User, string)

func (h *Handler) protected(next authenticatedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			respond(w, http.StatusUnauthorized, nil, core.Fail(http.StatusUnauthorized, "Cần Bearer token"))
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		u, err := h.auth.Authenticate(token)
		if err != nil {
			respond(w, http.StatusUnauthorized, nil, err)
			return
		}
		if !h.tenantAccess(w, r, u.TenantID) {
			return
		}
		next(w, r, u, token)
	}
}

// Middleware decorators add cross-cutting behavior without changing controllers.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				slog.Error("request panic", "value", v)
				respond(w, http.StatusInternalServerError, nil, core.Fail(http.StatusInternalServerError, "Lỗi hệ thống"))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
