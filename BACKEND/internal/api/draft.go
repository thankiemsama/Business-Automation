package api

import (
	"businessautomation/internal/core"
	"net/http"
	"time"
)

func (h *Handler) draft(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	if !h.allowAI(w, user.ID, 30, time.Minute) {
		return
	}
	if !h.acquireAuthSlot(w) {
		return
	}
	defer h.releaseAuthSlot()
	draft, err := h.conversations.Draft(user, r.PathValue("id"))
	respond(w, http.StatusOK, draft, err)
}
