package api

import (
	"businessautomation/internal/core"
	"businessautomation/internal/service"
	"crypto/subtle"
	"io"
	"net/http"
	"time"
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
	input.DeliveryID = r.Header.Get("X-Nexa-Delivery")
	message, err := h.conversations.IngestEmail(h.inboundEmailTenant, input)
	respond(w, http.StatusOK, map[string]string{"id": message.ID}, err)
}

func (h *Handler) deliverEmail(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	if !h.acquireAuthSlot(w) {
		return
	}
	defer h.releaseAuthSlot()
	message, err := h.conversations.DeliverEmail(r.Context(), user, r.PathValue("id"), r.PathValue("message"))
	if err == nil && h.workspace != nil {
		err = h.workspace.HydrateAttachment(r.Context(), &message)
	}
	status := http.StatusOK
	if message.Status == "pending" {
		status = http.StatusAccepted
	}
	respond(w, status, message, err)
}

func (h *Handler) inboundResend(w http.ResponseWriter, r *http.Request) {
	if h.resendInbound == nil || h.inboundEmailTenant == "" {
		respond(w, 404, nil, core.Fail(404, "Webhook Resend chưa cấu hình"))
		return
	}
	if !h.acquireAuthSlot(w) {
		return
	}
	defer h.releaseAuthSlot()
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		respond(w, 413, nil, core.Fail(413, "Webhook vượt 1 MiB hoặc không đọc được"))
		return
	}
	input, err := h.resendInbound.Receive(r.Context(), r.Header, raw, time.Now())
	if err != nil || input == nil {
		respond(w, 200, nil, err)
		return
	}
	message, err := h.conversations.IngestEmail(h.inboundEmailTenant, *input)
	respond(w, 200, map[string]string{"id": message.ID}, err)
}
