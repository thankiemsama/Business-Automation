package service

import (
	"sort"
	"time"

	"businessautomation/internal/core"
)

func sameNullableTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func (s *AIService) predict() {
	now := s.clock.Now().UTC()
	type prediction struct {
		ticket               core.Record
		reason               string
		transfer             bool
		response, resolution slaForecast
	}
	pending := []prediction{}
	_ = s.repo.View(func(store core.Reader) error {
		for _, tenant := range store.Tenants() {
			if !featureEnabled(store, tenant, "ai") {
				continue
			}
			tickets := store.Records(tenant, core.KindTickets)
			sort.Slice(tickets, func(i, j int) bool {
				if tickets[i].CreatedAt.Equal(tickets[j].CreatedAt) {
					return tickets[i].ID < tickets[j].ID
				}
				return tickets[i].CreatedAt.Before(tickets[j].CreatedAt)
			})
			channels := map[string]string{}
			for _, conversation := range store.Records(tenant, core.KindConversations) {
				channels[conversation.ID] = conversation.Channel
			}
			histories := slaWorkloads(tickets)
			features := func(ticket core.Record, workload int) slaFeatures {
				return slaFeatures{channels[ticket.ConversationID], ticket.Priority, ticket.CreatedAt.In(ReportZone).Hour(), workload}
			}
			var responses, resolutions []slaSample
			for _, ticket := range tickets {
				if ticket.CreatedAt.Before(now.AddDate(0, 0, -60)) {
					continue
				}
				f := features(ticket, histories[ticket.AssigneeID].at(ticket.CreatedAt))
				if ticket.FirstResponseAt != nil && ticket.FirstResponseAt.After(ticket.CreatedAt) && !ticket.FirstResponseAt.After(now) {
					responses = append(responses, slaSample{f, ticket.FirstResponseAt.Sub(ticket.CreatedAt)})
				}
				if ticket.ResolvedAt != nil && ticket.ResolvedAt.After(ticket.CreatedAt) && !ticket.ResolvedAt.After(now) {
					resolutions = append(resolutions, slaSample{f, ticket.ResolvedAt.Sub(ticket.CreatedAt)})
				}
			}
			responses = responses[max(0, len(responses)-2000):]
			resolutions = resolutions[max(0, len(resolutions)-2000):]
			responseMedian, responseTail := fitSLAQuantile(responses, .5), fitSLAQuantile(responses, .9)
			resolutionMedian, resolutionTail := fitSLAQuantile(resolutions, .5), fitSLAQuantile(resolutions, .9)
			for _, ticket := range tickets {
				var response, resolution slaForecast
				reason := ""
				elapsed := now.Sub(ticket.CreatedAt)
				if ticket.Status != core.StatusResolved && elapsed > 0 {
					respondBy, resolveBy := ticketDeadlines(ticket)
					f := features(ticket, max(0, histories[ticket.AssigneeID].at(now)-1))
					if ticket.FirstResponseAt == nil {
						if len(responses) >= 20 {
							response = forecastSLA(responseMedian, responseTail, f)
						} else {
							response = slaForecast{15 * time.Minute, 30 * time.Minute}
						}
					}
					if len(resolutions) >= 20 {
						resolution = forecastSLA(resolutionMedian, resolutionTail, f)
					} else {
						resolution = slaForecast{4 * time.Hour, 8 * time.Hour}
					}
					if !ticketOverdue(ticket, now) {
						responseRisk := response.p90 > respondBy.Sub(ticket.CreatedAt)
						resolutionRisk := resolution.p90 > resolveBy.Sub(ticket.CreatedAt)
						if len(responses) < 20 && response.p90 > 0 {
							responseRisk = elapsed*4 >= respondBy.Sub(ticket.CreatedAt)*3 && respondBy.Sub(now) <= response.p90
						}
						if len(resolutions) < 20 {
							resolutionRisk = elapsed*4 >= resolveBy.Sub(ticket.CreatedAt)*3 && resolveBy.Sub(now) <= resolution.p90
						}
						if responseRisk {
							reason = "Dự báo p90 có nguy cơ trễ phản hồi."
						} else if resolutionRisk {
							reason = "Dự báo p90 có nguy cơ trễ xử lý."
						}
					}
				}
				transfer := reason != "" && s.autoEscalate && ticket.EscalatedAt == nil
				if ticket.RiskReason != reason || transfer || forecastChanged(ticket, response, resolution) {
					pending = append(pending, prediction{ticket, reason, transfer, response, resolution})
				}
			}
		}
		return nil
	})
	if len(pending) == 0 {
		return
	}
	_ = s.repo.Update(func(store core.Writer) error {
		for _, item := range pending {
			prev := item.ticket
			current, ok := store.Record(prev.TenantID, prev.ID, core.KindTickets)
			if !ok || current.Version != prev.Version || current.Status != prev.Status || current.Priority != prev.Priority || !current.UpdatedAt.Equal(prev.UpdatedAt) || !sameNullableTime(current.FirstResponseAt, prev.FirstResponseAt) {
				continue
			}
			if item.transfer && current.EscalatedAt == nil {
				for _, member := range store.Users(prev.TenantID) {
					if member.Role == core.RoleOwner && !member.Disabled {
						current.AssigneeID, current.EscalatedAt = member.ID, &now
						break
					}
				}
			}
			if !forecastChanged(current, item.response, item.resolution) && current.RiskReason == item.reason && current.AssigneeID == prev.AssigneeID && sameNullableTime(current.EscalatedAt, prev.EscalatedAt) {
				continue
			}
			riskChanged := current.RiskReason != item.reason
			current.RiskReason = item.reason
			current.ResponseP50, current.ResponseP90 = slaMinutes(item.response.p50), slaMinutes(item.response.p90)
			current.ResolutionP50, current.ResolutionP90 = slaMinutes(item.resolution.p50), slaMinutes(item.resolution.p90)
			store.PutRecord(current)
			if riskChanged && current.RiskReason != "" {
				category, title := "sla_risk", "Phiếu có nguy cơ trễ SLA"
				if agentPresetEnabled(store, current.TenantID, "sla_watch") {
					category, title = "ai_sla_watch", current.RiskReason
				}
				if s.repo.aiJobsEnabled && aiOptIn(store, current.TenantID, "ai_ops_persona") {
					store.(*workspaceWriter).queueConversationReview(current, core.WorkflowOpsReview)
				}
				store.(*workspaceWriter).notify(current, category, title)
			}
		}
		return nil
	})
}

func slaMinutes(duration time.Duration) int {
	return int((duration + time.Minute - 1) / time.Minute)
}

func forecastChanged(ticket core.Record, response, resolution slaForecast) bool {
	return ticket.ResponseP50 != slaMinutes(response.p50) || ticket.ResponseP90 != slaMinutes(response.p90) ||
		ticket.ResolutionP50 != slaMinutes(resolution.p50) || ticket.ResolutionP90 != slaMinutes(resolution.p90)
}
