package service

import (
	"testing"
	"time"

	"businessautomation/internal/core"
)

func TestAnalyticsUsesDeadlinesAndDistinctCustomers(t *testing.T) {
	from := time.Date(2026, 11, 2, 0, 0, 0, 0, ReportZone)
	now := from.AddDate(0, 0, 15)
	service, _, owner := operationsFixture(t, now)
	at := from.Add(8 * time.Hour)
	response := at.Add(30 * time.Minute)
	resolution := at.Add(12 * time.Hour)
	rating := at.Add(16 * time.Hour)
	err := service.repo.Update(func(store core.Writer) error {
		store.PutRecord(core.Record{ID: "email", TenantID: owner.TenantID, Kind: core.KindConversations, Channel: "email"})
		data := []core.Record{
			{ID: "first", CustomerID: "customer", ConversationID: "email", Priority: "urgent", Status: core.StatusResolved, CreatedAt: at, FirstResponseAt: &response, RespondedBy: owner.ID, ResolvedAt: &resolution, ResolvedBy: owner.ID, RatedAt: &rating, Rating: 5},
			{ID: "second", CustomerID: "customer", Priority: "urgent", Status: core.StatusOpen, CreatedAt: at.AddDate(0, 0, 7), AssigneeID: owner.ID},
			{ID: "third", CustomerID: "customer", Priority: "urgent", Status: core.StatusOpen, CreatedAt: at.AddDate(0, 0, 8), AssigneeID: owner.ID},
		}
		for _, record := range data {
			record.TenantID, record.Kind = owner.TenantID, core.KindTickets
			store.PutRecord(record)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Analytics(owner, from, from.AddDate(0, 0, 16))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Daily) != 16 || result.Daily[0].Tickets != 1 || result.Daily[0].CSAT != nil || result.Daily[1].CSAT == nil || *result.Daily[1].CSAT != 5 {
		t.Fatal(result.Daily)
	}
	if len(result.SLA) != 2 || result.SLA[0].Total != 2 || result.SLA[0].Passed != 1 || result.SLA[1].Total != 4 || result.SLA[1].Passed != 0 {
		t.Fatal(result.SLA)
	}
	if len(result.Cohorts) != 1 || result.Cohorts[0].Size != 1 || *result.Cohorts[0].Active[1] != 1 || result.Cohorts[0].Active[2] != nil {
		t.Fatal(result.Cohorts)
	}
}
