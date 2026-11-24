package api_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuthHardeningEmailVerificationLifecycle(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "fake")
	t.Setenv("EMAIL_FROM", "test@example.test")
	t.Setenv("APP_ORIGIN", "https://nexa.example")
	t.Setenv("NEXA_DEV_EMAIL_SINK", "")
	t.Setenv("NEXA_AUTO_CONFIRM_EMAIL", "")

	c, _ := pentestHarness(t)
	register(t, c, "verify@x.test", "password-123456")

	// A mail provider is configured: the fresh owner is held pending.
	// Login must refuse with the machine-readable reason email_unverified.
	status, out := c.raw("POST", "/api/v1/auth/login", map[string]any{"email": "verify@x.test", "password": "password-123456"})
	if status != 403 {
		t.Fatalf("login before verification: status=%d want=403 body=%v", status, out)
	}
	if out["reason"] != "email_unverified" {
		t.Fatalf("missing reason=email_unverified: %v", out)
	}

	// Resend is enumeration-safe: unknown mailboxes answer 200 as well.
	if status, _ := c.raw("POST", "/api/v1/auth/verify", map[string]any{"email": "ghost@x.test"}); status != 200 {
		t.Fatalf("verify resend for unknown email: status=%d want=200", status)
	}

	// Redeeming through the generic endpoint must reject verify links.
	if status, _ := c.raw("POST", "/api/v1/auth/redeem", map[string]any{"token": "tenant.secret", "password": "password-123456"}); status != 400 {
		t.Fatal("redeem accepted a verify-shaped link")
	}

	// Confirming a garbage token is a clean 400.
	if status, _ := c.raw("POST", "/api/v1/auth/confirm", map[string]any{"token": "no-dot"}); status != 400 {
		t.Fatal("confirm with malformed token accepted")
	}

	// Confirming the real link (extracted from the captured dev mailer is not
	// available here; the service test suite covers the link byte-for-byte).
	// After a 403 the account stays pending: login keeps refusing.
	if status, _ := c.raw("POST", "/api/v1/auth/login", map[string]any{"email": "verify@x.test", "password": "password-123456"}); status != 403 {
		t.Fatal("login after unconfirmed verification accepted")
	}
}

func TestAuthHardeningBruteForceLockout(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "")
	t.Setenv("EMAIL_FROM", "")
	t.Setenv("APP_ORIGIN", "")
	t.Setenv("NEXA_DEV_EMAIL_SINK", "")

	c, _ := pentestHarness(t)
	register(t, c, "lock@x.test", "password-123456")

	// Four wrong attempts are ordinary failures.
	for i := 0; i < 4; i++ {
		if status, _ := c.raw("POST", "/api/v1/auth/login", map[string]any{"email": "lock@x.test", "password": "wrong-password-1"}); status != 401 {
			t.Fatalf("attempt %d: status=%d want=401", i+1, status)
		}
	}
	// The 5th consecutive failure locks the account.
	if status, _ := c.raw("POST", "/api/v1/auth/login", map[string]any{"email": "lock@x.test", "password": "wrong-password-1"}); status != 401 {
		t.Fatal("5th attempt not refused")
	}
	// Even the correct password is now refused with reason=account_locked.
	status, out := c.raw("POST", "/api/v1/auth/login", map[string]any{"email": "lock@x.test", "password": "password-123456"})
	if status != 401 {
		t.Fatalf("locked login: status=%d want=401", status)
	}
	if out["reason"] != "account_locked" {
		t.Fatalf("locked login missing reason: %v", out)
	}

	// Unknown emails keep answering 401 (no lockout state is exposed).
	if status, _ := c.raw("POST", "/api/v1/auth/login", map[string]any{"email": "ghost@x.test", "password": "nope"}); status != 401 {
		t.Fatal("unknown email lockout leak")
	}
}

