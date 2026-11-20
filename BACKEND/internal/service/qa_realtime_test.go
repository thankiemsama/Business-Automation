package service

import (
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"businessautomation/internal/core"
	"businessautomation/internal/repository"
)

func TestRealtimeQAUsesQueueQuotaAndWorkspaceEvent(t *testing.T) {
	repo, err := repository.Open(filepath.Join(t.TempDir(), "qa.json"))
	if err != nil {
		t.Fatal(err)
	}
	clock := &workspaceClock{now: time.Now().UTC()}
	workspace := NewWorkspace(repo, clock, RandomIDs{})
	owner := core.User{ID: "owner", TenantID: "tenant", Role: core.RoleOwner}
	model := NewOpenAIKnowledge("test-key", "")
	model.ConfigureUsage(workspace, clock, workspace.IDs)
	calls := 0
	model.client.Transport = webhookTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		input, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(input), "Gợi ý bản nháp") || !strings.Contains(string(input), "Khách muốn khiếu nại") {
			t.Fatalf("missing draft or context: %s", input)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(
			`{"choices":[{"finish_reason":"stop","message":{"content":"{\"labels\":[{\"kind\":\"complaint\",\"message\":\"Cần xác nhận khiếu nại\"},{\"kind\":\"unexpected\",\"message\":\"Bỏ qua\"}],\"suggestion\":\"Xin ghi nhận phản ánh\"}"}}]}`,
		))}, nil
	})
	ai := NewAI(workspace, clock, model, false, false)
	operations := NewOperations(workspace, nil, nil, clock)
	operations.ConfigureAIJobs(model)
	if err := workspace.Update(func(store core.Writer) error {
		store.PutUser(owner)
		store.PutRecord(core.Record{ID: "thread", TenantID: owner.TenantID, Kind: core.KindConversations, Status: core.StatusOpen})
		store.PutRecord(core.Record{ID: "message", TenantID: owner.TenantID, Kind: core.KindMessages, ConversationID: "thread", Sender: core.SenderCustomer, Content: "Khách muốn khiếu nại"})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := ai.QueueRealtimeQA(owner, "thread", "Gợi ý bản nháp"); err == nil {
		t.Fatal("QA was enabled by default")
	}
	if err := workspace.UpdateTenant(owner.TenantID, func(store core.Writer) error {
		store.PutRecord(core.Record{ID: settingsID(owner.TenantID), TenantID: owner.TenantID,
			Kind: tenantSettingsKind, Content: `{"flags":{"ai_realtime_qa":true},"daily_ai_requests":1}`})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	_, events, stop := workspace.Watch(owner.TenantID)
	defer stop()
	first, err := ai.QueueRealtimeQA(owner, "thread", "Gợi ý bản nháp")
	if err != nil || calls != 0 {
		t.Fatalf("queue contacted model or failed: %d %v", calls, err)
	}
	duplicate, err := ai.QueueRealtimeQA(owner, "thread", "Gợi ý bản nháp")
	if err != nil || duplicate.ID != first.ID {
		t.Fatalf("duplicate did not reuse request: %+v %v", duplicate, err)
	}
	operations.runAIJobs()
	select {
	case <-events:
	default:
		t.Fatal("workspace event was not sent")
	}
	result, err := ai.RealtimeQAResult(owner, "thread", first.ID)
	if err != nil || result.Status != "completed" || len(result.Labels) != 1 || result.Labels[0].Kind != "complaint" || result.Suggestion == "" {
		t.Fatalf("unexpected QA result: %+v %v", result, err)
	}
	second, err := ai.QueueRealtimeQA(owner, "thread", "Một bản nháp khác")
	if err != nil {
		t.Fatal(err)
	}
	operations.runAIJobs()
	result, err = ai.RealtimeQAResult(owner, "thread", second.ID)
	if err != nil || result.Status != "quota" || calls != 1 {
		t.Fatalf("quota did not prevent provider request: %+v %v, %d calls", result, err, calls)
	}
	if err := workspace.ViewTenant(owner.TenantID, func(store core.Reader) error {
		analytics := realtimeQAAnalytics(store, owner.TenantID, clock.now.Add(-time.Hour), clock.now.Add(time.Hour))
		if analytics.ByKind["complaint"] != 1 || analytics.ByAgent[owner.ID] != 1 ||
			analytics.ByDay[reportDay(clock.now).Format(time.DateOnly)] != 1 {
			t.Fatalf("incorrect QA analytics: %+v", analytics)
		}
		traces, err := agentTraceBuffer(store, owner.TenantID, clock.now)
		if err != nil || len(traces) != 2 || traces[0].RunID != first.ID || traces[0].Tool != core.WorkflowRealtimeQA {
			t.Fatalf("incorrect QA traces: %+v %v", traces, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	const otherTenant = "other"
	if err := workspace.UpdateTenant(otherTenant, func(store core.Writer) error {
		store.PutRecord(core.Record{ID: settingsID(otherTenant), TenantID: otherTenant,
			Kind: tenantSettingsKind, Content: `{"daily_ai_requests":1}`})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := model.reserveRequest(otherTenant); err != nil {
		t.Fatalf("one tenant's quota blocked another tenant: %v", err)
	}
}
