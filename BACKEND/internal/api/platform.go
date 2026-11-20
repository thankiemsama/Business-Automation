package api

import (
	"net/http"
	"time"

	"businessautomation/internal/core"
	"businessautomation/internal/service"
)

// platformRoutes registers the differentiation endpoints: public knowledge base,
// customer portal, CSAT surveys, Customer 360°, multi-channel config, outgoing
// webhooks, and public API keys.
func (h *Handler) platformRoutes(mux *http.ServeMux) {
	if h.platform == nil {
		return
	}
	// ---- Public knowledge base (SEO-friendly, guest access) ----
	mux.HandleFunc("GET /api/v1/kb/{tenant}", h.kbList)
	mux.HandleFunc("GET /api/v1/kb/{tenant}/{slug}", h.kbArticle)

	// ---- Customer self-serve portal (guest auth via email code) ----
	mux.HandleFunc("POST /api/v1/portal/{tenant}/code", h.portalCode)
	mux.HandleFunc("POST /api/v1/portal/{tenant}/login", h.portalLogin)
	mux.HandleFunc("GET /api/v1/portal/{tenant}/tickets", h.portalTickets)
	mux.HandleFunc("GET /api/v1/portal/{tenant}/tickets/{id}", h.portalTicket)
	mux.HandleFunc("POST /api/v1/portal/{tenant}/tickets", h.portalCreateTicket)

	// ---- CSAT public survey widget ----
	mux.HandleFunc("GET /api/v1/sat/{token}", h.csatResolve)
	mux.HandleFunc("POST /api/v1/sat/{token}", h.csatSubmit)

	// ---- Customer 360° profile (authenticated agent/owner) ----
	mux.HandleFunc("GET /api/v1/customers/{id}/profile", h.protected(h.customerProfile))
	mux.HandleFunc("POST /api/v1/customers/{id}/notes", h.protected(h.customerNote))
	mux.HandleFunc("POST /api/v1/customers/{id}/merge", h.protected(h.customerMerge))

	// ---- Multi-channel configuration (owner) ----
	mux.HandleFunc("GET /api/v1/channels", h.protected(h.channelStatus))
	mux.HandleFunc("POST /api/v1/channels/{channel}", h.protected(h.channelConfigure))

	// ---- Outgoing webhooks (owner) ----
	mux.HandleFunc("GET /api/v1/webhooks", h.protected(h.webhookList))
	mux.HandleFunc("POST /api/v1/webhooks", h.protected(h.webhookSave))
	mux.HandleFunc("POST /api/v1/webhooks/{id}", h.protected(h.webhookUpdate))
	mux.HandleFunc("POST /api/v1/webhooks/{id}/remove", h.protected(h.webhookRemove))
	mux.HandleFunc("GET /api/v1/webhooks/dead-letters", h.protected(h.webhookDeadLetters))
	mux.HandleFunc("POST /api/v1/webhooks/dead-letters/{id}/replay", h.protected(h.webhookReplay))

	// ---- Public API keys (owner manages; public API is BFF-gated) ----
	mux.HandleFunc("GET /api/v1/api-keys", h.protected(h.apiKeyList))
	mux.HandleFunc("POST /api/v1/api-keys", h.protected(h.apiKeyCreate))
	mux.HandleFunc("POST /api/v1/api-keys/{id}/revoke", h.protected(h.apiKeyRevoke))
}

// ---- Knowledge base ----

func (h *Handler) kbList(w http.ResponseWriter, r *http.Request) {
	articles, err := h.knowledgePublic.PublicList(r.PathValue("tenant"), "")
	respond(w, http.StatusOK, articles, err)
}

func (h *Handler) kbArticle(w http.ResponseWriter, r *http.Request) {
	articles, err := h.knowledgePublic.PublicList(r.PathValue("tenant"), r.PathValue("slug"))
	if err != nil {
		respond(w, http.StatusNotFound, nil, err)
		return
	}
	respond(w, http.StatusOK, articles[0], nil)
}

// ---- Customer portal ----

func (h *Handler) portalCode(w http.ResponseWriter, r *http.Request) {
	if !h.allowAuth(r) {
		respond(w, http.StatusTooManyRequests, nil, core.Fail(http.StatusTooManyRequests, "Vui lòng thử lại sau"))
		return
	}
	var input struct {
		Email string `json:"email"`
	}
	if err := decode(w, r, &input); err != nil {
		respond(w, http.StatusBadRequest, nil, err)
		return
	}
	tenant := r.PathValue("tenant")
	err := h.platform.RequestCode(tenant, input.Email)
	if err == nil {
		respond(w, http.StatusAccepted, map[string]string{"message": "Đã gửi mã nếu email tồn tại"}, nil)
		return
	}
	respond(w, http.StatusServiceUnavailable, nil, err)
}

