package service

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"businessautomation/internal/core"
)

// Turnstile verifies Cloudflare Turnstile proof-of-human tokens. When
// NEXA_TURNSTILE_SECRET is unset the verifier is nil and captcha is disabled.
type Turnstile struct {
	secret   string
	endpoint string
	client   *http.Client
}

// TurnstileFromEnvironment returns nil (captcha off) unless
// NEXA_TURNSTILE_SECRET is set. NEXA_TURNSTILE_VERIFY_URL overrides the verify
// endpoint (used by tests and self-hosted verifiers).
func TurnstileFromEnvironment() *Turnstile {
	secret := os.Getenv("NEXA_TURNSTILE_SECRET")
	if secret == "" {
		return nil
	}
	endpoint := os.Getenv("NEXA_TURNSTILE_VERIFY_URL")
	if endpoint == "" {
		endpoint = "https://challenges.cloudflare.com/turnstile/v0/siteverify"
	}
	return &Turnstile{secret: secret, endpoint: endpoint, client: &http.Client{Timeout: 5 * time.Second}}
}

// Verify returns nil when captcha is disabled or when the token is valid.
func (t *Turnstile) Verify(token string) error {
	if t == nil {
		return nil
	}
	if token == "" {
		return core.FailReason(400, "captcha_missing", "Thiếu xác minh captcha, vui lòng mở lại trang.")
	}
	form := url.Values{"secret": {t.secret}, "response": {token}}
	response, err := t.client.PostForm(t.endpoint, form)
	if err != nil {
		return core.Fail(502, "Không xác minh được captcha, vui lòng thử lại.")
	}
	defer response.Body.Close()
	var out struct {
		Success bool `json:"success"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&out); err != nil || !out.Success {
		return core.FailReason(400, "captcha_invalid", "Captcha không hợp lệ, vui lòng thử lại.")
	}
	return nil
}
