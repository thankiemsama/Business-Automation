package api

import (
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"

	"businessautomation/internal/core"
	"businessautomation/internal/service"
)

type securityAccount interface {
	MFA(core.User, string, string, string, string) (map[string]any, error)
	ActiveSessions(core.User, string) ([]service.ActiveSession, error)
	RevokeSession(core.User, string) error
}

func requestIP(r *http.Request) netip.Addr {
	raw, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		raw = r.RemoteAddr
	}
	ip, _ := netip.ParseAddr(raw)
	if ip.IsLoopback() {
		secret := os.Getenv("NEXA_PROXY_SECRET")
		if len(secret) >= 32 && subtle.ConstantTimeCompare([]byte(secret), []byte(r.Header.Get("X-Nexa-Proxy-Secret"))) == 1 {
			forwarded, err := netip.ParseAddr(r.Header.Get("X-Nexa-Client-IP"))
			if err == nil {
				return forwarded.Unmap()
			}
		}
	}
	return ip.Unmap()
}
func (h *Handler) tenantAccess(w http.ResponseWriter, r *http.Request, tenant string) bool {
	if h.admin == nil {
		return true
	}
	if !h.admin.Allowed(tenant, requestIP(r)) {
		respond(w, 403, nil, core.Fail(403, "Mạng hiện tại không được phép truy cập"))
		return false
	}
	h.admin.Meter(tenant)
	return true
}
func (h *Handler) adminRoutes(mux *http.ServeMux) {
	if h.admin == nil {
		return
	}
	for _, path := range []string{"settings", "audit", "usage", "export", "backup", "fields"} {
		mux.HandleFunc("GET /api/v1/admin/"+path, h.protected(h.adminRead(path)))
	}
	mux.HandleFunc("GET /api/v1/admin/brand", h.protected(func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		settings, err := h.admin.Settings(user.TenantID)
		respond(w, 200, map[string]string{"name": settings.DisplayName, "logo_url": settings.LogoURL, "accent": settings.Accent}, err)
	}))
	mux.HandleFunc("GET /api/v1/branding", func(w http.ResponseWriter, r *http.Request) {
		name := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("host")))
		value, err := h.admin.Branding(name)
		respond(w, 200, value, err)
	})
	mux.HandleFunc("PUT /api/v1/admin/settings", h.protected(func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		if !ownerIntegration(w, user) {
			return
		}
		var value service.TenantSettings
		if err := decode(w, r, &value); err != nil {
			respond(w, 400, nil, err)
			return
		}
		ip := requestIP(r)
		if len(value.Allowlist) > 0 && ip.IsLoopback() {
			respond(w, 400, nil, core.Fail(400, "Cần cấu hình IP thực qua ingress tin cậy trước khi bật allowlist"))
			return
		}
		saved, err := h.admin.SaveSettings(user, value, ip)
		respond(w, 200, saved, err)
	}))
	mux.HandleFunc("GET /api/v1/account/sessions", h.protected(h.accountSecurity("sessions")))
	mux.HandleFunc("POST /api/v1/account/sessions/revoke", h.protected(h.accountSecurity("revoke")))
	mux.HandleFunc("GET /api/v1/account/mfa", h.protected(h.accountSecurity("status")))
	mux.HandleFunc("POST /api/v1/account/mfa", h.protected(h.accountSecurity("mfa")))
	mux.HandleFunc("POST /api/v1/admin/fields", h.protected(h.saveField))
	mux.HandleFunc("GET /api/v1/admin/custom/{kind}/{id}", h.protected(h.customValues))
	mux.HandleFunc("PUT /api/v1/admin/custom/{kind}/{id}", h.protected(h.customValues))
	h.businessRoutes(mux)
	h.oidcRoutes(mux)
}
func (h *Handler) adminRead(path string) authenticatedHandler {
	return func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		if path != "fields" && !ownerIntegration(w, user) {
			return
		}
		switch path {
		case "settings":
			value, err := h.admin.Settings(user.TenantID)
			respond(w, 200, value, err)
		case "audit":
			result, err := h.admin.Audit(user)
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			start := min((min(max(1, page), 1000000)-1)*50, len(result))
			respond(w, 200, map[string]any{"items": result[start:min(start+50, len(result))], "total": len(result)}, err)
		case "usage":
			value, err := h.admin.Usage(user.TenantID)
			respond(w, 200, value, err)
		case "fields":
			fields, err := h.admin.Fields(user.TenantID)
			respond(w, 200, fields, err)
		default:
			// G8: bulk export/backup requires a 5-minute single-use step-up
			// token (password re-confirmation), so a leaked session cookie
			// alone cannot dump the tenant's data.
			if err := h.auth.ConsumeStepUp(user, r.URL.Query().Get("step_up")); err != nil {
				respond(w, 403, nil, err)
				return
			}
			value, err := h.admin.Export(user, r.URL.Query().Get("customer_id"))
			if err != nil {
				respond(w, 400, nil, err)
				return
			}
			if path == "backup" {
				backup, err := h.admin.Backup(user, value)
				if err != nil {
					respond(w, 400, nil, err)
					return
				}
				w.Header().Set("Content-Type", "application/octet-stream")
				w.Header().Set("Content-Disposition", `attachment; filename="nexa-backup.enc"`)
				_, _ = w.Write([]byte(backup))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Disposition", `attachment; filename="nexa-data-export.json"`)
			_ = json.NewEncoder(w).Encode(value)
		}
	}
}
func (h *Handler) accountSecurity(kind string) authenticatedHandler {
	return func(w http.ResponseWriter, r *http.Request, user core.User, token string) {
		auth, ok := h.auth.(securityAccount)
		if !ok {
			respond(w, 503, nil, core.Fail(503, "Cài đặt tài khoản chưa sẵn sàng"))
			return
		}
		if kind == "sessions" {
			value, err := auth.ActiveSessions(user, token)
			respond(w, 200, value, err)
			return
		}
		if kind == "status" {
			value, err := auth.MFA(user, "status", "", "", token)
			respond(w, 200, value, err)
			return
		}
		if !h.allowAuth(r) {
			respond(w, 429, nil, core.Fail(429, "Vui lòng thử lại sau"))
			return
		}
		if !h.acquireAuthSlot(w) {
			return
		}
		defer h.releaseAuthSlot()
		var input struct {
			Action   string `json:"action"`
			Password string `json:"password"`
			Code     string `json:"code"`
			ID       string `json:"id"`
		}
		if err := decode(w, r, &input); err != nil {
			respond(w, 400, nil, err)
			return
		}
		if kind == "revoke" {
			respond(w, 200, nil, auth.RevokeSession(user, input.ID))
			return
		}
		result, err := auth.MFA(user, input.Action, input.Password, input.Code, token)
		respond(w, 200, result, err)
	}
}
func (h *Handler) saveField(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	if !ownerIntegration(w, user) {
		return
	}
	var field service.CustomField
	if err := decode(w, r, &field); err != nil {
		respond(w, 400, nil, err)
		return
	}
	result, err := h.admin.SaveField(user, field)
	respond(w, 200, result, err)
}
func (h *Handler) customValues(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	kind, id := r.PathValue("kind"), r.PathValue("id")
	var fields map[string]string
	if r.Method == http.MethodPut {
		if err := decode(w, r, &fields); err != nil {
			respond(w, 400, nil, err)
			return
		}
	}
	result, err := h.admin.Custom(user, kind, id, fields)
	respond(w, 200, result, err)
}
