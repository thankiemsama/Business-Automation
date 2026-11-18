package api

import (
	"businessautomation/internal/core"
	"net/http"
)

func (h *Handler) assignment(kind string) authenticatedHandler {
	return func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		var input struct {
			AssigneeID string `json:"assignee_id"`
			Claim      bool   `json:"claim"`
		}
		if err := decode(w, r, &input); err != nil {
			respond(w, http.StatusBadRequest, nil, err)
			return
		}
		record, err := h.resources.Assign(user, kind, r.PathValue("id"), input.AssigneeID, input.Claim)
		respond(w, http.StatusOK, record, err)
	}
}

func filterAssignments(records []core.Record, userID, mode string) []core.Record {
	if mode != "mine" && mode != "unassigned" {
		return records
	}
	filtered := make([]core.Record, 0, len(records))
	for _, record := range records {
		if (mode == "mine" && record.AssigneeID == userID) || (mode == "unassigned" && record.AssigneeID == "") {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

func (h *Handler) readConversation(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	var input struct {
		Sequence uint64 `json:"sequence"`
	}
	if err := decode(w, r, &input); err != nil {
		respond(w, http.StatusBadRequest, nil, err)
		return
	}
	respond(w, http.StatusOK, nil, h.resources.ReadConversation(user, r.PathValue("id"), input.Sequence))
}

func (h *Handler) updateMember(w http.ResponseWriter, r *http.Request, owner core.User, _ string) {
	var input struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Disabled bool   `json:"disabled"`
	}
	if err := decode(w, r, &input); err != nil {
		respond(w, http.StatusBadRequest, nil, err)
		return
	}
	user, err := h.auth.UpdateMember(owner, r.PathValue("id"), input.Name, input.Email, input.Disabled)
	respond(w, http.StatusOK, publicUser(user), err)
}
