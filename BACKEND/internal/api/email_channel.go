package api

import (
	"businessautomation/internal/core"
	"businessautomation/internal/service"
	"crypto/subtle"
	"net/http"
)

func (h *Handler) inboundEmail(w http.ResponseWriter, r *http.Request) {
	if h.inboundEmailTenant == "" || len(h.inboundEmailSecret) < 32 {
		respond(w, http.StatusNotFound, nil, core.Fail(404, "Kênh email chưa cấu hình"))
		return
	}
	if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+h.inboundEmailSecret)) != 1 {
		respond(w, http.StatusUnauthorized, nil, core.Fail(401, "Yêu cầu không hợp lệ"))
		return
	}
	var input service.InboundEmail
	if err := decode(w, r, &input); err != nil {
		respond(w, http.StatusBadRequest, nil, err)
		return
	}
	message, err := h.conversations.IngestEmail(h.inboundEmailTenant, input)
	respond(w, http.StatusOK, map[string]string{"id": message.ID}, err)
}

func (h *Handler) deliverEmail(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	if !h.acquireAuthSlot(w) {
		return
	}
	defer h.releaseAuthSlot()
	message, err := h.conversations.DeliverEmail(user, r.PathValue("id"), r.PathValue("message"))
	respond(w, http.StatusOK, message, err)
}
