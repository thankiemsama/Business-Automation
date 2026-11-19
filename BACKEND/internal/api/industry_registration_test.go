package api_test

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"businessautomation/internal/app"
	"businessautomation/internal/core"
	"businessautomation/internal/repository"
	"businessautomation/internal/service"
)

func TestIndustryRegistration(t *testing.T) {
	repo, err := repository.Open(filepath.Join(t.TempDir(), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	root := client{t: t, h: app.NewHandler(repo)}
	credentials := map[string]any{"name": "Owner", "email": "industry@example.com", "password": "a-strong-password-2026"}
	credentials["industry"] = "unknown"
	root.call("POST", "/api/v1/auth/register", credentials, 400)
	credentials["industry"] = "logistics"
	owner := root.call("POST", "/api/v1/auth/register", credentials, 201)
	root.call("POST", "/api/v1/auth/register", credentials, 409)
	blank := root.call("POST", "/api/v1/auth/register", map[string]any{
		"name": "Blank", "email": "blank@example.com", "password": "a-strong-password-2026",
	}, 201)
	err = repo.View(func(store core.Reader) error {
		tenant := owner["tenant_id"].(string)
		articles := store.Records(tenant, core.KindKnowledge)
		if len(articles) != 1 || articles[0].Public || articles[0].Subject != "Tra cứu vận đơn và giao chậm" {
			t.Fatalf("unexpected knowledge starter: %+v", articles)
		}
		replies := store.Records(tenant, "templates")
		if len(replies) != 1 || !replies[0].Enabled {
			t.Fatalf("unexpected CSAT starter: %+v", replies)
		}
		rules := store.Records(tenant, "workflow_rules")
		if len(rules) != 2 {
			t.Fatalf("expected two starter rules, got %d", len(rules))
		}
		triggers := map[string]bool{}
		for _, record := range rules {
			var rule service.WorkflowRule
			if err := json.Unmarshal([]byte(record.Content), &rule); err != nil {
				return err
			}
			if rule.Enabled || record.Enabled || rule.ID != record.ID {
				t.Fatalf("workflow starter must be disabled: %+v", rule)
			}
			triggers[rule.Trigger] = true
		}
		if !triggers["message_received"] || !triggers["sla_breached"] {
			t.Fatalf("unexpected workflow triggers: %v", triggers)
		}
		for _, kind := range []string{core.KindKnowledge, "templates", "workflow_rules"} {
			if records := store.Records(blank["tenant_id"].(string), kind); len(records) != 0 {
				t.Fatalf("blank workspace received %d starter records of kind %s", len(records), kind)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
