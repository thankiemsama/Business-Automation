package api

import (
	"net/http"

	"businessautomation/internal/core"
	"businessautomation/internal/service"
)

// governanceRoutes exposes the owner-facing agent control plane: kill switches,
// risk tiers, approval gates and the agent inventory.
func (h *Handler) governanceRoutes(mux *http.ServeMux) {
	if h.governance == nil {
		return
	}
	mux.HandleFunc("GET /api/v1/ai/governance", h.protected(func(w http.ResponseWriter, _ *http.Request, user core.User, _ string) {
		config, entries, err := h.governance.Inventory(user)
		respond(w, 200, map[string]any{"governance": config, "inventory": entries}, err)
	}))
	mux.HandleFunc("POST /api/v1/ai/governance", h.protected(func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		if !ownerIntegration(w, user) {
			return
		}
		var input struct {
			PlatformKill bool                    `json:"platform_kill"`
			Config       service.AgentGovernance `json:"config"`
		}
		if err := decode(w, r, &input); err != nil {
			respond(w, 400, nil, err)
			return
		}
		saved, err := h.governance.Save(user, input.Config, input.PlatformKill)
		respond(w, 200, saved, err)
	}))
}

// aiActionRoutes exposes the AI action catalog and its approval gate.
func (h *Handler) aiActionRoutes(mux *http.ServeMux) {
	if h.actions == nil {
		return
	}
	mux.HandleFunc("GET /api/v1/ai/actions", h.protected(func(w http.ResponseWriter, _ *http.Request, user core.User, _ string) {
		items, err := h.actions.List(user)
		respond(w, 200, items, err)
	}))
	mux.HandleFunc("POST /api/v1/ai/actions", h.protected(func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		if !ownerIntegration(w, user) {
			return
		}
		var item service.AIAction
		if err := decode(w, r, &item); err != nil {
			respond(w, 400, nil, err)
			return
		}
		saved, err := h.actions.Save(user, item)
		respond(w, 200, saved, err)
	}))
	mux.HandleFunc("POST /api/v1/ai/actions/remove", h.protected(func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		if !ownerIntegration(w, user) {
			return
		}
		var input struct {
			ID string `json:"id"`
		}
		if err := decode(w, r, &input); err != nil {
			respond(w, 400, nil, err)
			return
		}
		respond(w, 200, nil, h.actions.Remove(user, input.ID))
	}))
}

// aiSkillRoutes exposes the AI skill catalog and the reply-feedback loop.
func (h *Handler) aiSkillRoutes(mux *http.ServeMux) {
	if h.skills == nil {
		return
	}
	mux.HandleFunc("GET /api/v1/ai/skills", h.protected(func(w http.ResponseWriter, _ *http.Request, user core.User, _ string) {
		items, err := h.skills.Skills(user)
		respond(w, 200, items, err)
	}))
	mux.HandleFunc("POST /api/v1/ai/skills", h.protected(func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		if !ownerIntegration(w, user) {
			return
		}
		var item service.AISkill
		if err := decode(w, r, &item); err != nil {
			respond(w, 400, nil, err)
			return
		}
		saved, err := h.skills.SaveSkill(user, item)
		respond(w, 200, saved, err)
	}))
	mux.HandleFunc("POST /api/v1/ai/skills/remove", h.protected(func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		if !ownerIntegration(w, user) {
			return
		}
		var input struct {
			ID string `json:"id"`
		}
		if err := decode(w, r, &input); err != nil {
			respond(w, 400, nil, err)
			return
		}
		respond(w, 200, nil, h.skills.RemoveSkill(user, input.ID))
	}))
	mux.HandleFunc("POST /api/v1/ai/feedback", h.protected(func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		if !ownerIntegration(w, user) {
			return
		}
		var input struct {
			MessageID string `json:"message_id"`
			Rating    int    `json:"rating"`
			Comment   string `json:"comment"`
		}
		if err := decode(w, r, &input); err != nil {
			respond(w, 400, nil, err)
			return
		}
		respond(w, 200, nil, h.skills.RecordFeedback(user, input.MessageID, input.Rating, input.Comment))
	}))
	mux.HandleFunc("GET /api/v1/ai/feedback/stats", h.protected(func(w http.ResponseWriter, _ *http.Request, user core.User, _ string) {
		stats, err := h.skills.FeedbackStats(user)
		respond(w, 200, stats, err)
	}))
}
