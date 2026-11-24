package service

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"businessautomation/internal/core"
	"businessautomation/internal/repository"
)

// TestInviteRedeemLifecycle covers the full invite → redeem → agent-login
// flow, plus rejection edge cases. It re-uses the recovery-capture mailer and
// clock helpers from auth_recover_test.go.
func TestInviteRedeemLifecycle(t *testing.T) {
	repo, err := repository.Open(filepath.Join(t.TempDir(), "invite.json"))
	if err != nil {
		t.Fatal(err)
	}
	clock := &recoveryClock{now: time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)}
	ids := RandomIDs{}
	auth := NewAuth(repo, clock, ids, PBKDF2Hasher{})

	// Owner registers.
	owner, err := auth.Register(Credentials{Name: "Owner", Email: "owner@invite.test", Password: "owner-password-12"})
	if err != nil {
		t.Fatal(err)
	}

	capture := &recoveryCapture{}
	auth.ConfigureEmail(newRecoveryMailer(capture), "https://nexa.example")

	// --- 1. Agent cannot invite (403) ---
	agentUser, err := auth.AddAgent(owner, Credentials{Name: "Agent", Email: "agent1@invite.test", Password: "agent-pass-12345"})
	if err != nil {
		t.Fatal(err)
	}
	if err := auth.Invite(agentUser, "X", "x@invite.test"); err == nil {
		t.Fatal("agent invite should fail with 403")
	}

	// --- 2. Owner invites an agent; extract the token from the captured email ---
	err = auth.Invite(owner, "New Agent", "agent2@invite.test")
	if err != nil {
		t.Fatalf("owner invite: %v", err)
	}
	if capture.calls != 1 {
		t.Fatalf("expected 1 mail, got %d", capture.calls)
	}
	inviteToken := recoveryLink(t, capture)

	// Token format: tenantID.secret — verify tenant matches owner's tenant.
	tenant, secret, found := strings.Cut(inviteToken, ".")
	if !found || tenant == "" || secret == "" {
		t.Fatalf("bad invite token format: %q", inviteToken)
	}
	if tenant != owner.TenantID {
		t.Fatalf("invite token tenant %q != owner tenant %q", tenant, owner.TenantID)
	}

	// --- 3. Redeem with too-short password → 400 ---
	if err := auth.Redeem(inviteToken, "short"); err == nil {
		t.Fatal("redeem invite with 5-char password accepted")
	}

	// --- 4. Redeem with valid password → agent user created ---
	if err := auth.Redeem(inviteToken, "agent2-password-12"); err != nil {
		t.Fatalf("redeem invite: %v", err)
	}

	// --- 5. Created user has RoleAgent in owner's tenant ---
	var created core.User
	if err := repo.View(func(store core.Reader) error {
		var ok bool
		created, ok = store.UserByEmail("agent2@invite.test")
		if !ok {
			return fmt.Errorf("agent2 not found after redeem")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if created.Role != core.RoleAgent {
		t.Fatalf("role = %q, want %q", created.Role, core.RoleAgent)
	}
	if created.TenantID != owner.TenantID {
		t.Fatalf("tenant = %q, want %q", created.TenantID, owner.TenantID)
	}

	// --- 6. Agent can login with the password set during redeem ---
	if _, err := auth.Login(Credentials{Email: "agent2@invite.test", Password: "agent2-password-12"}); err != nil {
		t.Fatalf("agent2 login after invite redeem: %v", err)
	}

	// --- 7. Invite link is single-use: replay fails ---
	if err := auth.Redeem(inviteToken, "replay-password-12"); err == nil {
		t.Fatal("replayed invite link accepted")
	}

	// --- 8. Invite for an email that already has a user → 409 ---
	if err := auth.Invite(owner, "Dup", "agent2@invite.test"); err == nil {
		t.Fatal("invite for already-existing email accepted")
	}

	// --- 9. Expired invite (48 h lifetime): advance clock 49 h, redeem fails ---
	if err := auth.Invite(owner, "Agent3", "agent3@invite.test"); err != nil {
		t.Fatal(err)
	}
	inviteToken3 := recoveryLink(t, capture)
	clock.now = clock.now.Add(49 * time.Hour)
	if err := auth.Redeem(inviteToken3, "agent3-password-12"); err == nil {
		t.Fatal("expired (49 h) invite link accepted")
	}

	// --- 10. Invite with empty name → 400 (normalizeCredentials rejects) ---
	if err := auth.Invite(owner, "", "agent4@invite.test"); err == nil {
		t.Fatal("invite with empty name accepted")
	}

	// --- 11. Invite with malformed email → 400 ---
	if err := auth.Invite(owner, "Agent5", "not-an-email"); err == nil {
		t.Fatal("invite with malformed email accepted")
	}
}
