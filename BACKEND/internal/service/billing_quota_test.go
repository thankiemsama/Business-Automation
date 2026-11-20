package service

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"

	"businessautomation/internal/core"
)

func TestBillingQuotaBlocksAtomicallyAndCreditsPaidInvoiceOnce(t *testing.T) {
	now := time.Date(2026, 11, 20, 2, 0, 0, 0, time.UTC)
	operations, clock, owner := operationsFixture(t, now)
	workspace := operations.repo
	admin := NewAdmin(workspace, clock)
	t.Setenv("NEXA_BILLING_PRO_TICKETS_LIMIT", "1")
	t.Setenv("NEXA_BILLING_PRO_AI_LIMIT", "1")
	t.Setenv("NEXA_BILLING_PRO_TICKETS_INVOICE_CREDITS", "1")
	t.Setenv("STRIPE_PRO_PRICE", "price_pro")
	t.Setenv("STRIPE_WEBHOOK_SECRET", "fixture-secret")
	if err := workspace.UpdateTenant(owner.TenantID, func(store core.Writer) error {
		store.PutRecord(core.Record{ID: tokenHash("plan:" + owner.TenantID), Kind: "billing_plan",
			TenantID: owner.TenantID, Name: "sub_fixture", CustomerID: "cus_fixture", Status: "pro", Enabled: true})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewWebhooks(workspace, clock, workspace.IDs).Save(owner, "", Webhook{
		Name: "Quota alerts", URL: "https://example.com/billing", Events: []string{core.EventBillingQuotaLow},
	}); err != nil {
		t.Fatal(err)
	}
	create := func(id string) error {
		return workspace.UpdateTenant(owner.TenantID, func(store core.Writer) error {
			store.PutRecord(core.Record{ID: "side-effect-" + id, TenantID: owner.TenantID, Kind: "fixture"})
			store.PutRecord(core.Record{ID: id, TenantID: owner.TenantID, Kind: core.KindTickets, CreatedAt: now})
			return nil
		})
	}
	if err := create("first"); err != nil {
		t.Fatal(err)
	}
	if err := create("blocked"); !errors.Is(err, errBillingQuota) {
		t.Fatalf("expected ticket quota error, got %v", err)
	}
	model := NewOpenAIKnowledge("fixture", "")
	model.ConfigureUsage(workspace, clock, workspace.IDs)
	if err := model.reserveRequest(owner.TenantID); err != nil {
		t.Fatal(err)
	}
	if err := model.reserveRequest(owner.TenantID); !errors.Is(err, errBillingQuota) {
		t.Fatalf("expected AI quota error, got %v", err)
	}
	for _, eventID := range []string{"evt_a", "evt_b"} {
		body := []byte(fmt.Sprintf(`{"id":%q,"type":"invoice.paid","created":%d,"data":{"object":{
			"id":"in_fixture","customer":"cus_fixture","status":"paid","amount_paid":1000,"billing_reason":"subscription_cycle",
			"parent":{"subscription_details":{"subscription":"sub_fixture","metadata":{"nexa_tenant":%q}}},
			"lines":{"data":[{"pricing":{"price_details":{"price":"price_pro"}}}]}}}}`, eventID, now.Unix(), owner.TenantID))
		mac := hmac.New(sha256.New, []byte("fixture-secret"))
		fmt.Fprintf(mac, "%d.", now.Unix())
		mac.Write(body)
		signature := fmt.Sprintf("t=%d,v1=%s", now.Unix(), hex.EncodeToString(mac.Sum(nil)))
		if err := admin.StripeEvent(signature, body); err != nil {
			t.Fatal(err)
		}
	}
	if err := create("overage"); err != nil {
		t.Fatal(err)
	}
	if err := create("blocked-again"); !errors.Is(err, errBillingQuota) {
		t.Fatalf("duplicate invoice added quota: %v", err)
	}
	if err := workspace.ViewTenant(owner.TenantID, func(store core.Reader) error {
		if _, found := store.Record(owner.TenantID, "side-effect-blocked", "fixture"); found {
			t.Error("failed quota transaction committed a side effect")
		}
		quota, _ := billingQuota(store, owner.TenantID, billingTickets, now)
		if !quota.Low || quota.Used != 2 || quota.Credits != 1 || quota.Overage != 1 ||
			len(store.Records(owner.TenantID, core.KindWebhookDeliveries)) != 3 {
			t.Errorf("unexpected quota or outbox: %+v", quota)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
