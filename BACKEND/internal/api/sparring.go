package api

import (
	"net/http"
	"time"

	"businessautomation/internal/core"
	"businessautomation/internal/service"
)

func (h *Handler) sparringOverview(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	data, err := h.ai.SparringOverview(user)
	respond(w, 200, data, err)
}

func (h *Handler) sparringPreview(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	data, err := h.ai.PreviewSparringScenario(user, r.PathValue("id"))
	respond(w, 200, data, err)
}

func (h *Handler) sparringScenario(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	var input service.SparringScenarioInput
	if err := decode(w, r, &input); err != nil {
		respond(w, 400, nil, err)
		return
	}
	result, err := h.ai.SaveSparringScenario(user, input)
	respond(w, 201, result, err)
}

func (h *Handler) sparringArchive(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	respond(w, 200, nil, h.ai.ArchiveSparringScenario(user, r.PathValue("id")))
}

func (h *Handler) sparringStart(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	var input struct {
		RequestID     string `json:"request_id"`
		ScenarioID    string `json:"scenario_id"`
		Certification bool   `json:"certification"`
	}
	if err := decode(w, r, &input); err != nil {
		respond(w, 400, nil, err)
		return
	}
	if !h.allowAI(w, "sparring-start:"+user.ID, 10, time.Hour) {
		return
	}
	result, err := h.ai.StartSparring(user, input.ScenarioID, input.RequestID, input.Certification)
	respond(w, 201, result, err)
}

func (h *Handler) sparringTurn(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	var input struct {
		Text      string `json:"text"`
		RequestID string `json:"request_id"`
		Turn      int    `json:"turn"`
	}
	if err := decode(w, r, &input); err != nil {
		respond(w, 400, nil, err)
		return
	}
	if !h.allowAI(w, "sparring-turn:"+user.ID, 15, time.Minute) || !h.acquireAuthSlot(w) {
		return
	}
	defer h.releaseAuthSlot()
	result, err := h.ai.SparringTurn(user, r.PathValue("id"), input.Text, input.RequestID, input.Turn)
	respond(w, 200, result, err)
}

func (h *Handler) sparringEnd(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	var input struct {
		Action string `json:"action"`
	}
	if err := decode(w, r, &input); err != nil {
		respond(w, 400, nil, err)
		return
	}
	respond(w, 202, nil, h.ai.EndSparring(user, r.PathValue("id"), input.Action))
}
