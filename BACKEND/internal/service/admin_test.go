package service

import (
	"encoding/base64"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"businessautomation/internal/core"
	"businessautomation/internal/repository"
)

type adminTestHasher struct{}

func (adminTestHasher) Hash(p, s string) string { return tokenHash(p + s) }

func TestMFAChallengeReplayAndRecovery(t *testing.T) {
	t.Setenv("NEXA_SECURITY_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	disk, err := repository.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	clock := &workspaceClock{now: time.Now().UTC()}
	ids := RandomIDs{}
	workspace := NewWorkspace(disk, clock, ids)
	auth := NewAuth(workspace, clock, ids, adminTestHasher{})
	credentials := Credentials{Name: "Agent", Email: "agent@example.test", Password: "synthetic-local-password"}
	user, err := auth.Register(credentials)
	if err != nil {
		t.Fatal(err)
	}
	first, err := auth.Login(credentials)
	if err != nil {
		t.Fatal(err)
	}
	other, err := auth.Login(credentials)
	if err != nil {
		t.Fatal(err)
	}
	setup, err := auth.MFA(user, "setup", credentials.Password, "", first)
	if err != nil {
		t.Fatal(err)
	}
	secret := setup["secret"].(string)
	raw, err := noPadding.DecodeString(secret)
	if err != nil {
		t.Fatal(err)
	}
	enabled, err := auth.MFA(user, "enable", "", totpCode(raw, clock.now.Unix()/30), first)
	if err != nil || len(enabled["recovery_codes"].([]string)) != 8 {
		t.Fatal(err)
	}
	if _, err := auth.Authenticate(other); err == nil {
		t.Fatal("other sessions remained active")
	}
	if _, err := auth.Login(credentials); err == nil {
		t.Fatal("password-only login passed")
	}
	credentials.Code = totpCode(raw, clock.now.Unix()/30)
	if _, err := auth.Login(credentials); err == nil {
		t.Fatal("enrollment code replay passed")
	}
	clock.now = clock.now.Add(30 * time.Second)
	credentials.Code = totpCode(raw, clock.now.Unix()/30)
	if _, err := auth.Login(credentials); err != nil {
		t.Fatal(err)
	}
	credentials.Code = enabled["recovery_codes"].([]string)[0]
	if _, err := auth.Login(credentials); err != nil {
		t.Fatal(err)
	}
	if _, err := auth.Login(credentials); err == nil {
		t.Fatal("recovery code reused")
	}
	if err := disk.View(func(store core.Reader) error {
		actual, _ := store.UserByID(user.ID)
		if actual.TOTPSecret == secret || strings.Contains(actual.RecoveryHashes, credentials.Code) {
			t.Fatal("credential stored in plaintext")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAdminCustomAuditAndBackupIsolation(t *testing.T) {
	t.Setenv("NEXA_SECURITY_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	disk, err := repository.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	clock := &workspaceClock{now: time.Now().UTC()}
	workspace := NewWorkspace(disk, clock, RandomIDs{})
	admin := NewAdmin(workspace, clock)
	user := core.User{ID: "owner", TenantID: "tenant", Role: core.RoleOwner}
	outsider := core.User{ID: "other", TenantID: "other", Role: core.RoleOwner}
	_, err = admin.SaveSettings(user, TenantSettings{Allowlist: []string{"192.0.2.0/24"}, Flags: map[string]bool{"ai": false}}, netip.MustParseAddr("192.0.2.1"))
	if err != nil {
		t.Fatal(err)
	}
	if admin.Allowed(user.TenantID, netip.MustParseAddr("203.0.113.2")) || admin.Feature(user.TenantID, "ai") || !admin.Allowed(outsider.TenantID, netip.MustParseAddr("203.0.113.2")) {
		t.Fatal("tenant settings scope")
	}
	field, err := admin.SaveField(user, CustomField{Kind: core.KindCustomers, Name: "Age", Type: "number", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	resources := NewResources(workspace, clock, workspace.IDs, DefaultPolicies())
	customer, err := resources.Save(user, core.KindCustomers, "", core.Record{Name: "Synthetic", Email: "private@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Custom(user, core.KindCustomers, customer.ID, map[string]string{field.ID: "NaN"}); err == nil {
		t.Fatal("invalid number passed")
	}
	if _, err := admin.Custom(user, core.KindCustomers, customer.ID, map[string]string{field.ID: "12"}); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Custom(outsider, core.KindCustomers, customer.ID, nil); err == nil {
		t.Fatal("cross-tenant read passed")
	}
	records, err := admin.Audit(user)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, record := range records {
		if strings.Contains(record.Content, "private@example.test") {
			t.Fatal("audit leaked PII")
		}
		if record.ConversationID == customer.ID && record.AuthorID == user.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("no attributed audit")
	}
	data, err := admin.Export(user, customer.ID)
	if err != nil {
		t.Fatal(err)
	}
	backup, err := admin.Backup(user, data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OpenTenantBackup(outsider.TenantID, backup); err == nil {
		t.Fatal("backup unlocked for wrong tenant")
	}
	restored, err := OpenTenantBackup(user.TenantID, backup)
	if err != nil || !strings.Contains(string(restored), "private@example.test") {
		t.Fatal(err)
	}
}
