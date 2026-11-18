package api

import (
	"net/http"

	"businessautomation/internal/core"
)

func (h *Handler) accountAction(action string) authenticatedHandler {
	return func(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
		if !h.allowAuth(r) {
			respond(w, http.StatusTooManyRequests, nil, core.Fail(429, "Vui lòng thử lại sau"))
			return
		}
		if !h.acquireAuthSlot(w) {
			return
		}
		defer h.releaseAuthSlot()
		var input struct {
			OldPassword string `json:"old_password"`
			Password    string `json:"password"`
			Name        string `json:"name"`
			Email       string `json:"email"`
		}
		if err := decode(w, r, &input); err != nil {
			respond(w, http.StatusBadRequest, nil, err)
			return
		}
		var err error
		if action == "invite" {
			err = h.auth.Invite(user, input.Name, input.Email)
		} else {
			err = h.auth.ChangePassword(user, input.OldPassword, input.Password)
		}
		respond(w, http.StatusOK, nil, err)
	}
}
