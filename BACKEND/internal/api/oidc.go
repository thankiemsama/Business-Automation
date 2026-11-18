package api

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"businessautomation/internal/core"
	"businessautomation/internal/service"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

type oidcAccounts interface {
	IssueOIDC(string, string) (string, error)
	ExchangeOIDC(string, string) (string, error)
}

func oidcSettings(name string) (oauth2.Config, string, bool) {
	if name != "google" && name != "okta" && name != "microsoft" {
		return oauth2.Config{}, "", false
	}
	prefix := "OIDC_" + strings.ToUpper(name) + "_"
	origin := strings.TrimRight(os.Getenv("APP_ORIGIN"), "/")
	issuer, client, secret := os.Getenv(prefix+"ISSUER"), os.Getenv(prefix+"CLIENT_ID"), os.Getenv(prefix+"CLIENT_SECRET")
	if name == "google" && issuer == "" {
		issuer = "https://accounts.google.com"
	}
	validated, err := url.Parse(issuer)
	if err != nil || validated.Scheme != "https" || validated.Hostname() == "" || validated.User != nil || origin == "" || client == "" || secret == "" {
		return oauth2.Config{}, "", false
	}
	return oauth2.Config{ClientID: client, ClientSecret: secret, RedirectURL: origin + "/api/oidc/" + name + "/callback", Scopes: []string{oidc.ScopeOpenID}}, issuer, true
}
func (h *Handler) oidcRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/oidc/providers", func(w http.ResponseWriter, r *http.Request) {
		providers := []string{}
		for _, name := range []string{"google", "okta", "microsoft"} {
			if _, _, enabled := oidcSettings(name); enabled {
				providers = append(providers, name)
			}
		}
		respond(w, 200, providers, nil)
	})
	mux.HandleFunc("GET /api/v1/oidc/{provider}/{step}", h.oidcLogin)
	mux.HandleFunc("/api/v1/auth/oidc-exchange", h.authEndpoint(func(c service.Credentials) (any, int, error) {
		auth, ok := h.auth.(oidcAccounts)
		if !ok {
			return nil, 503, core.Fail(503, "SSO chưa sẵn sàng")
		}
		token, err := auth.ExchangeOIDC(c.Token, c.Code)
		return tokenResponse{AccessToken: token, TokenType: "Bearer", ExpiresIn: int(core.SessionLifetime.Seconds())}, 200, err
	}))
}

func (h *Handler) oidcLogin(w http.ResponseWriter, r *http.Request) {
	name, step := r.PathValue("provider"), r.PathValue("step")
	cfg, issuer, enabled := oidcSettings(name)
	if !enabled || (step != "start" && step != "callback") {
		respond(w, 404, nil, core.Fail(404, "SSO chưa cấu hình"))
		return
	}
	if !h.allowAuth(r) {
		respond(w, 429, nil, core.Fail(429, "Vui lòng thử lại sau"))
		return
	}
	origin := strings.TrimRight(os.Getenv("APP_ORIGIN"), "/")
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		respond(w, 502, nil, core.Fail(502, "Không kết nối được nhà cung cấp SSO"))
		return
	}
	cfg.Endpoint = provider.Endpoint()
	if step == "start" {
		ids := service.RandomIDs{}
		state, nonce, verifier := ids.NewID(), ids.NewID(), oauth2.GenerateVerifier()
		http.SetCookie(w, &http.Cookie{Name: "nexa_oidc", Value: strings.Join([]string{name, state, nonce, verifier}, "."), Path: "/api/oidc", HttpOnly: true, Secure: strings.HasPrefix(origin, "https://"), SameSite: http.SameSiteLaxMode, MaxAge: 300})
		http.Redirect(w, r, cfg.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), http.StatusSeeOther)
		return
	}
	stored, err := r.Cookie("nexa_oidc")
	if err != nil {
		respond(w, 403, nil, core.Fail(403, "Phiên SSO không hợp lệ"))
		return
	}
	parts := strings.Split(stored.Value, ".")
	if len(parts) != 4 || parts[0] != name || parts[1] != r.URL.Query().Get("state") || r.URL.Query().Get("code") == "" {
		respond(w, 403, nil, core.Fail(403, "Phiên SSO không hợp lệ"))
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "nexa_oidc", Value: "", Path: "/api/oidc", HttpOnly: true, Secure: strings.HasPrefix(origin, "https://"), SameSite: http.SameSiteLaxMode, MaxAge: -1})
	access, err := cfg.Exchange(ctx, r.URL.Query().Get("code"), oauth2.VerifierOption(parts[3]))
	if err != nil {
		respond(w, 403, nil, core.Fail(403, "Không đổi được mã SSO"))
		return
	}
	raw, _ := access.Extra("id_token").(string)
	token, err := provider.Verifier(&oidc.Config{ClientID: cfg.ClientID}).Verify(ctx, raw)
	if err != nil || token.Nonce != parts[2] || token.Subject == "" {
		respond(w, 403, nil, core.Fail(403, "Không xác thực được danh tính SSO"))
		return
	}
	auth, ok := h.auth.(oidcAccounts)
	if !ok {
		respond(w, 503, nil, core.Fail(503, "SSO chưa sẵn sàng"))
		return
	}
	ticket, err := auth.IssueOIDC(token.Issuer, token.Subject)
	if err != nil {
		respond(w, 403, nil, err)
		return
	}
	http.Redirect(w, r, origin+"/sso/complete#"+url.PathEscape(ticket), http.StatusSeeOther)
}
