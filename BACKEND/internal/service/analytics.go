package service

import (
	"sort"
	"time"

	"businessautomation/internal/core"
)

var ReportZone = time.FixedZone("ICT", 7*3600)

type AnalyticsPoint struct {
	Day     string   `json:"day"`
	Tickets int      `json:"tickets"`
	Ratings int      `json:"ratings"`
	CSAT    *float64 `json:"csat"`
}
type AnalyticsBreakdown struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}
type SLAWeek struct {
	Week    string `json:"week"`
	AgentID string `json:"agent_id"`
	Agent   string `json:"agent"`
	Passed  int    `json:"passed"`
	Total   int    `json:"total"`
}
type Cohort struct {
	Week   string `json:"week"`
	Size   int    `json:"size"`
	Active []*int `json:"active"`
}
type Analytics struct {
	Daily       []AnalyticsPoint     `json:"daily"`
	Channels    []AnalyticsBreakdown `json:"channels"`
	Priorities  []AnalyticsBreakdown `json:"priorities"`
	SLA         []SLAWeek            `json:"sla"`
	Cohorts     []Cohort             `json:"cohorts"`
	Leaderboard []ReportRow          `json:"leaderboard"`
}

func reportDay(at time.Time) time.Time {
	local := at.In(ReportZone)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, ReportZone)
}
func reportWeek(at time.Time) time.Time {
	day := reportDay(at)
	return day.AddDate(0, 0, -(int(day.Weekday())+6)%7)
}

func (s *DashboardService) Analytics(user core.User, from, to, now time.Time) (Analytics, error) {
	report, err := s.Report(user, from, to)
	if err != nil {
		return Analytics{}, err
	}
	result := Analytics{Daily: []AnalyticsPoint{}, Channels: []AnalyticsBreakdown{}, Priorities: []AnalyticsBreakdown{}, SLA: []SLAWeek{}, Cohorts: []Cohort{}, Leaderboard: []ReportRow{}}
	for _, row := range report.Agents {
		if row.ID != "" {
			result.Leaderboard = append(result.Leaderboard, row)
		}
	}
	sort.SliceStable(result.Leaderboard, func(i, j int) bool { return result.Leaderboard[i].Resolved > result.Leaderboard[j].Resolved })
	byDay := map[string]*AnalyticsPoint{}
	for day := reportDay(from); day.Before(to); day = day.AddDate(0, 0, 1) {
		label := day.Format(time.DateOnly)
		byDay[label] = &AnalyticsPoint{Day: label}
	}
	cutoff := minTime(now, to)
	ratingTotal := map[string]int{}
	channels, priorities := map[string]int{}, map[string]int{}
	weeks := map[[2]string]*SLAWeek{}
	histories := map[string]map[time.Time]bool{}
	inRange := func(at time.Time) bool { return !at.Before(from) && at.Before(to) }
	err = s.repo.View(func(store core.Reader) error {
		names := map[string]string{"": "Chưa phân công"}
		for _, user := range store.Users(user.TenantID) {
			names[user.ID] = user.Name
		}
		addRating := func(r core.Record) {
			if r.RatedAt == nil || r.Rating < 1 || r.Rating > 5 || !inRange(*r.RatedAt) {
				return
			}
			day := r.RatedAt.In(ReportZone).Format(time.DateOnly)
			byDay[day].Ratings++
			ratingTotal[day] += r.Rating
		}
		for _, conversation := range store.Records(user.TenantID, core.KindConversations) {
			addRating(conversation)
		}
		for _, ticket := range store.Records(user.TenantID, core.KindTickets) {
			if inRange(ticket.CreatedAt) {
				byDay[ticket.CreatedAt.In(ReportZone).Format(time.DateOnly)].Tickets++
				priorities[ticket.Priority]++
				channel := "unknown"
				if conv, ok := store.Record(user.TenantID, ticket.ConversationID, core.KindConversations); ok && conv.Channel != "" {
					channel = conv.Channel
				}
				channels[channel]++
			}
			if ticket.CustomerID != "" && ticket.CreatedAt.Before(cutoff) {
				if histories[ticket.CustomerID] == nil {
					histories[ticket.CustomerID] = map[time.Time]bool{}
				}
				histories[ticket.CustomerID][reportWeek(ticket.CreatedAt)] = true
			}
			addRating(ticket)
			response, resolution := ticketDeadlines(ticket)
			stages := []struct {
				deadline  time.Time
				completed *time.Time
				actor     string
			}{
				{response, ticket.FirstResponseAt, ticket.RespondedBy},
				{resolution, ticket.ResolvedAt, ticket.ResolvedBy},
			}
			for index, stage := range stages {
				if index == 1 && ticket.Status == core.StatusResolved && ticket.ResolvedAt == nil {
					continue
				}
				if !inRange(stage.deadline) || stage.deadline.After(cutoff) {
					continue
				}
				actor := stage.actor
				if actor == "" || stage.completed == nil || stage.completed.After(cutoff) {
					actor = ticket.AssigneeID
				}
				if names[actor] == "" {
					actor = ""
				}
				week := reportWeek(stage.deadline).Format(time.DateOnly)
				key := [2]string{week, actor}
				if weeks[key] == nil {
					weeks[key] = &SLAWeek{Week: week, AgentID: actor, Agent: names[actor]}
				}
				row := weeks[key]
				row.Total++
				if stage.completed != nil && !stage.completed.After(stage.deadline) {
					row.Passed++
				}
			}
		}
		return nil
	})
	if err != nil {
		return Analytics{}, err
	}
	for day, p := range byDay {
		if p.Ratings > 0 {
			value := float64(ratingTotal[day]) / float64(p.Ratings)
			p.CSAT = &value
		}
		result.Daily = append(result.Daily, *p)
	}
	sort.Slice(result.Daily, func(i, j int) bool { return result.Daily[i].Day < result.Daily[j].Day })
	result.Channels, result.Priorities = breakdown(channels), breakdown(priorities)
	for _, row := range weeks {
		result.SLA = append(result.SLA, *row)
	}
	sort.Slice(result.SLA, func(i, j int) bool {
		a, b := result.SLA[i], result.SLA[j]
		if a.Week != b.Week {
			return a.Week < b.Week
		}
		if a.Agent != b.Agent {
			return a.Agent < b.Agent
		}
		return a.AgentID < b.AgentID
	})
	result.Cohorts = buildCohorts(histories, from, cutoff)
	return result, nil
}
func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func breakdown(values map[string]int) []AnalyticsBreakdown {
	result := []AnalyticsBreakdown{}
	for name, count := range values {
		result = append(result, AnalyticsBreakdown{Name: name, Count: count})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result
}

func buildCohorts(histories map[string]map[time.Time]bool, from, cutoff time.Time) []Cohort {
	groups := map[time.Time]*Cohort{}
	for _, visits := range histories {
		var first time.Time
		for week := range visits {
			if first.IsZero() || week.Before(first) {
				first = week
			}
		}
		if first.Before(reportWeek(from)) {
			continue
		}
		row := groups[first]
		if row == nil {
			row = &Cohort{Week: first.Format(time.DateOnly), Active: make([]*int, 9)}
			for offset := range row.Active {
				if !first.AddDate(0, 0, 7*(offset+1)).After(cutoff) {
					value := 0
					row.Active[offset] = &value
				}
			}
			groups[first] = row
		}
		row.Size++
		for offset, count := range row.Active {
			if count != nil && visits[first.AddDate(0, 0, 7*offset)] {
				*count++
			}
		}
	}
	result := []Cohort{}
	for _, row := range groups {
		result = append(result, *row)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Week < result[j].Week })
	return result
}
