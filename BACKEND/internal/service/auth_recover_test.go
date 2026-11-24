package service

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"businessautomation/internal/core"
	"businessautomation/internal/repository"
)

type recoveryClock struct{ now time.Time }

func (c *recoveryClock) Now() time.Time { return c.now }

type recoveryCapture struct {
	calls  int
	bodies []string
}

func newRecoveryMailer(capture *recoveryCapture) *EmailMailer {
	return &EmailMailer{
		key: "fake", from: "support@example.test",
		client: &http.Client{Transport: webhookTransport(func(r *http.Request) (*http.Response, error) {
			capture.calls++
			body, err := io.ReadAll(r.Body)
			if err != nil {
				return nil, err
			}
			capture.bodies = append(capture.bodies, string(body))
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":"mail-1"}`))}, nil
		})},
	}
}

// recoveryLink extracts the raw "tenant.token" link from the last captured mail.
func recoveryLink(t *testing.T, capture *recoveryCapture) string {
	t.Helper()
	payload := struct {
		To   []string `json:"to"`
		Text string   `json:"text"`
	}{}
	if err := json.Unmarshal([]byte(capture.bodies[len(capture.bodies)-1]), &payload); err != nil {
		t.Fatalf("mail body is not json: %v", err)
	}
	lines := strings.Split(payload.Text, "\r\n")
	parsed, err := url.Parse(lines[len(lines)-1])
	if err != nil {
		t.Fatalf("mail has no valid link: %q", payload.Text)
	}
	// sendLink percent-encodes the tenant token; unescape before splitting.
	fragment, err := url.PathUnescape(parsed.Fragment)
	if err != nil {
		t.Fatalf("bad fragment %q: %v", parsed.Fragment, err)
	}
	tenant, secret, ok := strings.Cut(fragment, ".")
	if !ok || tenant == "" || secret == "" {
		t.Fatalf("link fragment is not tenant.token: %q", fragment)
	}
	return tenant + "." + secret
}

func TestPasswordRecoveryIssuesAndRedeemsSingleUseLink(t *testing.T) {
	repo, err := repository.Open(filepath.Join(t.TempDir(), "recovery.json"))
	if err != nil {
		t.Fatal(err)
	}
	clock := &recoveryClock{now: time.Date(2026, 11, 22, 8, 0, 0, 0, time.UTC)}
	ids := RandomIDs{}
	auth := NewAuth(repo, clock, ids, PBKDF2Hasher{})
	if _, err := auth.Register(Credentials{Name: "Owner", Email: "owner@example.test", Password: "original-password-12"}); err != nil {
		t.Fatal(err)
	}
	capture := &recoveryCapture{}
	auth.ConfigureEmail(newRecoveryMailer(capture), "https://nexa.example")

	if err := auth.SendReset("owner@example.test"); err != nil {
		t.Fatal(err)
	}
	if capture.calls != 1 {
		t.Fatalf("expected exactly one mail, got %d", capture.calls)
	}
	fullToken := recoveryLink(t, capture)

	// An expired link is rejected even though the password is valid.
	clock.now = clock.now.Add(31 * time.Minute)
	if err := auth.Redeem(fullToken, "new-password-12345"); err == nil {
		t.Fatal("expired recovery link was accepted")
	}
	// Back inside the 30 minute window: the link works once...
	clock.now = clock.now.Add(-16 * time.Minute)
	if err := auth.Redeem(fullToken, "new-password-12345"); err != nil {
		t.Fatalf("redeem failed: %v", err)
	}
	// ...and the new password replaces the old one.
	if _, err := auth.Login(Credentials{Email: "owner@example.test", Password: "new-password-12345"}); err != nil {
		t.Fatalf("login with new password failed: %v", err)
	}
	if _, err := auth.Login(Credentials{Email: "owner@example.test", Password: "original-password-12"}); err == nil {
		t.Fatal("old password still works after recovery")
	}
	// Redeem is single-use: replaying the same link fails.
	if err := auth.Redeem(fullToken, "another-password-12"); err == nil {
		t.Fatal("reused recovery link was accepted")
	}
}

func TestPasswordRecoveryUnknownEmailSendsNoMail(t *testing.T) {
	repo, err := repository.Open(filepath.Join(t.TempDir(), "recovery.json"))
	if err != nil {
		t.Fatal(err)
	}
	clock := &recoveryClock{now: time.Now().UTC()}
	auth := NewAuth(repo, clock, RandomIDs{}, PBKDF2Hasher{})
	capture := &recoveryCapture{}
	auth.ConfigureEmail(newRecoveryMailer(capture), "https://nexa.example")

	// Unknown emails must not disclose existence: success, zero mail.
	if err := auth.SendReset("ghost@example.test"); err != nil {
		t.Fatal(err)
	}
	if capture.calls != 0 {
		t.Fatalf("mail sent for unknown email: %d", capture.calls)
	}
}

func TestPasswordRecoveryWithoutMailerIsUnavailable(t *testing.T) {
	repo, err := repository.Open(filepath.Join(t.TempDir(), "recovery.json"))
	if err != nil {
		t.Fatal(err)
	}
	auth := NewAuth(repo, &recoveryClock{now: time.Now().UTC()}, RandomIDs{}, PBKDF2Hasher{})
	err = auth.SendReset("owner@example.test")
	var apiErr *core.Error
	if !errors.As(err, &apiErr) || apiErr.Status != 503 {
		t.Fatalf("expected 503 without mailer, got %v", err)
	}
}

// With NEXA_DEV_EMAIL_SINK enabled, recovery must succeed even though no
// mail provider is configured: the link is written to the backend log.
func TestPasswordRecoveryDevEmailSink(t *testing.T) {
	t.Setenv("NEXA_DEV_EMAIL_SINK", "true")
	t.Setenv("RESEND_API_KEY", "")
	t.Setenv("EMAIL_FROM", "")
	mailer := EmailFromEnvironment()
	if mailer == nil || !mailer.devSink {
		t.Fatal("dev sink mailer was not created")
	}
	repo, err := repository.Open(filepath.Join(t.TempDir(), "recovery.json"))
	if err != nil {
		t.Fatal(err)
	}
	clock := &recoveryClock{now: time.Now().UTC()}
	auth := NewAuth(repo, clock, RandomIDs{}, PBKDF2Hasher{})
	if _, err := auth.Register(Credentials{Name: "Owner", Email: "owner@example.test", Password: "original-password-12"}); err != nil {
		t.Fatal(err)
	}
	auth.ConfigureEmail(mailer, "https://nexa.example")
	if err := auth.SendReset("owner@example.test"); err != nil {
		t.Fatalf("dev sink should let SendReset succeed: %v", err)
	}
}
