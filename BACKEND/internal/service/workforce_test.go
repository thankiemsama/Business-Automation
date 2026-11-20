package service

import (
	"testing"
	"time"

	"businessautomation/internal/core"
)

func TestWorkforceForecastAndResolverProvenance(t *testing.T) {
	cutoff := time.Date(2026, 11, 20, 12, 0, 0, 0, ReportZone)
	activity := newWorkforceActivity(cutoff)
	firstForecast := reportDay(cutoff).AddDate(0, 0, 1)
	activity.observeVolume(firstForecast.AddDate(0, 0, -14))
	for range 3 {
		activity.observeVolume(firstForecast.AddDate(0, 0, -7))
	}
	activity.observeVolume(cutoff.Add(-time.Hour))
	closed := cutoff.Add(-time.Hour)
	for _, id := range []string{"human", "agent", "unknown"} {
		activity.observeTicket(id, closed.Add(-time.Hour))
	}
	activity.observeTicket("later-touch", closed.Add(time.Minute))
	activity.observePlan(core.Record{ConversationID: "agent", Name: "resolve_ticket", Status: "applied", UpdatedAt: closed})
	for _, row := range []struct{ id, resolver string }{{"human", "staff"}, {"agent", "approver"}, {"unknown", ""}, {"later-touch", "staff"}} {
		activity.observeResolution(core.Record{ID: row.id, Status: core.StatusResolved, Priority: core.PriorityHigh,
			ResolvedBy: row.resolver, ResolvedAt: &closed}, "zalo", cutoff.Add(-24*time.Hour))
	}
	report := activity.report()
	if total := report.AIToHuman; total.Touched != 3 || total.Human != 1 || total.Automated != 1 || total.Unknown != 1 {
		t.Fatalf("wrong resolver provenance: %+v", total)
	}
	point := report.Forecast[0]
	if point.Day != firstForecast.Format(time.DateOnly) || point.Samples != 2 || point.Expected == nil || *point.Expected != 2 {
		t.Fatalf("wrong weekday forecast: %+v", point)
	}
}
