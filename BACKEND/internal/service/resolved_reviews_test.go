package service

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"businessautomation/internal/core"
)

func TestResolvedReviewQueuesOnceAndPreservesManualTags(t *testing.T) {
	now := time.Date(2026, 11, 19, 9, 0, 0, 0, ReportZone)
	operations, _, owner := operationsFixture(t, now)
	model := NewOpenAIKnowledge("test", "")
	calls := 0
	model.client.Transport = webhookTransport(func(request *http.Request) (*http.Response, error) {
		calls++
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), "internal-note-marker") || !strings.Contains(string(body), "agent-response-marker") {
			t.Fatalf("unexpected review context: %s", body)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(
			`{"choices":[{"finish_reason":"stop","message":{"content":"{\"score\":85,\"findings\":[\"Đã hướng dẫn\"],\"tags\":[\" theo Dõi \",\"đổi trả\"],\"customer_impression\":\"Cần đổi hàng\"}"}}]}`,
		))}, nil
	})
	operations.ConfigureAIJobs(model)
	ticket := core.Record{
		ID: "test-ticket", TenantID: owner.TenantID, Kind: core.KindTickets,
		Status: core.StatusOpen, Subject: "Đổi hàng", ConversationID: "thread",
		Tags: []string{"theo dõi"}, CreatedAt: now, UpdatedAt: now,
	}
	if err := operations.repo.Update(func(w core.Writer) error {
		w.PutRecord(core.Record{
			ID: settingsID(owner.TenantID), TenantID: owner.TenantID, Kind: tenantSettingsKind,
			Content: `{"flags":{"ai_quality_review":true}}`,
		})
		w.PutRecord(ticket)
		for _, msg := range []core.Record{
			{ID: "reply", Sender: core.SenderAgent, Content: "agent-response-marker"},
			{ID: "note", Sender: "note", Content: "internal-note-marker"},
		} {
			msg.Kind, msg.TenantID, msg.ConversationID, msg.CreatedAt = core.KindMessages, owner.TenantID, ticket.ConversationID, now
			w.PutRecord(msg)
		}
		ticket.Status, ticket.ResolvedAt = core.StatusResolved, &now
		w.PutRecord(ticket)
		w.PutRecord(ticket)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	operations.runJobs()
	operations.runAIJobs()
	operations.runAIJobs()
	ai := NewAI(operations.repo, operations.clock, model, false, false)
	review, err := ai.TicketReview(owner, ticket.ID)
	if err != nil || review == nil || review.Score == nil || *review.Score != 85 || review.Status != "completed" || calls != 1 {
		t.Fatalf("review=%+v calls=%d err=%v", review, calls, err)
	}
	if err := operations.repo.View(func(r core.Reader) error {
		saved, _ := r.Record(owner.TenantID, ticket.ID, ticket.Kind)
		if strings.Join(saved.Tags, ",") != "theo dõi,đổi trả" {
			t.Fatalf("unexpected tags: %v", saved.Tags)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	report, err := operations.Analytics(owner, now.Add(-time.Hour), now.Add(time.Hour))
	if err != nil || report.AI.ReviewsQueued != 1 || report.AI.ReviewsScored != 1 || report.AI.ReviewsPending != 0 {
		t.Fatalf("metrics=%+v err=%v", report.AI, err)
	}
}
