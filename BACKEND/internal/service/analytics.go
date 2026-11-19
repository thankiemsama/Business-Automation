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
type MetricAverage struct {
	Name    string   `json:"name"`
	Samples int      `json:"samples"`
	Value   *float64 `json:"value"`
}

type AIMetrics struct {
	AgentProposed     int      `json:"agent_proposed"`
	AgentApplied      int      `json:"agent_applied"`
	AgentAutoResolved int      `json:"agent_auto_resolved"`
	ReviewsQueued     int      `json:"reviews_queued"`
	ReviewsScored     int      `json:"reviews_scored"`
	ReviewsPending    int      `json:"reviews_pending"`
	ReviewsFailed     int      `json:"reviews_failed"`
	TriageOffered     int      `json:"triage_offered"`
	TriageAccepted    int      `json:"triage_accepted"`
	TriageAutoApplied int      `json:"triage_auto_applied"`
	BotCases          int      `json:"bot_cases"`
	AvoidedTickets    int      `json:"avoided_tickets"`
	AutoResolved      int      `json:"auto_resolved"`
	Tickets           int      `json:"tickets"`
	Metered           int      `json:"metered_requests"`
	Unpriced          int      `json:"unpriced_requests"`
	CostUSD           *float64 `json:"cost_usd"`
	CostPerTicket     *float64 `json:"cost_per_ticket_usd"`
}

type averageAccumulator struct {
	total float64
	count int
}

func addAverage(groups map[string]averageAccumulator, key string, value float64) {
	item := groups[key]
	item.total += value
	item.count++
	groups[key] = item
}

func metricAverages(groups map[string]averageAccumulator) []MetricAverage {
	rows := make([]MetricAverage, 0, len(groups))
	for name, item := range groups {
		value := item.total / float64(item.count)
		rows = append(rows, MetricAverage{Name: name, Samples: item.count, Value: &value})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows
}

type Analytics struct {
	AI               AIMetrics            `json:"ai"`
	ChannelCSAT      []MetricAverage      `json:"csat_by_channel"`
	PriorityResponse []MetricAverage      `json:"response_minutes_by_priority"`
	Daily            []AnalyticsPoint     `json:"daily"`
	Channels         []AnalyticsBreakdown `json:"channels"`
	Priorities       []AnalyticsBreakdown `json:"priorities"`
	SLA              []SLAWeek            `json:"sla"`
	Cohorts          []Cohort             `json:"cohorts"`
	Leaderboard      []ReportRow          `json:"leaderboard"`
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
	channelRatings, priorityResponse := map[string]averageAccumulator{}, map[string]averageAccumulator{}
	botCases := map[string]bool{}
	var aiCost float64
	channels, priorities := map[string]int{}, map[string]int{}
	weeks := map[[2]string]*SLAWeek{}
	histories := map[string]map[time.Time]bool{}
	inRange := func(at time.Time) bool { return !at.Before(from) && at.Before(to) }
	err = viewTenant(s.repo, user.TenantID, func(store core.Reader) error {
		conversationChannels := map[string]string{}
		for _, conversation := range store.Records(user.TenantID, core.KindConversations) {
			conversationChannels[conversation.ID] = conversation.Channel
		}
		channelFor := func(id string) string {
			if channel := conversationChannels[id]; channel != "" {
				return channel
			}
			return "unknown"
		}
		for _, marker := range store.Records(user.TenantID, "ai_bot_markers") {
			if marker.ConversationID != "" && inRange(marker.CreatedAt) {
				botCases[marker.ConversationID] = true
			}
		}
		for _, resolution := range store.Records(user.TenantID, "ai_bot_resolutions") {
			if inRange(resolution.CreatedAt) {
				botCases[resolution.ConversationID] = true
				result.AI.AutoResolved++
				if resolution.Enabled {
					result.AI.AvoidedTickets++
				}
			}
		}
		for _, sample := range store.Records(user.TenantID, "ai_usage_samples") {
			if !inRange(sample.CreatedAt) {
				continue
			}
			if sample.AICostUSD == nil {
				result.AI.Unpriced++
			} else {
				result.AI.Metered++
				aiCost += *sample.AICostUSD
			}
		}
		for _, event := range store.Records(user.TenantID, aiTriageEventsKind) {
			if !inRange(event.CreatedAt) {
				continue
			}
			switch event.Status {
			case "offered":
				result.AI.TriageOffered++
			case "accepted":
				result.AI.TriageAccepted++
			case "auto_applied":
				result.AI.TriageAutoApplied++
			}
		}
		for _, job := range store.Records(user.TenantID, workflowJobsKind) {
			if job.Subject != core.WorkflowAIReview || !inRange(job.CreatedAt) {
				continue
			}
			result.AI.ReviewsQueued++
			switch job.Status {
			case "completed":
				result.AI.ReviewsScored++
			case "running", "pending":
				result.AI.ReviewsPending++
			case "failed":
				result.AI.ReviewsFailed++
			}
		}
		for _, plan := range store.Records(user.TenantID, ticketAgentPlansKind) {
			if (plan.Status == "proposed" || plan.Status == "applied") && inRange(plan.CreatedAt) {
				result.AI.AgentProposed++
			}
			if plan.Status == "applied" && inRange(plan.UpdatedAt) {
				result.AI.AgentApplied++
			}
			if plan.Status == "auto_applied" && inRange(plan.UpdatedAt) {
				result.AI.AgentAutoResolved++
			}
		}
		names := map[string]string{"": "Chưa phân công"}
		for _, user := range store.Users(user.TenantID) {
			names[user.ID] = user.Name
		}
		addRating := func(r core.Record, channel string) {
			if r.RatedAt == nil || r.Rating < 1 || r.Rating > 5 || !inRange(*r.RatedAt) {
				return
			}
			day := r.RatedAt.In(ReportZone).Format(time.DateOnly)
			byDay[day].Ratings++
			ratingTotal[day] += r.Rating
			addAverage(channelRatings, channel, float64(r.Rating))
		}
		for _, conversation := range store.Records(user.TenantID, core.KindConversations) {
			addRating(conversation, channelFor(conversation.ID))
		}
		for _, ticket := range store.Records(user.TenantID, core.KindTickets) {
			if inRange(ticket.CreatedAt) {
				byDay[ticket.CreatedAt.In(ReportZone).Format(time.DateOnly)].Tickets++
				priorities[ticket.Priority]++
				channels[channelFor(ticket.ConversationID)]++
				result.AI.Tickets++
				if ticket.FirstResponseAt != nil && !ticket.FirstResponseAt.Before(ticket.CreatedAt) && !ticket.FirstResponseAt.After(cutoff) {
					addAverage(priorityResponse, ticket.Priority, ticket.FirstResponseAt.Sub(ticket.CreatedAt).Minutes())
				}
			}
			if ticket.CustomerID != "" && ticket.CreatedAt.Before(cutoff) {
				if histories[ticket.CustomerID] == nil {
					histories[ticket.CustomerID] = map[time.Time]bool{}
				}
				histories[ticket.CustomerID][reportWeek(ticket.CreatedAt)] = true
			}
			addRating(ticket, channelFor(ticket.ConversationID))
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
	result.AI.BotCases = len(botCases)
	if result.AI.Metered > 0 {
		result.AI.CostUSD = &aiCost
		if result.AI.Tickets > 0 {
			value := aiCost / float64(result.AI.Tickets)
			result.AI.CostPerTicket = &value
		}
	}
	result.ChannelCSAT = metricAverages(channelRatings)
	result.PriorityResponse = metricAverages(priorityResponse)
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