func (h *Handler) portalLogin(w http.ResponseWriter, r *http.Request) {
	if !h.allowAuth(r) {
		respond(w, http.StatusTooManyRequests, nil, core.Fail(http.StatusTooManyRequests, "Vui lòng thử lại sau"))
		return
	}
	var input struct {
		Email string `json:"email"`
		Code  string `json:"code"`
	}
	if err := decode(w, r, &input); err != nil {
		respond(w, http.StatusBadRequest, nil, err)
		return
	}
	tenant := r.PathValue("tenant")
	customer, err := h.platform.Login(tenant, input.Email, input.Code)
	if err != nil {
		respond(w, http.StatusUnauthorized, nil, err)
		return
	}
	respond(w, http.StatusOK, customer, nil)
}

// portalAuth resolves a "Bearer <portal_token>" header to a tenant + token pair.
func (h *Handler) portalAuth(r *http.Request) (tenant, token string, ok bool) {
	tenant = r.PathValue("tenant")
	token = bearerToken(r)
	if tenant == "" || token == "" {
		return tenant, token, false
	}
	_, err := h.platform.PortalTickets(tenant, token)
	return tenant, token, err == nil
}

func (h *Handler) portalTickets(w http.ResponseWriter, r *http.Request) {
	tenant, token, ok := h.portalAuth(r)
	if !ok {
		respond(w, http.StatusUnauthorized, nil, core.Fail(http.StatusUnauthorized, "Phiên khách hàng không hợp lệ"))
		return
	}
	items, err := h.platform.PortalTickets(tenant, token)
	respond(w, http.StatusOK, items, err)
}

func (h *Handler) portalTicket(w http.ResponseWriter, r *http.Request) {
	tenant, token, ok := h.portalAuth(r)
	if !ok {
		respond(w, http.StatusUnauthorized, nil, core.Fail(http.StatusUnauthorized, "Phiên khách hàng không hợp lệ"))
		return
	}
	ticket, messages, err := h.platform.PortalTicket(tenant, token, r.PathValue("id"))
	respond(w, http.StatusOK, map[string]any{"ticket": ticket, "messages": messages}, err)
}

func (h *Handler) portalCreateTicket(w http.ResponseWriter, r *http.Request) {
	tenant, token, ok := h.portalAuth(r)
	if !ok {
		respond(w, http.StatusUnauthorized, nil, core.Fail(http.StatusUnauthorized, "Phiên khách hàng không hợp lệ"))
		return
	}
	var input struct {
		Subject        string `json:"subject"`
		Content        string `json:"content"`
		Priority       string `json:"priority"`
		AttachmentName string `json:"attachment_name"`
		AttachmentData string `json:"attachment_data"`
	}
	if err := decode(w, r, &input); err != nil {
		respond(w, http.StatusBadRequest, nil, err)
		return
	}
	result, err := h.platform.CreatePortalTicket(tenant, token, input.Subject, input.Content, input.Priority, input.AttachmentName, input.AttachmentData)
	respond(w, http.StatusCreated, result, err)
}

// ---- CSAT ----

func parseSatToken(raw string) (tenant, secret string) {
	tenant, secret, _ = cutTenant(raw)
	return tenant, secret
}

// cutTenant splits a "{tenant}.{secret}" token on the first dot.
func cutTenant(raw string) (tenant, secret string, ok bool) {
	i := 0
	for i < len(raw) && raw[i] != '.' {
		i++
	}
	if i == 0 || i == len(raw) {
		return "", "", false
	}
	return raw[:i], raw[i+1:], true
}

func (h *Handler) csatResolve(w http.ResponseWriter, r *http.Request) {
	tenant, secret := parseSatToken(r.PathValue("token"))
	result, err := h.csat.ResolveLink(tenant, secret)
	respond(w, http.StatusOK, result, err)
}

func (h *Handler) csatSubmit(w http.ResponseWriter, r *http.Request) {
	if !h.allowAuth(r) {
		respond(w, http.StatusTooManyRequests, nil, core.Fail(http.StatusTooManyRequests, "Vui lòng thử lại sau"))
		return
	}
	tenant, secret := parseSatToken(r.PathValue("token"))
	var input struct {
		Rating  int    `json:"rating"`
		Comment string `json:"comment"`
	}
	if err := decode(w, r, &input); err != nil {
		respond(w, http.StatusBadRequest, nil, err)
		return
	}
	_, err := h.csat.Submit(tenant, secret, input.Rating, input.Comment)
	respond(w, http.StatusCreated, map[string]bool{"rated": err == nil}, err)
}

// ---- Customer 360° ----

func (h *Handler) customerProfile(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	profile, err := h.platform.Profile(user, r.PathValue("id"))
	respond(w, http.StatusOK, profile, err)
}

