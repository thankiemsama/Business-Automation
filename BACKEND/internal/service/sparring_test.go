package service

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"businessautomation/internal/core"
)

func seedSparringCertificate(store core.Writer, tenant, model string, now time.Time) {
	for _, id := range []string{"case-a", "case-b"} {
		store.PutRecord(core.Record{ID: id, TenantID: tenant, Kind: trainingScenariosKind, Enabled: true})
	}
	expiry := now.Add(time.Hour)
	store.PutRecord(core.Record{ID: sparringCertificateID(model), TenantID: tenant, Kind: sparringCertificatesKind,
		Tags: []string{"case-a", "case-b"}, Enabled: true, EmailRetryAt: &expiry})
}

func TestSparringCertificateExpiresAndDependsOnActiveScenarios(t *testing.T) {
	now := time.Date(2026, 11, 20, 9, 0, 0, 0, ReportZone)
	operations, clock, owner := operationsFixture(t, now)
	model := NewOpenAIKnowledge("test", "")
	ai := NewAI(operations.repo, clock, model, false, false)
	if err := operations.repo.UpdateTenant(owner.TenantID, func(store core.Writer) error {
		seedSparringCertificate(store, owner.TenantID, model.model, now)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	assertPermission := func(at time.Time, want bool) {
		t.Helper()
		if err := operations.repo.ViewTenant(owner.TenantID, func(store core.Reader) error {
			if got := sparringPermitsResolution(store, owner.TenantID, model.model, at); got != want {
				t.Errorf("permission at %v = %v, want %v", at, got, want)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	assertPermission(now, true)
	assertPermission(now.Add(time.Hour), false)
	if err := ai.ArchiveSparringScenario(owner, "case-a"); err != nil {
		t.Fatal(err)
	}
	assertPermission(now, false)
}

func TestSparringPracticeAndCertificationHaveSeparateAuthority(t *testing.T) {
	now := time.Date(2026, 11, 20, 9, 0, 0, 0, ReportZone)
	operations, clock, owner := operationsFixture(t, now)
	model := NewOpenAIKnowledge("test", "sparring-test-model")
	operations.ConfigureAIJobs(model)
	ai := NewAI(operations.repo, clock, model, false, false)
	another := core.User{ID: "another", TenantID: owner.TenantID, Role: core.RoleAgent}
	ticket := core.Record{ID: "ticket", TenantID: owner.TenantID, Kind: core.KindTickets, ConversationID: "source", CustomerID: "customer", Status: core.StatusOpen}
	conversation := core.Record{ID: "source", TenantID: owner.TenantID, Kind: core.KindConversations, CustomerID: "customer"}
	if err := operations.repo.Update(func(store core.Writer) error {
		store.PutUser(another)
		store.PutRecord(core.Record{ID: settingsID(owner.TenantID), TenantID: owner.TenantID, Kind: tenantSettingsKind,
			Content: `{"flags":{"ai_sparring":true,"ai_ticket_agent":true,"ai_ticket_agent_auto_resolve":true}}`})
		store.PutRecord(core.Record{ID: "customer", TenantID: owner.TenantID, Kind: core.KindCustomers, Name: "An Nguyễn"})
		store.PutRecord(conversation)
		store.PutRecord(ticket)
		for _, msg := range []core.Record{
			{ID: "question", Sequence: 1, Sender: core.SenderCustomer, Content: "An Nguyễn đang chờ đơn hàng. Gọi 0912345678 hoặc gửi foo@example.test."},
			{ID: "reply", Sequence: 2, Sender: core.SenderAgent, Content: "Vui lòng kiểm tra mục Đơn hàng."},
			{ID: "note", Sequence: 3, Sender: "note", Content: "ghi chú nội bộ"},
		} {
			msg.TenantID, msg.Kind, msg.ConversationID, msg.CreatedAt = owner.TenantID, core.KindMessages, conversation.ID, now
			store.PutRecord(msg)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	lines, err := ai.PreviewSparringScenario(owner, conversation.ID)
	if err != nil || len(lines) != 2 || !strings.Contains(lines[0].Text, "đang") {
		t.Fatalf("unexpected preview %+v %v", lines, err)
	}
	for _, raw := range []string{"An Nguyễn", "0912345678", "foo@example.test"} {
		if strings.Contains(lines[0].Text, raw) {
			t.Fatalf("identifying value in preview: %s", raw)
		}
	}
	if _, err := ai.PreviewSparringScenario(another, conversation.ID); err == nil {
		t.Fatal("agent accessed training source")
	}
	input := SparringScenarioInput{ConversationID: conversation.ID, Title: "Kiểm tra đơn", ExpectedAction: "handoff", Lines: lines, Anonymized: true}
	handoff, err := ai.SaveSparringScenario(owner, input)
	if err != nil {
		t.Fatal(err)
	}
	input.ExpectedAction, input.Title = "resolve_ticket", "Đã xử lý đơn"
	input.Lines = append(input.Lines, SparringLine{Role: "customer", Text: "Đã giải quyết"})
	if _, err := ai.SaveSparringScenario(owner, input); err != nil {
		t.Fatal(err)
	}
	wrongAction := false
	gradeScore := 93
	model.client.Transport = webhookTransport(func(request *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatal(err)
		}
		content := string(body)
		switch {
		case strings.Contains(content, "Đóng vai khách hàng"):
			return testCompletion(map[string]any{"text": "Tôi vẫn chưa thấy thông tin giao hàng."}), nil
		case strings.Contains(content, "Bạn là AI agent"):
			replies := []string{"Tôi sẽ chuyển nhân viên kiểm tra."}
			if strings.Contains(content, "Đã giải quyết") {
				replies = []string{"Mở mục Đơn hàng.", "Cảm ơn đã xác nhận."}
			}
			return testCompletion(map[string]any{"replies": replies}), nil
		case strings.Contains(content, "Đề xuất một công cụ"):
			action := "handoff"
			if strings.Contains(content, "Đã giải quyết") || wrongAction {
				action = "resolve_ticket"
			}
			return testCompletion(map[string]any{"action": action, "reason": "Kết quả mô phỏng."}), nil
		case strings.Contains(content, "Chấm độc lập"):
			return testCompletion(map[string]any{"score": gradeScore, "unsafe": false, "coaching": "Giải thích rõ. Hãy nhắc lại bước tiếp theo."}), nil
		default:
			t.Fatalf("unexpected provider request %s", content)
			return nil, nil
		}
	})
	practice, err := ai.StartSparring(another, handoff.ID, "practice-request-1", false)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		reply, err := ai.SparringTurn(another, practice.ID, "Tôi sẽ kiểm tra.", "turn-request-0001", 1)
		if err != nil || len(reply.Lines) != 3 {
			t.Fatalf("unexpected practice turn %+v %v", reply, err)
		}
	}
	if err := ai.EndSparring(another, practice.ID, "handoff"); err != nil {
		t.Fatal(err)
	}
	operations.runAIJobs()
	overview, err := ai.SparringOverview(another)
	if err != nil || len(overview.Sessions) != 1 || len(overview.Sessions[0].Assessments) != 1 ||
		!overview.Sessions[0].Assessments[0].Passed || overview.CertifiedTo != nil || ai.TicketAgentStatus(owner)["can_resolve"] {
		t.Fatalf("practice should pass without granting authority: %+v %v", overview, err)
	}
	checkAutoResolution := func(expected bool) {
		t.Helper()
		err := operations.repo.UpdateTenant(owner.TenantID, func(store core.Writer) error {
			got := resolveOnCustomerConfirmation(store, conversation, core.Record{Sender: core.SenderAgent},
				core.Record{ID: "confirmation", Sender: core.SenderCustomer, Content: "Đã giải quyết", CreatedAt: now})
			if got != expected {
				t.Fatalf("auto resolve = %v, want %v", got, expected)
			}
			store.PutRecord(ticket)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	checkAutoResolution(false)
	if _, err := ai.StartSparring(owner, "", "certification-request-1", true); err != nil {
		t.Fatal(err)
	}
	operations.runAIJobs()
	if ai.TicketAgentStatus(owner)["can_resolve"] {
		t.Fatal("certificate granted before both cases completed")
	}
	operations.runAIJobs()
	if !ai.TicketAgentStatus(owner)["can_resolve"] {
		t.Fatal("eligible model should have certificate")
	}
	checkAutoResolution(true)
	otherModel := NewAI(operations.repo, clock, NewOpenAIKnowledge("test", "another-model"), false, false)
	if otherModel.TicketAgentStatus(owner)["can_resolve"] {
		t.Fatal("certificate reused for a different model")
	}
	wrongAction = true
	if _, err := ai.StartSparring(owner, "", "certification-request-2", true); err != nil {
		t.Fatal(err)
	}
	operations.runAIJobs()
	operations.runAIJobs()
	if ai.TicketAgentStatus(owner)["can_resolve"] {
		t.Fatal("grader score granted a certificate after a wrong action")
	}
	checkAutoResolution(false)
	wrongAction, gradeScore = false, 85
	regression, err := ai.StartSparring(owner, "", "certification-request-3", true)
	if err != nil {
		t.Fatal(err)
	}
	operations.runAIJobs()
	operations.runAIJobs()
	result, err := ai.SparringOverview(owner)
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range result.Sessions {
		if session.ID != regression.ID {
			continue
		}
		if session.Status != "not_certified" || len(session.Assessments) != 2 ||
			!session.Assessments[0].Regression || ai.TicketAgentStatus(owner)["can_resolve"] {
			t.Fatalf("regression should block certificate renewal: %+v", session)
		}
		return
	}
	t.Fatal("missing regression result")
}
