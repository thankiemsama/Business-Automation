package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"businessautomation/internal/core"
	"businessautomation/internal/repository"
)

func operationsFixture(t *testing.T, now time.Time) (*Operations, *workspaceClock, core.User) {
	t.Helper()
	repo, err := repository.Open(filepath.Join(t.TempDir(), "operations.json"))
	if err != nil {
		t.Fatal(err)
	}
	clock := &workspaceClock{now: now}
	workspace := NewWorkspace(repo, clock, RandomIDs{})
	owner := core.User{ID: "owner", TenantID: "tenant", Role: core.RoleOwner, Email: "owner@example.test", Name: "Quản trị"}
	if err := workspace.Update(func(store core.Writer) error { store.PutUser(owner); return nil }); err != nil {
		t.Fatal(err)
	}
	return NewOperations(workspace, NewDashboard(workspace), nil, clock), clock, owner
}

func TestWorkflowSLAAndIdleDoNotDuplicateActions(t *testing.T) {
	now := time.Date(2026, 11, 18, 9, 0, 0, 0, ReportZone)
	service, _, owner := operationsFixture(t, now)
	_, err := service.SaveRule(owner, WorkflowRule{Name: "Email urgent", Enabled: true, Trigger: "sla_breached",
		Conditions: WorkflowConditions{Channel: "email", Priority: "urgent"},
		Actions:    []WorkflowAction{{Type: "assign_owner"}, {Type: "create_ticket"}, {Type: "email_owner"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.SaveRule(owner, WorkflowRule{Name: "Idle", Enabled: true, Trigger: "ticket_idle", IdleDays: 3,
		Conditions: WorkflowConditions{Channel: "web"}, Actions: []WorkflowAction{{Type: "resolve_ticket"}},
		Branch: &WorkflowBranch{Conditions: WorkflowConditions{Status: "resolved"}, Then: []WorkflowAction{{Type: "notify_owner", Value: "Đã tự đóng"}}, Otherwise: []WorkflowAction{{Type: "notify_owner", Value: "Sai nhánh"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.repo.Update(func(store core.Writer) error {
		for _, channel := range []string{"email", "web"} {
			store.PutRecord(core.Record{ID: channel, TenantID: owner.TenantID, Kind: core.KindConversations, Channel: channel, Status: core.StatusOpen, UpdatedAt: now.AddDate(0, 0, -4)})
			ticket := core.Record{ID: channel + "-ticket", TenantID: owner.TenantID, Kind: core.KindTickets, ConversationID: channel, Priority: core.PriorityUrgent, Status: core.StatusOpen,
				Subject: "Phiếu " + channel, CreatedAt: now.AddDate(0, 0, -4), UpdatedAt: now.AddDate(0, 0, -4)}
			store.PutRecord(ticket)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		service.repo.sweepSLA()
		service.scheduleIdle()
		service.runJobs()
	}
	if err := service.repo.View(func(store core.Reader) error {
		email, _ := store.Record(owner.TenantID, "email-ticket", core.KindTickets)
		web, _ := store.Record(owner.TenantID, "web-ticket", core.KindTickets)
		if email.AssigneeID != owner.ID || email.EscalatedAt == nil || web.Status != core.StatusResolved || web.ResolvedAt == nil {
			t.Fatal(email, web)
		}
		if len(store.Records(owner.TenantID, core.KindTickets)) != 3 || len(store.Records(owner.TenantID, operationMailKind)) != 1 {
			t.Fatal("actions repeated or failed")
		}
		yes, no := 0, 0
		for _, note := range store.Records(owner.TenantID, notificationKind) {
			if note.Subject == "Đã tự đóng" {
				yes++
			}
			if note.Subject == "Sai nhánh" {
				no++
			}
		}
		if yes != 1 || no != 0 {
			t.Fatal(yes, no)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestWorkflowMigrationDisablesLegacyRuleAndKeepsOpenTicketDeduplication(t *testing.T) {
	now := time.Date(2026, 11, 18, 9, 0, 0, 0, ReportZone)
	service, _, owner := operationsFixture(t, now)
	legacy := core.Record{ID: "legacy", TenantID: owner.TenantID, Kind: core.KindAutomations, Name: "Hoàn tiền", Enabled: true, Keywords: []string{"refund", "hoàn tiền"}}
	conversation := core.Record{ID: "chat", TenantID: owner.TenantID, Kind: core.KindConversations, Status: core.StatusOpen}
	existing := core.Record{ID: "old", TenantID: owner.TenantID, Kind: core.KindTickets, ConversationID: conversation.ID, AutomationID: legacy.ID, Status: core.StatusOpen}
	if err := service.repo.Update(func(store core.Writer) error {
		store.PutRecord(legacy)
		store.PutRecord(conversation)
		store.PutRecord(existing)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	migrated, err := service.MigrateKeywordRule(owner, legacy.ID)
	if err != nil || !migrated.Enabled || migrated.MigratedFrom != legacy.ID || len(migrated.Conditions.Keywords) != 2 {
		t.Fatal(migrated, err)
	}
	again, err := service.MigrateKeywordRule(owner, legacy.ID)
	if err != nil || again.ID != migrated.ID {
		t.Fatal(again, err)
	}
	send := func(text string) {
		t.Helper()
		if err := service.repo.Update(func(store core.Writer) error {
			store.PutRecord(core.Record{ID: service.repo.IDs.NewID(), TenantID: owner.TenantID, Kind: core.KindMessages, ConversationID: conversation.ID, Sender: core.SenderCustomer, Content: text, CreatedAt: now})
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		service.runJobs()
	}
	count := func(expected int) {
		t.Helper()
		if err := service.repo.View(func(store core.Reader) error {
			current, _ := store.Record(owner.TenantID, legacy.ID, core.KindAutomations)
			if current.Enabled || len(store.Records(owner.TenantID, core.KindTickets)) != expected {
				t.Fatal("legacy rule or ticket count incorrect", current.Enabled)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	send("Yêu cầu refund")
	count(1)
	if err := service.repo.Update(func(store core.Writer) error {
		existing.Status = core.StatusResolved
		existing.ResolvedAt = &now
		store.PutRecord(existing)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	send("Người dùng cần hoàn tiền")
	send("Nhắc lại: refund")
	count(2)
}

func TestWeeklyEmailQueuesOnceAndSendsCSV(t *testing.T) {
	now := time.Date(2026, 11, 15, 9, 0, 0, 0, ReportZone)
	service, clock, owner := operationsFixture(t, now)
	sent := 0
	service.mailer = &EmailMailer{key: "fake", from: "test@example.test", client: &http.Client{Transport: webhookTransport(func(request *http.Request) (*http.Response, error) {
		sent++
		var payload struct {
			To          []string
			Attachments []struct{ Filename, Content string }
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.To) != 1 || payload.To[0] != owner.Email || len(payload.Attachments) != 1 || payload.Attachments[0].Filename != "bao-cao-nexa.csv" || payload.Attachments[0].Content == "" || request.Header.Get("Idempotency-Key") == "" {
			t.Fatal(payload)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("{}"))}, nil
	})}}
	schedule, err := service.SaveSchedule(owner, ReportSchedule{Name: "Tuần", Frequency: "weekly", Recipient: owner.ID, Enabled: true, Day: 1, Hour: 8})
	if err != nil || schedule.NextAt == nil || schedule.NextAt.Format(time.RFC3339) != "2026-11-16T08:00:00+07:00" {
		t.Fatal(schedule, err)
	}
	clock.now = schedule.NextAt.Add(2 * time.Minute)
	service.scheduleReports()
	service.scheduleReports()
	service.deliverMail(context.Background())
	service.deliverMail(context.Background())
	if sent != 1 {
		t.Fatal(sent)
	}
}