func (h *Handler) customerNote(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	if r.Method != http.MethodPost {
		method(w, http.MethodPost)
		return
	}
	var input struct {
		Content string `json:"content"`
	}
	if err := decode(w, r, &input); err != nil {
		respond(w, http.StatusBadRequest, nil, err)
		return
	}
	result, err := h.platform.AddNote(user, r.PathValue("id"), input.Content)
	respond(w, http.StatusCreated, result, err)
}

func (h *Handler) customerMerge(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	if r.Method != http.MethodPost {
		method(w, http.MethodPost)
		return
	}
	var input struct {
		TargetID string `json:"target_id"`
	}
	if err := decode(w, r, &input); err != nil {
		respond(w, http.StatusBadRequest, nil, err)
		return
	}
	if input.TargetID == "" {
		respond(w, http.StatusBadRequest, nil, core.Fail(http.StatusBadRequest, "Cần target_id"))
		return
	}
	result, err := h.platform.MergeCustomers(user, r.PathValue("id"), input.TargetID)
	respond(w, http.StatusOK, result, err)
}

// ---- Multi-channel ----

func (h *Handler) channelStatus(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	if !ownerIntegration(w, user) {
		return
	}
	if r.Method != http.MethodGet {
		method(w, http.MethodGet)
		return
	}
	status, err := h.channels.Status(user)
	respond(w, http.StatusOK, status, err)
}

func (h *Handler) channelConfigure(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	if r.Method != http.MethodPost {
		method(w, http.MethodPost)
		return
	}
	var input struct {
		Name    string `json:"name"`
		Content string `json:"content"`
		Enabled bool   `json:"enabled"`
	}
	if err := decode(w, r, &input); err != nil {
		respond(w, http.StatusBadRequest, nil, err)
		return
	}
	record := core.Record{Name: input.Name, Content: input.Content, Enabled: input.Enabled}
	result, err := h.channels.Configure(user, r.PathValue("channel"), record)
	respond(w, http.StatusCreated, result, err)
}

// ---- Webhooks ----

func (h *Handler) webhookDeadLetters(w http.ResponseWriter, _ *http.Request, user core.User, _ string) {
	items, err := h.webhooks.DeadLetters(user)
	respond(w, http.StatusOK, items, err)
}

func (h *Handler) webhookReplay(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	respond(w, http.StatusOK, nil, h.webhooks.Replay(user, r.PathValue("id")))
}

func (h *Handler) webhookList(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	if !ownerIntegration(w, user) {
		return
	}
	items, err := h.webhooks.List(user)
	respond(w, http.StatusOK, items, err)
}

func (h *Handler) webhookSave(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	if r.Method != http.MethodPost {
		method(w, http.MethodPost)
		return
	}
	var input service.Webhook
	if err := decode(w, r, &input); err != nil {
		respond(w, http.StatusBadRequest, nil, err)
		return
	}
	result, err := h.webhooks.Save(user, "", input)
	respond(w, http.StatusCreated, result, err)
}

func (h *Handler) webhookUpdate(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	if r.Method != http.MethodPost {
		method(w, http.MethodPost)
		return
	}
	var input service.Webhook
	if err := decode(w, r, &input); err != nil {
		respond(w, http.StatusBadRequest, nil, err)
		return
	}
	result, err := h.webhooks.Save(user, r.PathValue("id"), input)
	respond(w, http.StatusOK, result, err)
}

func (h *Handler) webhookRemove(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	respond(w, http.StatusOK, nil, h.webhooks.Remove(user, r.PathValue("id")))
}

// ---- API keys ----

func (h *Handler) apiKeyList(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	if !ownerIntegration(w, user) {
		return
	}
	items, err := h.platform.ListKeys(user)
	respond(w, http.StatusOK, items, err)
}

func (h *Handler) apiKeyCreate(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	if r.Method != http.MethodPost {
		method(w, http.MethodPost)
		return
	}
	var input struct {
		Name       string `json:"name"`
		Scope      string `json:"scope"`
		RatePerMin int    `json:"rate_per_min"`
	}
	if err := decode(w, r, &input); err != nil {
		respond(w, http.StatusBadRequest, nil, err)
		return
	}
	result, err := h.platform.CreateKey(user, input.Name, input.Scope, input.RatePerMin)
	respond(w, http.StatusCreated, result, err)
}

func (h *Handler) apiKeyRevoke(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	respond(w, http.StatusOK, nil, h.platform.RevokeKey(user, r.PathValue("id")))
}

// bearerToken extracts a Bearer token from the Authorization header.
func ownerIntegration(w http.ResponseWriter, user core.User) bool {
	if user.Role == core.RoleOwner {
		return true
	}
	respond(w, 403, nil, core.Fail(403, "Chỉ owner được quản lý tích hợp"))
	return false
}

func bearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if len(auth) > 7 && auth[:7] == "Bearer " {
		return auth[7:]
	}
	return ""
}

var _ = time.Now // keep time import for future rate-limit windows
