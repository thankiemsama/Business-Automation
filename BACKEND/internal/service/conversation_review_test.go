package service

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"businessautomation/internal/core"
)

func TestAutomaticQAReviewsImageAndVoiceTranscript(t *testing.T) {
	ops, clock, owner := operationsFixture(t, time.Now())
	model := NewOpenAIKnowledge("mock-key", "")
	ops.ConfigureAIJobs(model)
	requests := []string{}
	model.client.Transport = webhookTransport(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		requests = append(requests, string(body))
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(
			`{"choices":[{"finish_reason":"stop","message":{"content":"{\"labels\":[{\"kind\":\"complaint\",\"message\":\"Cần xem lại phản ánh\"}]}"}}]}`,
		))}, nil
	})
	now := clock.Now()
	if err := ops.repo.UpdateTenant(owner.TenantID, func(store core.Writer) error {
		store.PutRecord(core.Record{ID: settingsID(owner.TenantID), TenantID: owner.TenantID, Kind: tenantSettingsKind,
			Content: `{"flags":{"ai_multimodal_qa":true}}`})
		store.PutRecord(core.Record{ID: "thread", TenantID: owner.TenantID, Kind: core.KindConversations, Status: core.StatusOpen})
		image := core.Record{ID: "image", TenantID: owner.TenantID, Kind: core.KindMessages, ConversationID: "thread",
			Sender: core.SenderCustomer, CreatedAt: now}
		store.PutRecord(image)
		image.AttachmentName, image.AttachmentData = "image.png", "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAACklEQVR4nGNgAAIAAAUAAYoz4wAAAAAASUVORK5CYII="
		store.PutRecord(image)
		store.PutRecord(core.Record{ID: "voice", TenantID: owner.TenantID, Kind: core.KindMessages, ConversationID: "thread",
			Sender: "note", AutomationID: voiceTranscriptMarker, Content: "Nội dung ghi âm cần xem", CreatedAt: now})
		store.PutRecord(core.Record{ID: "metadata-only", TenantID: owner.TenantID, Kind: core.KindMessages, ConversationID: "thread",
			Sender: core.SenderCustomer, AttachmentName: "Ảnh kênh ngoài", CreatedAt: now})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"image", "voice", "metadata-only"} {
		job := core.Record{ID: tokenHash(core.WorkflowMultimodalQA + ":" + owner.TenantID + ":" + id),
			TenantID: owner.TenantID, Subject: core.WorkflowMultimodalQA}
		if !ops.runConversationReview(job) {
			t.Fatalf("message %s was not queued", id)
		}
	}
	if len(requests) != 2 || !strings.Contains(requests[0], "data:image/png;base64,") ||
		!strings.Contains(requests[1], "Nội dung ghi âm cần xem") || strings.Contains(requests[1], "image_url") {
		t.Fatalf("unexpected multimodal requests: %d", len(requests))
	}
	if err := ops.repo.ViewTenant(owner.TenantID, func(store core.Reader) error {
		report := multimodalCoverage(store, owner.TenantID, now.Add(-time.Hour), now.Add(time.Hour))
		if report.Statuses["completed"] != 2 || report.Statuses["unsupported"] != 1 ||
			report.Modalities["image"] != 1 || report.Modalities["voice_transcript"] != 1 ||
			report.Risks["complaint"] != 2 || len(report.Alerts) != 2 {
			t.Fatalf("unexpected coverage: %+v", report)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSalesAndOpsUseSeparateSOPAndSkipOptedOutLead(t *testing.T) {
	ops, clock, owner := operationsFixture(t, time.Now())
	model := NewOpenAIKnowledge("fixture", "")
	ops.ConfigureAIJobs(model)
	var prompts []string
	model.client.Transport = webhookTransport(func(r *http.Request) (*http.Response, error) {
		raw, _ := io.ReadAll(r.Body)
		prompts = append(prompts, string(raw))
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(
			`{"choices":[{"finish_reason":"stop","message":{"content":"{\"advice\":\"Kiểm tra và chuyển cho nhân viên\"}"}}]}`,
		))}, nil
	})
	tenant := owner.TenantID
	var opsJob core.Record
	if err := ops.repo.UpdateTenant(tenant, func(store core.Writer) error {
		store.PutRecord(core.Record{ID: settingsID(tenant), TenantID: tenant, Kind: tenantSettingsKind,
			Content: `{"flags":{"ai_sales_persona":true,"ai_ops_persona":true},"persona_sop":{"sales":"Hỏi quy mô","ops":"Xác nhận SLA"}}`})
		store.PutRecord(core.Record{ID: "thread", TenantID: tenant, Kind: core.KindConversations, Channel: core.ChannelZalo, Status: core.StatusOpen})
		store.PutRecord(core.Record{ID: "campaign", TenantID: tenant, Kind: leadCampaignKind, Enabled: true})
		for _, id := range []string{"lead", "out"} {
			lead := core.Record{ID: id, TenantID: tenant, Kind: campaignLeadKind, ConversationID: "thread", CustomerID: id,
				Channel: core.ChannelZalo, AutomationID: "campaign", CreatedAt: clock.Now()}
			store.PutRecord(lead)
			if id == "out" {
				store.PutRecord(core.Record{ID: campaignOptOutID(lead), TenantID: tenant, Kind: "campaign_opt_outs"})
			}
		}
		ticket := core.Record{ID: "sla", TenantID: tenant, Kind: core.KindTickets, AssigneeID: owner.ID,
			Status: core.StatusOpen, RiskReason: "Sắp quá hạn", CreatedAt: clock.Now()}
		store.PutRecord(ticket)
		store.(*workspaceWriter).queueConversationReview(ticket, core.WorkflowOpsReview)
		for _, job := range store.Records(tenant, workflowJobsKind) {
			if job.Subject == core.WorkflowOpsReview {
				opsJob = job
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"lead", "out"} {
		job := core.Record{ID: tokenHash(core.WorkflowSalesReview + ":" + tenant + ":" + id),
			TenantID: tenant, Subject: core.WorkflowSalesReview}
		ops.runConversationReview(job)
	}
	if opsJob.ID == "" || !ops.runConversationReview(opsJob) {
		t.Fatal("no Ops review ran")
	}
	if len(prompts) != 2 || !strings.Contains(prompts[0], "Hỏi quy mô") || strings.Contains(prompts[0], "Xác nhận SLA") ||
		!strings.Contains(prompts[1], "Xác nhận SLA") {
		t.Fatalf("unexpected persona requests: %d", len(prompts))
	}
	leads, err := NewChannels(ops.repo).CampaignLeads(owner, "campaign")
	if err != nil || len(leads) != 2 {
		t.Fatalf("unexpected leads: %+v %v", leads, err)
	}
	for _, lead := range leads {
		if (lead.SalesAdvice == "") != lead.Unsubscribed {
			t.Fatalf("unexpected advice visibility: %+v", lead)
		}
	}
}
