package service

import (
	"testing"
	"time"

	"businessautomation/internal/core"
)

func TestBrandCardIsAttachedOnceToHumanReply(t *testing.T) {
	ops, clock, owner := operationsFixture(t, time.Now())
	repo := ops.repo
	conversations := NewConversations(repo, clock, repo.IDs, LocalKnowledgeProvider{}, NewKeywordAutomation(repo.IDs, clock))
	if err := repo.UpdateTenant(owner.TenantID, func(store core.Writer) error {
		store.PutRecord(core.Record{ID: settingsID(owner.TenantID), TenantID: owner.TenantID, Kind: tenantSettingsKind,
			Content: `{"display_name":"Nexa Support","brand_card":{"enabled":true,"intro":"Hỗ trợ khách hàng","product_url":"https://example.com/products"}}`})
		store.PutRecord(core.Record{ID: "thread", TenantID: owner.TenantID, Kind: core.KindConversations, Channel: core.ChannelWeb, Status: core.StatusOpen})
		store.PutRecord(core.Record{ID: tokenHash("fixture-token"), TenantID: owner.TenantID, Kind: "chat_sessions", ConversationID: "thread"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, role := range []string{"note", core.SenderAgent, core.SenderAgent} {
		if _, err := conversations.Message(owner, "thread", "Nội dung hỗ trợ", role); err != nil {
			t.Fatal(err)
		}
	}
	messages, err := conversations.PublicChat(owner.TenantID+".fixture-token", "", "", "", 0)
	if err != nil || len(messages) != 2 {
		t.Fatalf("unexpected guest messages: %+v, %v", messages, err)
	}
	if card := messages[0].BrandCard; card == nil || card.Name != "Nexa Support" || card.ProductURL != "https://example.com/products" {
		t.Fatalf("incorrect card: %+v", card)
	}
	if messages[1].BrandCard != nil {
		t.Fatal("brand card repeated on a subsequent reply")
	}
	if err := validateBrandCard("Nexa", &BrandCardSettings{Enabled: true, ProductURL: "javascript:alert(1)"}); err == nil {
		t.Fatal("non-HTTPS product link accepted")
	}
}
