package api

import (
	"io"
	"net/http"

	"businessautomation/internal/core"
)

func (h *Handler) businessRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/admin/billing", h.protected(func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		if !ownerIntegration(w, user) {
			return
		}
		respond(w, 200, h.admin.BillingOptions(), nil)
	}))
	mux.HandleFunc("POST /api/v1/admin/billing", h.protected(func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		if !ownerIntegration(w, user) {
			return
		}
		var input struct {
			Plan string `json:"plan"`
		}
		if err := decode(w, r, &input); err != nil {
			respond(w, 400, nil, err)
			return
		}
		value, err := h.admin.BillingLink(r.Context(), user, input.Plan)
		respond(w, 200, map[string]string{"url": value}, err)
	}))
	mux.HandleFunc("POST /api/v1/stripe/webhook", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 256<<10))
		if err != nil {
			respond(w, 413, nil, core.Fail(413, "Payload quá lớn"))
			return
		}
		respond(w, 200, nil, h.admin.StripeEvent(r.Header.Get("Stripe-Signature"), body))
	})
	mux.HandleFunc("GET /api/v1/admin/crm", h.protected(func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		if !ownerIntegration(w, user) {
			return
		}
		respond(w, 200, h.admin.CRMOptions(user.TenantID), nil)
	}))
	mux.HandleFunc("POST /api/v1/admin/crm", h.protected(func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		if !ownerIntegration(w, user) {
			return
		}
		var input struct {
			Provider string `json:"provider"`
			Customer string `json:"customer_id"`
		}
		if err := decode(w, r, &input); err != nil {
			respond(w, 400, nil, err)
			return
		}
		if !h.acquireAuthSlot(w) {
			return
		}
		defer h.releaseAuthSlot()
		respond(w, 200, nil, h.admin.SyncCRM(r.Context(), user, input.Provider, input.Customer))
	}))
}
