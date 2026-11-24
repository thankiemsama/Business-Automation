package api

import (
	"errors"
	"net/http"

	"businessautomation/internal/core"
	"businessautomation/internal/service"
)

type authAction func(service.Credentials) (any, int, error)

// authEndpoint serves a guest credential endpoint. resolveCaptcha gates
// brute-force-prone actions (register/login/forgot/verify) behind the
// optional Turnstile check; IP is resolved server-side for the audit trail.
func (h *Handler) authEndpoint(action authAction, resolveCaptcha bool) http.HandlerFunc {
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
		c.IP = requestIP(r).String()
		if resolveCaptcha {
			if err := h.captcha.Verify(c.CAPTCHA); err != nil {
				respond(w, http.StatusBadRequest, nil, err)
				return
			}
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

// confirmEmail redeems the 30-minute mailbox-verification link (G1).
func (h *Handler) confirmEmail(credentials service.Credentials) (any, int, error) {
	return nil, http.StatusOK, h.auth.ConfirmEmail(credentials.Token)
}

// resendVerification re-issues the pending verification link (G1). Unknown or
// already-confirmed emails return success so the endpoint does not disclose
// which mailboxes exist, mirroring /auth/forgot. Only a missing mail
// provider (503) is surfaced: the form shows the "chưa cấu hình" state.
func (h *Handler) resendVerification(credentials service.Credentials) (any, int, error) {
	err := h.auth.SendVerification(credentials.Email)
	var apiErr *core.Error
	if err != nil && (errors.As(err, &apiErr) && apiErr.Status != http.StatusNotFound) {
		return nil, apiErr.Status, err
	}
	return nil, http.StatusOK, nil
}

// issueStepUp mints a 5-minute single-use token for sensitive operations (G8).
func (h *Handler) issueStepUp(w http.ResponseWriter, r *http.Request, user core.User, _ string) {
	if !h.allowAuth(r) {
		respond(w, http.StatusTooManyRequests, nil, core.Fail(http.StatusTooManyRequests, "Vui lòng thử lại sau"))
		return
	}
	if !h.acquireAuthSlot(w) {
		return
	}
	defer h.releaseAuthSlot()
	var input struct {
		Password string `json:"password"`
	}
	if err := decode(w, r, &input); err != nil {
		respond(w, http.StatusBadRequest, nil, err)
		return
	}
	token, err := h.auth.IssueStepUp(user, input.Password)
	if err != nil {
		respond(w, http.StatusForbidden, nil, err)
		return
	}
	respond(w, http.StatusOK, map[string]any{"token": token, "expires_in": 300}, nil)
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
