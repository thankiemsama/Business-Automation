package service

import (
	"net/http"
	"testing"
	"time"

	"businessautomation/internal/core"
)

func TestHandoffPolicyRoutesExplicitHumanRequestWithoutModel(t *testing.T) {
	ops, clock, owner := operationsFixture(t, time.Now())
	repo := ops.repo
	model := NewOpenAIKnowledge("fixture", "")
	model.client.Transport = webhookTransport(func(*http.Request) (*http.Response, error) {
		t.Fatal("explicit handoff must not contact the model")
		return nil, nil
	})
	if err := repo.UpdateTenant(owner.TenantID, func(store core.Writer) error {
		store.PutRecord(core.Record{ID: settingsID(owner.TenantID), TenantID: owner.TenantID, Kind: tenantSettingsKind,
			Content: `{"ai_handoff":{"enabled":true,"minimum_confidence":0.9}}`})
		store.PutRecord(core.Record{ID: "chat", TenantID: owner.TenantID, Kind: core.KindConversations, Channel: core.ChannelWeb, Status: core.StatusOpen})
		store.PutRecord(core.Record{ID: "session", TenantID: owner.TenantID, Kind: "chat_sessions", ConversationID: "chat"})
		store.PutRecord(core.Record{ID: "customer-message", TenantID: owner.TenantID, Kind: core.KindMessages,
			ConversationID: "chat", Sender: core.SenderCustomer, Content: "Tôi muốn gặp nhân viên.", CreatedAt: clock.Now()})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	NewAI(repo, clock, model, false, true).runChatbot()
	if err := repo.ViewTenant(owner.TenantID, func(store core.Reader) error {
		tickets := store.Records(owner.TenantID, core.KindTickets)
		traces, err := agentTraceBuffer(store, owner.TenantID, clock.Now())
		marker, exists := store.Record(owner.TenantID, tokenHash("chatbot:customer-message"), "ai_bot_markers")
		if len(tickets) != 1 || !exists || marker.Status != "handoff" || marker.MessageID != "customer-message" ||
			err != nil || len(traces) != 1 || traces[0].Outcome != "customer_requested_human" || traces[0].Policy == "" {
			t.Fatalf("handoff was not committed and traceable: %+v %+v %v", marker, traces, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
