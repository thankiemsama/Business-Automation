package api

import (
	"io"
	"net/http"
	"time"

	"businessautomation/internal/core"
)

// voicebotRoutes exposes the P4 voicebot: a Twilio-authenticated webhook that
// receives each call turn and returns TwiML, plus owner-facing endpoints to
// initiate outbound AI calls and inspect call history.
func (h *Handler) voicebotRoutes(mux *http.ServeMux) {
	if h.voicebot == nil {
		return
	}

	// POST /api/v1/voicebot/calls/{tenant}
	// Twilio Voice webhook: form-encoded turn data, returns a TwiML document.
	mux.HandleFunc("POST /api/v1/voicebot/calls/{tenant}", func(w http.ResponseWriter, r *http.Request) {
		if !h.acquireAuthSlot(w) {
			return
		}
		defer h.releaseAuthSlot()
		tenant := r.PathValue("tenant")
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
		if err != nil {
			respond(w, 413, nil, core.Fail(413, "Webhook vượt 1 MiB hoặc không đọc được"))
			return
		}
		twiML, err := h.voicebot.InboundCall(tenant, r.Header, body)
		if err != nil {
			respond(w, 500, nil, err)
			return
		}
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = io.WriteString(w, twiML)
	})

	// GET /api/v1/voicebot/twiml/{tenant}/{ref}
	// Twilio fetches this URL for the first turn of an outbound call.
	mux.HandleFunc("GET /api/v1/voicebot/twiml/{tenant}/{ref}", func(w http.ResponseWriter, r *http.Request) {
		if !h.acquireAuthSlot(w) {
			return
		}
		defer h.releaseAuthSlot()
		tenant, ref := r.PathValue("tenant"), r.PathValue("ref")
		query := r.URL.Query()
		twiML := h.voicebot.TwimlForOutbound(tenant, ref, query.Get("to"), query.Get("greeting"))
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = io.WriteString(w, twiML)
	})

	// Owner-facing endpoints.
	mux.HandleFunc("POST /api/v1/ai/voicebot/call", h.protected(func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		if !ownerIntegration(w, user) {
			return
		}
		if !h.allowAI(w, "voicebot-call:"+user.ID, 10, time.Minute) {
			return
		}
		var input struct {
			To       string `json:"to"`
			Greeting string `json:"greeting"`
		}
		if err := decode(w, r, &input); err != nil {
			respond(w, 400, nil, err)
			return
		}
		sid, err := h.voicebot.OutboundCall(user, input.To, input.Greeting)
		respond(w, 202, map[string]string{"call_sid": sid}, err)
	}))
	mux.HandleFunc("GET /api/v1/ai/voicebot/calls", h.protected(func(w http.ResponseWriter, _ *http.Request, user core.User, _ string) {
		enabled := false
		if h.voicebot != nil {
			enabled = h.voicebot.Enabled(user)
		}
		history, err := h.voicebot.CallHistory(user)
		respond(w, 200, map[string]any{"enabled": enabled, "calls": history}, err)
	}))
}
