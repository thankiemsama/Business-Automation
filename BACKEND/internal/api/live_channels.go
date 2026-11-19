package api

import (
	"io"
	"net/http"
	"time"

	"businessautomation/internal/core"
	"businessautomation/internal/service"
)

func (h *Handler) liveChannelRoutes(mux *http.ServeMux) {
	channels, ok := h.channels.(*service.ChannelManager)
	if !ok {
		return
	}
	mux.HandleFunc("GET /api/v1/channel-webhooks/{tenant}/{channel}", func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		challenge := query.Get("hub.challenge")
		if len(challenge) > 256 || challenge == "" || !channels.VerifyChallenge(r.PathValue("tenant"), r.PathValue("channel"), query.Get("hub.mode"), query.Get("hub.verify_token")) {
			http.Error(w, "Webhook verification failed", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, challenge)
	})
	mux.HandleFunc("POST /api/v1/channel-webhooks/{tenant}/{channel}", func(w http.ResponseWriter, r *http.Request) {
		if !h.acquireAuthSlot(w) {
			return
		}
		defer h.releaseAuthSlot()
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			respond(w, 413, nil, core.Fail(413, "Webhook vượt 1 MiB hoặc không đọc được"))
			return
		}
		err = channels.Receive(r.PathValue("tenant"), r.PathValue("channel"), r.Header, body)
		respond(w, http.StatusOK, map[string]bool{"received": true}, err)
	})
	mux.HandleFunc("POST /api/v1/channels/{channel}/test", h.protected(func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		if !h.allowAI(w, "channel-test:"+user.ID, 10, time.Minute) {
			return
		}
		var input struct {
			To   string `json:"to"`
			Text string `json:"text"`
		}
		if err := decode(w, r, &input); err != nil {
			respond(w, http.StatusBadRequest, nil, err)
			return
		}
		delivery, err := channels.TestDelivery(user, r.PathValue("channel"), input.To, input.Text)
		respond(w, http.StatusAccepted, delivery, err)
	}))
	mux.HandleFunc("GET /api/v1/channels/{channel}/deliveries", h.protected(func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		items, err := channels.Deliveries(user, r.PathValue("channel"))
		respond(w, http.StatusOK, items, err)
	}))
	mux.HandleFunc("POST /api/v1/channels/{channel}/deliveries/{id}/retry", h.protected(func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		err := channels.Retry(user, r.PathValue("channel"), r.PathValue("id"))
		respond(w, http.StatusAccepted, map[string]bool{"queued": true}, err)
	}))
}
