package service

import (
	"businessautomation/internal/core"
	"sort"
	"time"
)

type ReportRow struct {
	ID                       string  `json:"id"`
	Name                     string  `json:"name"`
	Opened                   int     `json:"opened"`
	Backlog                  int     `json:"backlog"`
	Responses                int     `json:"responses"`
	Resolved                 int     `json:"resolved"`
	Ratings                  int     `json:"ratings"`
	AverageResponseMinutes   float64 `json:"average_response_minutes"`
	AverageResolutionMinutes float64 `json:"average_resolution_minutes"`
	AverageRating            float64 `json:"average_rating"`
}

type OperationsReport struct {
	Total  ReportRow   `json:"total"`
	Agents []ReportRow `json:"agents"`
}

func (s *DashboardService) Report(user core.User, from, to time.Time) (OperationsReport, error) {
	result := OperationsReport{Agents: []ReportRow{}}
	inRange := func(at time.Time) bool { return !at.Before(from) && at.Before(to) }
	err := s.repo.View(func(store core.Reader) error {
		agents := map[string]*ReportRow{"": {Name: "Chưa phân công"}}
		for _, member := range store.Users(user.TenantID) {
			agents[member.ID] = &ReportRow{ID: member.ID, Name: member.Name}
		}
		agent := func(id string) *ReportRow {
			if row := agents[id]; row != nil {
				return row
			}
			return agents[""]
		}
		total := &result.Total
		for _, ticket := range store.Records(user.TenantID, core.KindTickets) {
			assigned := agent(ticket.AssigneeID)
			if ticket.RatedAt != nil && inRange(*ticket.RatedAt) {
				for _, row := range []*ReportRow{total, assigned} {
					row.Ratings++
					row.AverageRating += float64(ticket.Rating)
				}
			}
			if inRange(ticket.CreatedAt) {
				total.Opened++
				assigned.Opened++
			}
			if ticket.Status != core.StatusResolved {
				total.Backlog++
				assigned.Backlog++
			}
			if ticket.FirstResponseAt != nil && inRange(*ticket.FirstResponseAt) {
				minutes := ticket.FirstResponseAt.Sub(ticket.CreatedAt).Minutes()
				for _, row := range []*ReportRow{total, agent(ticket.RespondedBy)} {
					row.Responses++
					row.AverageResponseMinutes += minutes
				}
			}
			if ticket.ResolvedAt != nil && inRange(*ticket.ResolvedAt) {
				minutes := ticket.ResolvedAt.Sub(ticket.CreatedAt).Minutes()
				for _, row := range []*ReportRow{total, agent(ticket.ResolvedBy)} {
					row.Resolved++
					row.AverageResolutionMinutes += minutes
				}
			}
		}
		for _, conversation := range store.Records(user.TenantID, core.KindConversations) {
			if conversation.RatedAt != nil && inRange(*conversation.RatedAt) {
				for _, row := range []*ReportRow{total, agent(conversation.AssigneeID)} {
					row.Ratings++
					row.AverageRating += float64(conversation.Rating)
				}
			}
		}
		for _, row := range agents {
			row.average()
			result.Agents = append(result.Agents, *row)
		}
		return nil
	})
	result.Total.average()
	sort.Slice(result.Agents, func(i, j int) bool { return result.Agents[i].Name < result.Agents[j].Name })
	return result, err
}

func (row *ReportRow) average() {
	if row.Responses > 0 {
		row.AverageResponseMinutes /= float64(row.Responses)
	}
	if row.Resolved > 0 {
		row.AverageResolutionMinutes /= float64(row.Resolved)
	}
	if row.Ratings > 0 {
		row.AverageRating /= float64(row.Ratings)
	}
}
