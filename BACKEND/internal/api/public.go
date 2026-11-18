package api

import (
	"encoding/json"
	"net/http"
	"time"

	"businessautomation/internal/core"
	"businessautomation/internal/service"
)

// publicRoutes exposes developer REST endpoints authenticated with tenant API
// keys. These are distinct from the cookie/CSRF-gated BFF routes: they are
// keyed, rate-limited per key, and intended for external integrations
// (Zapier/n8n/custom). Public keys are read-only.
func (h *Handler) publicRoutes(mux *http.ServeMux) {
	if h.platform == nil {
		return
	}
	mux.HandleFunc("GET /api/public/openapi.json", h.openapiSpec)
	for _, pattern := range []string{
		"/api/public/v1/customers",
		"/api/public/v1/customers/{id}",
		"/api/public/v1/tickets",
		"/api/public/v1/tickets/{id}",
		"/api/public/v1/conversations",
		"/api/public/v1/conversations/{id}",
	} {
		mux.HandleFunc(pattern, h.publicResource(pattern))
	}
}

// publicResource handles a public, read-only REST resource via API-key auth.
func (h *Handler) publicResource(pattern string) http.HandlerFunc {
	kind := resourceFromPattern(pattern)
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			method(w, http.MethodGet)
			return
		}
		owner, key, err := h.publicAuth(r)
		if err != nil {
			if typed, ok := err.(*core.Error); ok && typed.Status == 429 {
				w.Header().Set("Retry-After", "60")
			}
			respond(w, http.StatusUnauthorized, nil, err)
			return
		}
		if !h.tenantAccess(w, r, owner.TenantID) {
			return
		}
		if h.admin != nil && !h.admin.Feature(owner.TenantID, "public_api") {
			respond(w, 403, nil, core.Fail(403, "API công khai đã tắt"))
			return
		}
		_ = key
		id := r.PathValue("id")
		items, listErr := h.resources.List(owner, kind, id)
		if id == "" {
			h.page(w, r, items, listErr)
			return
		}
		if listErr != nil {
			respond(w, http.StatusNotFound, nil, listErr)
			return
		}
		if len(items) == 0 {
			respond(w, http.StatusNotFound, nil, core.Fail(404, "Không tìm thấy dữ liệu"))
			return
		}
		respond(w, http.StatusOK, items[0], nil)
	}
}

// resourceFromPattern maps a public route pattern to its resource kind.
func resourceFromPattern(pattern string) string {
	switch {
	case pattern == "/api/public/v1/customers" || pattern == "/api/public/v1/customers/{id}":
		return core.KindCustomers
	case pattern == "/api/public/v1/tickets" || pattern == "/api/public/v1/tickets/{id}":
		return core.KindTickets
	default:
		return core.KindConversations
	}
}

// publicAuth resolves a developer API key to its tenant owner, applying the
// per-key sliding-window rate limit.
func (h *Handler) publicAuth(r *http.Request) (core.User, *service.APIKey, error) {
	key, owner, err := h.platform.ResolveAPIKey(bearerToken(r))
	if err != nil {
		return core.User{}, nil, err
	}
	if !hasReadScope(key.Scopes) {
		return core.User{}, nil, core.Fail(403, "Khóa API thiếu quyền đọc")
	}
	limit := key.RatePerMin
	if limit <= 0 {
		limit = 60
	}
	h.mu.Lock()
	now := time.Now()
	for id, active := range h.keyRate {
		if !active.until.After(now) {
			delete(h.keyRate, id)
		}
	}
	bucket, _ := h.keyRate[key.ID]
	if now.After(bucket.until) {
		bucket = window{until: now.Add(time.Minute)}
	}
	bucket.count++
	h.keyRate[key.ID] = bucket
	exceeds := bucket.count > limit
	h.mu.Unlock()
	if exceeds {
		return core.User{}, nil, core.Fail(429, "Vượt giới hạn tốc độ của khóa API")
	}
	return owner, &key, nil
}

func hasReadScope(scopes []string) bool {
	for _, scope := range scopes {
		if scope == "read" {
			return true
		}
	}
	return false
}

// openapiSpec serves a minimal OpenAPI 3.0 document describing the public API.
func (h *Handler) openapiSpec(w http.ResponseWriter, r *http.Request) {
	paths := map[string]any{}
	for _, kind := range []string{"customers", "tickets", "conversations"} {
		responses := map[string]any{
			"200": map[string]string{"description": "Thành công"},
			"401": map[string]string{"description": "Khóa API không hợp lệ"},
			"403": map[string]string{"description": "Khóa thiếu quyền đọc"},
			"429": map[string]string{"description": "Vượt giới hạn của khóa"},
		}
		prefix := "/api/public/v1/"
		if r.URL.Query().Get("proxy") == "1" {
			prefix = "/public/v1/"
		}
		path := prefix + kind
		paths[path] = map[string]any{"get": map[string]any{"operationId": "list_" + kind, "responses": responses}}
		detail := map[string]any{}
		for status, response := range responses {
			detail[status] = response
		}
		detail["404"] = map[string]string{"description": "Không tìm thấy"}
		paths[path+"/{id}"] = map[string]any{"get": map[string]any{"operationId": "get_" + kind, "parameters": []map[string]any{{"in": "path", "name": "id", "required": true, "schema": map[string]string{"type": "string"}}}, "responses": detail}}
	}
	spec := map[string]any{
		"openapi": "3.0.3",
		"info": map[string]any{
			"title":       "Nexa Public API",
			"version":     "1.0",
			"description": "REST API công khai cho tích hợp bên thứ ba. Xác thực bằng API key trong header Authorization: Bearer <nexa_key>.",
		},
		"security": []map[string][]string{{"api_key": {}}},
		"paths":    paths,
		"components": map[string]any{
			"securitySchemes": map[string]any{
				"api_key": map[string]any{"type": "http", "scheme": "bearer", "description": "API key của tenant Nexa"},
			},
		},
	}
	if r.URL.Query().Get("proxy") == "1" {
		spec["servers"] = []map[string]string{{"url": "/api/platform"}}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(spec)
}
