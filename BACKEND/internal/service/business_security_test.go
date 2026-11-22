package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"businessautomation/internal/core"
)

func TestStripeSubscriptionAuthenticityAndTenantMembership(t *testing.T) {
	now := time.Now().UTC()
	ops, clock, owner := operationsFixture(t, now)
	admin := NewAdmin(ops.repo, clock)
	t.Setenv("STRIPE_WEBHOOK_SECRET", "synthetic-stripe-secret")
	t.Setenv("STRIPE_PRO_PRICE", "price_test_pro")
	sign := func(payload []byte) string {
		mac := hmac.New(sha256.New, []byte("synthetic-stripe-secret"))
		_, _ = fmt.Fprintf(mac, "%d.", now.Unix())
		mac.Write(payload)
		return fmt.Sprintf("t=%d,v1=%s", now.Unix(), hex.EncodeToString(mac.Sum(nil)))
	}
	event := func(id, tenant, kind, status string) []byte {
		body, _ := json.Marshal(map[string]any{"id": id, "type": kind, "created": now.Unix(), "data": map[string]any{"object": map[string]any{
			"id": "sub_demo", "status": status, "customer": "cus_demo", "metadata": map[string]string{"nexa_tenant": tenant},
			"items": map[string]any{"data": []any{map[string]any{"price": map[string]string{"id": "price_test_pro"}}}},
		}}})
		return body
	}
	created := event("evt_created", owner.TenantID, "customer.subscription.created", "active")
	if err := admin.StripeEvent("t=0,v1=invalid", created); err == nil {
		t.Fatal("invalid signature passed")
	}
	if err := admin.StripeEvent(sign(created), created); err != nil {
		t.Fatal(err)
	}
	if err := admin.StripeEvent(sign(created), created); err != nil {
		t.Fatal(err)
	}
	usage, err := admin.Usage(owner.TenantID)
	if err != nil || usage["plan"] != "pro" {
		t.Fatal(usage, err)
	}
	if err := ops.repo.View(func(store core.Reader) error {
		if len(store.Records(owner.TenantID, "billing_events")) != 1 {
			t.Fatal("event replay wrote duplicates")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	deleted := event("evt_delete", owner.TenantID, "customer.subscription.deleted", "canceled")
	if err := admin.StripeEvent(sign(deleted), deleted); err != nil {
		t.Fatal(err)
	}
	usage, err = admin.Usage(owner.TenantID)
	if err != nil || usage["plan"] != "free" {
		t.Fatal(usage, err)
	}
}

func TestOIDCExchangeIsBoundToIssuerAndConsumed(t *testing.T) {
	now := time.Now().UTC()
	ops, clock, owner := operationsFixture(t, now)
	auth := NewAuth(ops.repo, clock, ops.repo.IDs, adminTestHasher{})
	t.Setenv("NEXA_OIDC_BINDINGS", `{"https://issuer.example.test|subject":"owner@example.test"}`)
	if _, err := auth.IssueOIDC("https://other.example.test", "subject"); err == nil {
		t.Fatal("wrong issuer accepted")
	}
	exchange, err := auth.IssueOIDC("https://issuer.example.test", "subject")
	if err != nil {
		t.Fatal(err)
	}
	token, err := auth.ExchangeOIDC(exchange, "")
	if err != nil {
		t.Fatal(err)
	}
	user, err := auth.Authenticate(token)
	if err != nil || user.ID != owner.ID {
		t.Fatal(user, err)
	}
	if _, err := auth.ExchangeOIDC(exchange, ""); err == nil {
		t.Fatal("SSO exchange token reused")
	}
}
