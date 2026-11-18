package api

import (
	"net/http"

	"businessautomation/internal/core"
	"businessautomation/internal/service"
)

type authAction func(service.Credentials) (any, int, error)

func (h *Handler) authEndpoint(action authAction) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			method(w, http.MethodPost)
			return
		}
		if !h.allowAuth(r) {
			respond(w, http.StatusTooManyRequests, nil, core.Fail(http.StatusTooManyRequests, "Quá nhiều yêu cầu, thử lại sau một phút"))
			return
		}
		if !h.acquireAuthSlot(w) {
			return
		}
		defer h.releaseAuthSlot()
		var c service.Credentials
		if err := decode(w, r, &c); err != nil {
			respond(w, http.StatusBadRequest, nil, err)
			return
		}
		data, status, err := action(c)
		respond(w, status, data, err)
	}
}

func (h *Handler) me(w http.ResponseWriter, r *http.Request, u core.User, _ string) {
	if r.Method != http.MethodGet {
		method(w, http.MethodGet)
		return
	}
	respond(w, http.StatusOK, publicUser(u), nil)
}

func (h *Handler) logout(w http.ResponseWriter, r *http.Request, _ core.User, token string) {
	if r.Method != http.MethodPost {
		method(w, http.MethodPost)
		return
	}
	respond(w, http.StatusOK, nil, h.auth.Logout(token))
}

func (h *Handler) team(w http.ResponseWriter, r *http.Request, u core.User, _ string) {
	if r.Method == http.MethodGet {
		users, err := h.auth.Members(u)
		members := make([]UserResponse, 0, len(users))
		for _, user := range users {
			members = append(members, publicUser(user))
		}
		respond(w, http.StatusOK, members, err)
		return
	}
	if r.Method != http.MethodPost {
		method(w, http.MethodPost)
		return
	}
	if u.Role != core.RoleOwner {
		respond(w, http.StatusForbidden, nil, core.Fail(http.StatusForbidden, "Chỉ owner được tạo agent"))
		return
	}
	// Agent creation also performs password hashing and shares the capacity guard.
	if !h.acquireAuthSlot(w) {
		return
	}
	defer h.releaseAuthSlot()
	var c service.Credentials
	if err := decode(w, r, &c); err != nil {
		respond(w, http.StatusBadRequest, nil, err)
		return
	}
	agent, err := h.auth.AddAgent(u, c)
	respond(w, http.StatusCreated, publicUser(agent), err)
}

func (h *Handler) stats(w http.ResponseWriter, r *http.Request, u core.User, _ string) {
	if r.Method != http.MethodGet {
		method(w, http.MethodGet)
		return
	}
	data, err := h.dashboard.Dashboard(u)
	respond(w, http.StatusOK, data, err)
}

func (h *Handler) register(credentials service.Credentials) (any, int, error) {
	user, err := h.auth.Register(credentials)
	return publicUser(user), http.StatusCreated, err
}

func (h *Handler) login(credentials service.Credentials) (any, int, error) {
	token, err := h.auth.Login(credentials)
	return tokenResponse{AccessToken: token, TokenType: "Bearer", ExpiresIn: int(core.SessionLifetime.Seconds())}, http.StatusOK, err
}

type tokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}
