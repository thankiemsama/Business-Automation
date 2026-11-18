package api

import (
	"businessautomation/internal/core"
	"businessautomation/internal/service"
	"net/http"
)

func (h *Handler) push(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	if r.Method == http.MethodGet {
		respond(w, 200, map[string]string{"key": h.workspace.PushKey()}, nil)
		return
	}
	var input service.PushInput
	if err := decode(w, r, &input); err != nil {
		respond(w, 400, nil, err)
		return
	}
	respond(w, 200, nil, h.workspace.SavePush(user, input))
}
