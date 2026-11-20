package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"businessautomation/internal/core"
)

func testCompletion(output any) *http.Response {
	payload, _ := json.Marshal(output)
	envelope, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{{"finish_reason": "stop", "message": map[string]string{"content": string(payload)}}},
	})
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(envelope)))}
}

func TestExtractionPreservesExistingFieldsAndResumesAfterQuota(t *testing.T) {
	now := time.Date(2026, 11, 19, 23, 50, 0, 0, ReportZone)
	operations, clock, owner := operationsFixture(t, now)
	model := NewOpenAIKnowledge("test", "")
	model.ConfigureUsage(operations.repo, clock, RandomIDs{})
	calls := 0
	model.client.Transport = webhookTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return testCompletion(map[string]any{"values": map[string]string{"keep": "ghi đè", "quantity": "3", "unknown": "bỏ qua"}}), nil
	})
	operations.ConfigureAIJobs(model)
	admin := NewAdmin(operations.repo, clock)
	if err := operations.repo.Update(func(store core.Writer) error {
		store.PutRecord(core.Record{
			ID: settingsID(owner.TenantID), TenantID: owner.TenantID, Kind: tenantSettingsKind,
			Content: `{"daily_ai_requests":1}`,
		})
		for _, field := range []CustomField{
			{ID: "keep", Kind: core.KindTickets, Name: "Ghi chú", Type: "text", Enabled: true},
			{ID: "quantity", Kind: core.KindTickets, Name: "Số lượng", Type: "number", Enabled: true},
		} {
			data, _ := json.Marshal(field)
			store.PutRecord(core.Record{ID: field.ID, TenantID: owner.TenantID, Kind: "custom_fields", Enabled: true, Content: string(data)})
		}
		store.PutRecord(core.Record{ID: "ticket", TenantID: owner.TenantID, Kind: core.KindTickets,
			Content: "Khách cần đổi 3 sản phẩm.", Status: core.StatusOpen, CustomValues: `{"keep":"nhãn thủ công"}`})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := model.reserveRequest(owner.TenantID); err != nil {
		t.Fatal(err)
	}
	if err := model.reserveRequest(owner.TenantID); !errors.Is(err, errAIQuota) {
		t.Fatalf("quota should block: %v", err)
	}
	request := FieldExtractionRequest{RequestID: "unique-test-request", TicketIDs: []string{"ticket"}, FieldIDs: []string{"keep", "quantity"}}
	for range 2 {
		if err := operations.QueueFieldExtraction(owner, request); err != nil {
			t.Fatal(err)
		}
	}
	operations.runAIJobs()
	if calls != 0 {
		t.Fatal("provider called after quota exhaustion")
	}
	batches, err := operations.FieldExtractionBatches(owner)
	if err != nil || len(batches) != 1 || batches[0].Total != 1 || batches[0].Pending != 1 {
		t.Fatalf("unexpected pending batch: %+v, %v", batches, err)
	}
	clock.now = now.Add(15 * time.Minute)
	operations.runAIJobs()
	values, err := admin.Custom(owner, core.KindTickets, "ticket", nil)
	if err != nil || len(values) != 2 || values["keep"] != "nhãn thủ công" || values["quantity"] != "3" || calls != 1 {
		t.Fatalf("unexpected extraction: %+v calls=%d err=%v", values, calls, err)
	}
	batches, err = operations.FieldExtractionBatches(owner)
	if err != nil || len(batches) != 1 || batches[0].Applied != 1 || batches[0].Pending != 0 {
		t.Fatalf("unexpected completed batch: %+v, %v", batches, err)
	}
}

func TestKnowledgeGapDraftRequiresResolvedHumanEvidence(t *testing.T) {
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, ReportZone)
	operations, _, owner := operationsFixture(t, now)
	model := NewOpenAIKnowledge("test", "")
	model.client.Transport = webhookTransport(func(*http.Request) (*http.Response, error) {
		return testCompletion(map[string]string{"title": "Đổi địa chỉ giao hàng", "body": "Mở mục Đơn hàng để sửa địa chỉ."}), nil
	})
	operations.ConfigureAIJobs(model)
	if err := operations.repo.Update(func(store core.Writer) error {
		store.PutRecord(core.Record{ID: settingsID(owner.TenantID), TenantID: owner.TenantID, Kind: tenantSettingsKind, Content: `{"flags":{"ai_knowledge_drafts":true}}`})
		ticket := core.Record{ID: "ticket", TenantID: owner.TenantID, Kind: core.KindTickets, Status: core.StatusOpen, ConversationID: "thread"}
		store.PutRecord(ticket)
		store.PutRecord(core.Record{ID: "question", TenantID: owner.TenantID, Kind: core.KindMessages, Sender: core.SenderCustomer, ConversationID: "thread", Sequence: 1, Content: "Đổi địa chỉ ở đâu?", CreatedAt: now})
		store.PutRecord(core.Record{ID: "answer", TenantID: owner.TenantID, Kind: core.KindMessages, Sender: core.SenderAgent, ConversationID: "thread", Sequence: 2, Content: "Mở mục Đơn hàng để sửa địa chỉ.", CreatedAt: now})
		store.PutRecord(core.Record{ID: "gap", TenantID: owner.TenantID, Kind: "ai_bot_markers", Name: "knowledge_gap", MessageID: "question", ConversationID: "thread", CreatedAt: now})
		ticket.Status = core.StatusResolved
		store.PutRecord(ticket)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	operations.runAIJobs()
	operations.runAIJobs()
	if err := operations.repo.ViewTenant(owner.TenantID, func(store core.Reader) error {
		items := store.Records(owner.TenantID, core.KindKnowledge)
		if len(items) != 1 || items[0].Status != "draft" || items[0].Public || items[0].Enabled {
			t.Fatalf("unexpected knowledge draft: %+v", items)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTicketAgentRejectsStalePlanAndAppliesOnce(t *testing.T) {
	now := time.Date(2026, 11, 19, 11, 0, 0, 0, ReportZone)
	operations, clock, owner := operationsFixture(t, now)
	model := NewOpenAIKnowledge("test", "")
	model.client.Transport = webhookTransport(func(*http.Request) (*http.Response, error) {
		return testCompletion(map[string]string{"action": "resolve_ticket", "reason": "Khách xác nhận đã xử lý."}), nil
	})
	ai := NewAI(operations.repo, clock, model, false, false)
	ticket := core.Record{ID: "ticket", TenantID: owner.TenantID, Kind: core.KindTickets, ConversationID: "thread", Status: core.StatusOpen}
	addMessage := func(store core.Writer, id string, seq uint64) {
		store.PutRecord(core.Record{ID: id, Kind: core.KindMessages, TenantID: owner.TenantID, ConversationID: "thread",
			Sender: core.SenderCustomer, Sequence: seq, Content: "Đã giải quyết", CreatedAt: now})
	}
	if err := operations.repo.Update(func(store core.Writer) error {
		store.PutRecord(core.Record{ID: settingsID(owner.TenantID), TenantID: owner.TenantID, Kind: tenantSettingsKind, Content: `{"flags":{"ai_ticket_agent":true}}`})
		seedSparringCertificate(store, owner.TenantID, model.model, now)
		store.PutRecord(ticket)
		addMessage(store, "first", 1)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	stale, err := ai.PlanTicketAction(owner, ticket.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := operations.repo.Update(func(store core.Writer) error { addMessage(store, "second", 2); return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := ai.ApplyTicketAction(owner, ticket.ID, stale.ID); err == nil {
		t.Fatal("stale plan applied")
	}
	plan, err := ai.PlanTicketAction(owner, ticket.ID)
	if err != nil {
		t.Fatal(err)
	}
	resolutions := 0
	operations.repo.SetOnResolved(func(core.Record) { resolutions++ })
	for range 2 {
		if applied, err := ai.ApplyTicketAction(owner, ticket.ID, plan.ID); err != nil || applied.Status != "applied" {
			t.Fatalf("unexpected apply: %+v, %v", applied, err)
		}
	}
	if resolutions != 1 {
		t.Fatalf("resolution hooks=%d", resolutions)
	}
	traces, err := ai.AgentToolTraces(owner)
	if err != nil || len(traces) != 3 || traces[0].Outcome != "applied" || traces[0].RunID != plan.ID {
		t.Fatalf("expected one applied trace after two proposals: %+v, %v", traces, err)
	}
}

func TestVoiceImportReusesTranscriptWithoutStoringAudio(t *testing.T) {
	operations, clock, owner := operationsFixture(t, time.Now())
	model := NewOpenAIKnowledge("test", "")
	calls := 0
	model.client.Transport = webhookTransport(func(*http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"text":"Nội dung cuộc gọi."}`))}, nil
	})
	ai := NewAI(operations.repo, clock, model, false, false)
	if err := operations.repo.Update(func(store core.Writer) error {
		store.PutRecord(core.Record{ID: settingsID(owner.TenantID), TenantID: owner.TenantID, Kind: tenantSettingsKind, Content: `{"flags":{"ai_voice_import":true}}`})
		store.PutRecord(core.Record{ID: "customer", TenantID: owner.TenantID, Kind: core.KindCustomers, Name: "Khách A"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		created, err := ai.ImportVoice(owner, "customer", []byte("ID3 audio sample"))
		if err != nil || created.Channel != core.ChannelPhone {
			t.Fatalf("unexpected voice import: %+v, %v", created, err)
		}
	}
	if err := operations.repo.ViewTenant(owner.TenantID, func(store core.Reader) error {
		messages := store.Records(owner.TenantID, core.KindMessages)
		if calls != 1 || len(messages) != 1 || messages[0].AttachmentData != "" || messages[0].Sender != "note" || messages[0].Content != "Nội dung cuộc gọi." {
			t.Fatalf("unexpected transcript: %+v (calls %d)", messages, calls)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestLeadCampaignStopsPendingFollowUpsAfterOptOut(t *testing.T) {
	now := time.Date(2026, 11, 19, 11, 0, 0, 0, ReportZone)
	operations, clock, owner := operationsFixture(t, now)
	t.Setenv("NEXA_CHANNEL_CREDENTIALS", `{"tenant":{"zalo":{"account_id":"111","app_id":"222","access_token":"test-token","webhook_secret":"test-secret"}}}`)
	conversations := NewConversations(operations.repo, clock, operations.repo.IDs, LocalKnowledgeProvider{}, NewKeywordAutomation(operations.repo.IDs, clock))
	manager := NewChannels(operations.repo)
	manager.ConfigureCampaignOrigin("https://nexa.example.test")
	manager.ConfigureConversations(conversations)
	if _, err := manager.Configure(owner, core.ChannelZalo, core.Record{Enabled: true, Content: `{"account_id":"111","app_id":"222"}`}); err != nil {
		t.Fatal(err)
	}
	campaign, err := manager.SaveLeadCampaign(owner, LeadCampaign{
		Name: "Tư vấn", Channel: core.ChannelZalo, Welcome: "Chào bạn!",
		FollowUps: []string{"Bạn cần hỗ trợ thêm?", "Chúng tôi vẫn có thể hỗ trợ."}, Enabled: true,
	})
	if err != nil || !strings.HasSuffix(campaign.URL, campaign.ID) {
		t.Fatalf("unexpected campaign: %+v, %v", campaign, err)
	}
	landing, err := manager.OpenLeadCampaign(owner.TenantID, campaign.ID)
	if err != nil || landing.Destination != "https://zalo.me/111" || landing.FollowUps != 2 {
		t.Fatalf("unexpected landing: %+v, %v", landing, err)
	}
	if image, err := manager.LeadQRCode(owner, campaign.ID); err != nil || !strings.HasPrefix(image, "data:image/png;base64,iVBOR") {
		t.Fatalf("invalid QR image: %v", err)
	}
	sendInbound := func(id, text string) {
		if err := operations.repo.UpdateTenant(owner.TenantID, func(store core.Writer) error {
			return manager.ingest(store, owner.TenantID, core.ChannelZalo, "111", inboundChannelMessage{id: id, sender: "12345", text: text})
		}); err != nil {
			t.Fatal(err)
		}
	}
	sendInbound("registration", landing.Message+" OK2")
	sendInbound("registration", landing.Message+" OK2")
	deliveries := 0
	manager.Provider(core.ChannelZalo).(*liveChannel).client.Transport = webhookTransport(func(*http.Request) (*http.Response, error) {
		deliveries++
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"error":0,"data":{"message_id":"receipt"}}`))}, nil
	})
	manager.processChannelOutbox(context.Background())
	sendInbound("opt-out", "STOP")
	clock.now = now.Add(9 * time.Hour)
	manager.processChannelOutbox(context.Background())
	leads, err := manager.CampaignLeads(owner, campaign.ID)
	if err != nil || len(leads) != 1 || !leads[0].Unsubscribed || leads[0].Score != 30 || deliveries != 1 {
		t.Fatalf("unexpected leads: %+v deliveries=%d err=%v", leads, deliveries, err)
	}
}
