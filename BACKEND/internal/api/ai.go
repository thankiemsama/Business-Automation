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
	enabled := h.ai.TranscriptionEnabled() && os.Getenv("AI_CHAT_WIDGET_VOICE") == "true" && (h.admin == nil || h.admin.Feature(r.PathValue("tenant"), "ai"))
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
	h.transcribe(w, r, r.PathValue("tenant"))
}

func (h *Handler) aiRoutes(mux *http.ServeMux) {
	if h.ai == nil {
		return
	}
	mux.HandleFunc("GET /api/v1/ai/realtime-qa/status", h.protected(func(w http.ResponseWriter, _ *http.Request, user core.User, _ string) {
		enabled, err := h.ai.RealtimeQAEnabled(user)
		respond(w, 200, map[string]bool{"enabled": enabled}, err)
	}))
	mux.HandleFunc("POST /api/v1/conversations/{id}/realtime-qa", h.protected(func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		if !h.allowAI(w, "realtime-qa:"+user.ID, 8, time.Minute) {
			return
		}
		var input struct {
			Draft string `json:"draft"`
		}
		if err := decode(w, r, &input); err != nil {
			respond(w, 400, nil, err)
			return
		}
		result, err := h.ai.QueueRealtimeQA(user, r.PathValue("id"), input.Draft)
		respond(w, 202, result, err)
	}))
	mux.HandleFunc("GET /api/v1/conversations/{id}/realtime-qa/{request}", h.protected(func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		result, err := h.ai.RealtimeQAResult(user, r.PathValue("id"), r.PathValue("request"))
		respond(w, 200, result, err)
	}))
	mux.HandleFunc("GET /api/v1/ai/status", h.protected(func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		enabled := h.ai.Enabled() && (h.admin == nil || h.admin.Feature(user.TenantID, "ai"))
		respond(w, 200, map[string]bool{"enabled": enabled, "transcription": enabled && h.ai.TranscriptionEnabled()}, nil)
	}))
	mux.HandleFunc("GET /api/v1/ai/search", h.protected(h.aiSearch))
	mux.HandleFunc("POST /api/v1/ai/summary/{kind}/{id}", h.protected(h.aiSummary))
	mux.HandleFunc("GET /api/v1/ai/suggestion/{id}", h.protected(h.aiSuggestion))
	mux.HandleFunc("POST /api/v1/ai/suggestion/{id}", h.protected(h.aiSuggestion))
	mux.HandleFunc("GET /api/v1/ai/review/{id}", h.protected(func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		result, err := h.ai.TicketReview(user, r.PathValue("id"))
		respond(w, 200, result, err)
	}))
	mux.HandleFunc("POST /api/v1/ai/transcribe", h.protected(h.aiTranscribe))
	mux.HandleFunc("GET /api/v1/ai/voice-inbox/status", h.protected(func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		respond(w, 200, map[string]bool{"enabled": h.ai.VoiceImportEnabled(user)}, nil)
	}))
	mux.HandleFunc("GET /api/v1/ai/ticket-agent/status", h.protected(func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		respond(w, 200, h.ai.TicketAgentStatus(user), nil)
	}))
	mux.HandleFunc("POST /api/v1/ai/ticket-agent/{id}/plan", h.protected(h.planTicketAction))
	mux.HandleFunc("POST /api/v1/ai/ticket-agent/{id}/apply", h.protected(h.applyTicketAction))
	mux.HandleFunc("GET /api/v1/ai/ticket-agent/traces", h.protected(func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		rows, err := h.ai.AgentToolTraces(user)
		respond(w, 200, rows, err)
	}))
	mux.HandleFunc("GET /api/v1/ai/sparring", h.protected(h.sparringOverview))
	mux.HandleFunc("GET /api/v1/ai/sparring/preview/{id}", h.protected(h.sparringPreview))
	mux.HandleFunc("POST /api/v1/ai/sparring/scenarios", h.protected(h.sparringScenario))
	mux.HandleFunc("POST /api/v1/ai/sparring/scenarios/{id}/archive", h.protected(h.sparringArchive))
	mux.HandleFunc("POST /api/v1/ai/sparring/sessions", h.protected(h.sparringStart))
	mux.HandleFunc("POST /api/v1/ai/sparring/sessions/{id}/turn", h.protected(h.sparringTurn))
	mux.HandleFunc("POST /api/v1/ai/sparring/sessions/{id}/end", h.protected(h.sparringEnd))
	mux.HandleFunc("POST /api/v1/ai/voice-inbox/transcribe", h.protected(h.importVoiceRecording))
	mux.HandleFunc("GET /api/v1/chat/{tenant}/voice-status", h.guestVoice)
	mux.HandleFunc("POST /api/v1/chat/{tenant}/transcribe", h.guestVoice)
}

func (h *Handler) planTicketAction(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	if !h.allowAI(w, user.ID, 10, time.Minute) || !h.acquireAuthSlot(w) {
		return
	}
	defer h.releaseAuthSlot()
	result, err := h.ai.PlanTicketAction(user, r.PathValue("id"))
	respond(w, 200, result, err)
}

func (h *Handler) applyTicketAction(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	var input struct {
		PlanID string `json:"plan_id"`
	}
	if err := decode(w, r, &input); err != nil {
		respond(w, 400, nil, err)
		return
	}
	result, err := h.ai.ApplyTicketAction(user, r.PathValue("id"), input.PlanID)
	respond(w, 200, result, err)
}

func (h *Handler) importVoiceRecording(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	query := r.URL.Query()
	if query.Get("authorized") != "true" || query.Get("customer_id") == "" {
		respond(w, 400, nil, core.Fail(400, "Chọn khách hàng và xác nhận quyền xử lý bản ghi âm"))
		return
	}
	if !h.ai.VoiceImportEnabled(user) {
		respond(w, 403, nil, core.Fail(403, "Nhập ghi âm chưa được bật hoặc bạn không có quyền"))
		return
	}
	if !h.allowAI(w, user.ID, 10, time.Minute) || !h.acquireAuthSlot(w) {
		return
	}
	defer h.releaseAuthSlot()
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(45 * time.Second))
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, service.MaxAudioBytes))
	if err != nil {
		respond(w, 413, nil, core.Fail(413, "Tệp vượt 4 MiB hoặc không đọc được"))
		return
	}
	conversation, err := h.ai.ImportVoice(user, query.Get("customer_id"), data)
	respond(w, 200, conversation, err)
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
	h.transcribe(w, r, user.TenantID)
}

func (h *Handler) transcribe(w http.ResponseWriter, r *http.Request, tenant string) {
	if !h.acquireAuthSlot(w) {
		return
	}
	defer h.releaseAuthSlot()
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(45 * time.Second))
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, service.MaxAudioBytes))
	if err != nil {
		respond(w, 413, nil, core.Fail(413, "Tệp vượt 4 MiB hoặc không đọc được"))
		return
	}
	result, err := h.ai.AnalyzeVoice(tenant, data)
	respond(w, 200, result, err)
}