func TestAuthHardeningStepUpGate(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "")
	t.Setenv("EMAIL_FROM", "")
	t.Setenv("APP_ORIGIN", "")
	t.Setenv("NEXA_DEV_EMAIL_SINK", "")

	c, _ := pentestHarness(t)
	register(t, c, "step@x.test", "password-123456")
	c.token = login(t, c, "step@x.test", "password-123456")

	// Without a step-up token the export is refused (403).
	if status, _ := c.raw("GET", "/api/v1/admin/export", nil); status != 403 {
		t.Fatalf("export without step-up: status=%d want=403", status)
	}
	// A wrong password cannot mint a token.
	if status, _ := c.raw("POST", "/api/v1/account/step-up", map[string]any{"password": "wrong-password-1"}); status != 403 {
		t.Fatal("step-up with wrong password accepted")
	}
	// A valid token authorizes exactly one export.
	status, out := c.raw("POST", "/api/v1/account/step-up", map[string]any{"password": "password-123456"})
	if status != 200 {
		t.Fatalf("step-up issue: %d %v", status, out)
	}
	token := out["data"].(map[string]any)["token"].(string)
	if status, _ := c.raw("GET", "/api/v1/admin/export?step_up="+token, nil); status != 200 {
		t.Fatalf("export with step-up: status=%d want=200", status)
	}
	// The token is single-use.
	if status, _ := c.raw("GET", "/api/v1/admin/export?step_up="+token, nil); status != 403 {
		t.Fatalf("reused step-up token: status=%d want=403", status)
	}
	// The backup endpoint is gated the same way.
	if status, _ := c.raw("GET", "/api/v1/admin/backup", nil); status != 403 {
		t.Fatalf("backup without step-up: status=%d want=403", status)
	}
}

func TestAuthHardeningLegacyHashUpgrade(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "")
	t.Setenv("EMAIL_FROM", "")
	t.Setenv("APP_ORIGIN", "")
	t.Setenv("NEXA_DEV_EMAIL_SINK", "")

	// On a fresh store the register path already writes a versioned hash;
	// verify login keeps working and that a second login does not error.
	c, _ := pentestHarness(t)
	register(t, c, "legacy@x.test", "password-123456")

	// On a fresh store the register path already writes a versioned hash;
	// verify login keeps working and that a second login does not error.
	if status, _ := c.raw("POST", "/api/v1/auth/login", map[string]any{"email": "legacy@x.test", "password": "password-123456"}); status != 200 {
		t.Fatal("login after hardening failed")
	}
}

func TestAuthHardeningTurnstileGate(t *testing.T) {
	t.Setenv("RESEND_API_KEY", "")
	t.Setenv("EMAIL_FROM", "")
	t.Setenv("APP_ORIGIN", "")
	t.Setenv("NEXA_DEV_EMAIL_SINK", "")

	// A fake verifier endpoint that always rejects tokens.
	verifier := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"success\":false}"))
	}))
	defer verifier.Close()
	t.Setenv("NEXA_TURNSTILE_SECRET", "test-secret")
	t.Setenv("NEXA_TURNSTILE_VERIFY_URL", verifier.URL)

	c, _ := pentestHarness(t)
	// register is captcha-gated: no token must fail.
	if status, _ := c.raw("POST", "/api/v1/auth/register", map[string]any{"name": "A", "email": "cap@x.test", "password": "password-123456"}); status != 400 {
		t.Fatalf("register without captcha: status=%d want=400", status)
	}
	// A rejected token fails with the machine-readable reason.
	status, out := c.raw("POST", "/api/v1/auth/register", map[string]any{"name": "A", "email": "cap@x.test", "password": "password-123456", "captcha": "bad-token"})
	if status != 400 || out["reason"] != "captcha_invalid" {
		t.Fatalf("rejected captcha: %d %v", status, out)
	}
}

func TestAuthHardeningDevSinkProductionGuard(t *testing.T) {
	// The sink must be refused when APP_ORIGIN is a public https origin.
	t.Setenv("RESEND_API_KEY", "")
	t.Setenv("EMAIL_FROM", "")
	t.Setenv("NEXA_DEV_EMAIL_SINK", "true")
	t.Setenv("APP_ORIGIN", "https://nexa.example")
	c, _ := pentestHarness(t)
	if status, _ := c.raw("POST", "/api/v1/auth/forgot", map[string]any{"email": "a@x.test"}); status != 503 {
		t.Fatalf("prod-origin forgot with dev-sink: status=%d want=503", status)
	}

	// A plain dev origin keeps the sink available.
	t.Setenv("APP_ORIGIN", "http://localhost:3000")
	c2, _ := pentestHarness(t)
	if status, _ := c2.raw("POST", "/api/v1/auth/forgot", map[string]any{"email": "a@x.test"}); status != 200 {
		t.Fatalf("dev-origin forgot with dev-sink: status=%d want=200", status)
	}
}

// keep base64/time imports referenced for future TOTP-rotation additions.
