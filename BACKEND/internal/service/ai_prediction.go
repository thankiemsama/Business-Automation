package service

import (
	"sort"
	"time"

	"businessautomation/internal/core"
)

func medianDuration(samples []time.Duration, fallback time.Duration) time.Duration {
	if len(samples) < 5 {
		return fallback
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	return samples[len(samples)/2]
}

func sameNullableTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func (s *AIService) predict() {
	now := s.clock.Now().UTC()
	type prediction struct {
		ticket   core.Record
		reason   string
		transfer bool
	}
	pending := []prediction{}
	_ = s.repo.View(func(store core.Reader) error {
		for _, tenant := range store.Tenants() {
			if !featureEnabled(store, tenant, "ai") {
				continue
			}
			tickets := store.Records(tenant, core.KindTickets)
			responseSamples, resolutionSamples := []time.Duration{}, []time.Duration{}
			queues := map[string]int{}
			for _, ticket := range tickets {
				if ticket.Status != core.StatusResolved && ticket.FirstResponseAt == nil {
					queues[ticket.AssigneeID]++
				}
				if ticket.CreatedAt.Before(now.AddDate(0, 0, -60)) {
					continue
				}
				if ticket.FirstResponseAt != nil && ticket.FirstResponseAt.After(ticket.CreatedAt) {
					responseSamples = append(responseSamples, ticket.FirstResponseAt.Sub(ticket.CreatedAt))
				}
				if ticket.ResolvedAt != nil && ticket.ResolvedAt.After(ticket.CreatedAt) {
					resolutionSamples = append(resolutionSamples, ticket.ResolvedAt.Sub(ticket.CreatedAt))
				}
			}
			responseETA := medianDuration(responseSamples, 30*time.Minute)
			resolutionETA := medianDuration(resolutionSamples, 8*time.Hour)
			for _, ticket := range tickets {
				reason := ""
				elapsed := now.Sub(ticket.CreatedAt)
				if ticket.Status != core.StatusResolved && !ticketOverdue(ticket, now) && elapsed > 0 {
					response, resolution := ticketDeadlines(ticket)
					if ticket.FirstResponseAt == nil && elapsed*4 >= response.Sub(ticket.CreatedAt)*3 &&
						response.Sub(now) <= responseETA+time.Duration(min(queues[ticket.AssigneeID]-1, 8))*10*time.Minute {
						reason = "Có nguy cơ trễ phản hồi theo lịch sử và số phiếu đang chờ."
					} else if elapsed*4 >= resolution.Sub(ticket.CreatedAt)*3 && resolution.Sub(now) <= resolutionETA {
						reason = "Có nguy cơ trễ xử lý theo thời gian giải quyết gần đây."
					}
				}
				transfer := reason != "" && s.autoEscalate && ticket.EscalatedAt == nil
				if ticket.RiskReason != reason || transfer {
					pending = append(pending, prediction{ticket, reason, transfer})
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
			if !ok || current.Status != prev.Status || current.Priority != prev.Priority || !current.UpdatedAt.Equal(prev.UpdatedAt) || !sameNullableTime(current.FirstResponseAt, prev.FirstResponseAt) {
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
			if current.RiskReason == item.reason && current.AssigneeID == prev.AssigneeID && sameNullableTime(current.EscalatedAt, prev.EscalatedAt) {
				continue
			}
			current.RiskReason = item.reason
			store.PutRecord(current)
			if current.RiskReason != "" {
				store.(*workspaceWriter).notify(current, "sla_risk", "Phiếu có nguy cơ trễ SLA")
			}
		}
		return nil
	})
}
