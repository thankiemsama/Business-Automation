package api

import (
	"io"
	"net/http"
	"os"
	"time"

	"businessautomation/internal/core"
	"businessautomation/internal/service"
)

func (h *Handler) allowAI(w http.ResponseWriter, key string, limit int, period time.Duration) bool {
	now := time.Now()
	h.mu.Lock()
	for id, bucket := range h.aiRate {
		if !bucket.until.After(now) {
			delete(h.aiRate, id)
		}
	}
	bucket, exists := h.aiRate[key]
	if !exists {
		bucket = window{until: now.Add(period)}
	}
	accepted := bucket.count < limit && (exists || len(h.aiRate) < 5000)
	if accepted {
		bucket.count++
		h.aiRate[key] = bucket
	}
	h.mu.Unlock()
	if !accepted {
		respond(w, 429, nil, core.Fail(429, "Đã đạt giới hạn yêu cầu AI"))
	}
	return accepted
}

func (h *Handler) guestVoice(w http.ResponseWriter, r *http.Request) {
	enabled := h.ai.Enabled() && os.Getenv("AI_CHAT_WIDGET_VOICE") == "true" && (h.admin == nil || h.admin.Feature(r.PathValue("tenant"), "ai"))
	if r.Method == http.MethodGet {
		respond(w, 200, map[string]bool{"enabled": enabled}, nil)
		return
	}
	if !enabled {
		respond(w, 503, nil, core.Fail(503, "Chuyển giọng nói cho widget chưa bật"))
		return
	}
	scope, err := h.workspace.GuestScope(r.PathValue("tenant"), bearerToken(r))
	if err != nil {
		respond(w, 401, nil, err)
		return
	}
	if !h.allowAuth(r) {
		respond(w, 429, nil, core.Fail(429, "Vui lòng thử lại sau"))
		return
	}
	if !h.allowAI(w, scope, 5, time.Hour) {
		return
	}
	h.transcribe(w, r)
}

func (h *Handler) aiRoutes(mux *http.ServeMux) {
	if h.ai == nil {
		return
	}
	mux.HandleFunc("GET /api/v1/ai/status", h.protected(func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		respond(w, 200, map[string]bool{"enabled": h.ai.Enabled() && (h.admin == nil || h.admin.Feature(user.TenantID, "ai"))}, nil)
	}))
	mux.HandleFunc("GET /api/v1/ai/search", h.protected(h.aiSearch))
	mux.HandleFunc("POST /api/v1/ai/summary/{kind}/{id}", h.protected(h.aiSummary))
	mux.HandleFunc("GET /api/v1/ai/suggestion/{id}", h.protected(h.aiSuggestion))
	mux.HandleFunc("POST /api/v1/ai/suggestion/{id}", h.protected(h.aiSuggestion))
	mux.HandleFunc("POST /api/v1/ai/transcribe", h.protected(h.aiTranscribe))
	mux.HandleFunc("GET /api/v1/chat/{tenant}/voice-status", h.guestVoice)
	mux.HandleFunc("POST /api/v1/chat/{tenant}/transcribe", h.guestVoice)
}

func (h *Handler) aiSearch(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	if h.admin != nil && !h.admin.Feature(user.TenantID, "ai") {
		respond(w, 403, nil, core.Fail(403, "AI đã tắt cho doanh nghiệp"))
		return
	}
	if !h.allowAI(w, user.ID, 30, time.Minute) {
		return
	}
	if !h.acquireAuthSlot(w) {
		return
	}
	defer h.releaseAuthSlot()
	matches, err := h.ai.Search(user, r.URL.Query().Get("q"))
	respond(w, 200, matches, err)
}
func (h *Handler) aiSummary(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	if h.admin != nil && !h.admin.Feature(user.TenantID, "ai") {
		respond(w, 403, nil, core.Fail(403, "AI đã tắt cho doanh nghiệp"))
		return
	}
	if !h.allowAI(w, user.ID, 30, time.Minute) {
		return
	}
	summary, err := h.ai.Summary(user, r.PathValue("kind"), r.PathValue("id"))
	respond(w, 200, map[string]string{"summary": summary}, err)
}
func (h *Handler) aiSuggestion(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	result, err := h.ai.Suggest(user, r.PathValue("id"), r.Method == http.MethodPost)
	respond(w, 200, result, err)
}
func (h *Handler) aiTranscribe(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	if h.admin != nil && !h.admin.Feature(user.TenantID, "ai") {
		respond(w, 403, nil, core.Fail(403, "AI đã tắt cho doanh nghiệp"))
		return
	}
	if !h.allowAI(w, user.ID, 30, time.Minute) {
		return
	}
	h.transcribe(w, r)
}

func (h *Handler) transcribe(w http.ResponseWriter, r *http.Request) {
	if !h.acquireAuthSlot(w) {
		return
	}
	defer h.releaseAuthSlot()
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, service.MaxAudioBytes))
	if err != nil {
		respond(w, 413, nil, core.Fail(413, "Tệp vượt 4 MiB hoặc không đọc được"))
		return
	}
	text, err := h.ai.Transcribe(data)
	respond(w, 200, map[string]string{"text": text}, err)
}
