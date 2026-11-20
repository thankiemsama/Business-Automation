package service

import (
	"math"
	"sort"
	"time"

	"businessautomation/internal/core"
)

type WorkforceAnalytics struct {
	Forecast    []VolumeForecast    `json:"forecast"`
	AIToHuman   HandoffResolution   `json:"ai_to_human"`
	ByQueueType []HandoffResolution `json:"by_queue_type"`
}

type VolumeForecast struct {
	Day      string   `json:"day"`
	Expected *float64 `json:"expected"`
	Samples  int      `json:"samples"`
}

type HandoffResolution struct {
	Channel   string `json:"channel,omitempty"`
	Priority  string `json:"priority,omitempty"`
	Touched   int    `json:"touched_resolved"`
	Human     int    `json:"human_resolved"`
	Automated int    `json:"agent_resolved"`
	Unknown   int    `json:"unknown_resolver"`
}

type workforceActivity struct {
	cutoff, firstDay, historyEnd, forecastStart time.Time
	volumes                                     map[time.Time]int
	touch                                       map[string]time.Time
	bot                                         map[string][]time.Time
	agentResolved                               map[string][]time.Time
	total                                       HandoffResolution
	byQueue                                     map[[2]string]HandoffResolution
}

func newWorkforceActivity(cutoff time.Time) *workforceActivity {
	start := reportDay(cutoff)
	forecast := start
	if !cutoff.Equal(start) {
		forecast = start.AddDate(0, 0, 1)
	}
	return &workforceActivity{cutoff: cutoff, historyEnd: start, forecastStart: forecast,
		volumes: map[time.Time]int{}, touch: map[string]time.Time{}, bot: map[string][]time.Time{},
		agentResolved: map[string][]time.Time{}, byQueue: map[[2]string]HandoffResolution{}}
}

func (a *workforceActivity) observeVolume(at time.Time) {
	if at.IsZero() || !at.Before(a.historyEnd) {
		return
	}
	day := reportDay(at)
	if a.firstDay.IsZero() || day.Before(a.firstDay) {
		a.firstDay = day
	}
	if !day.Before(a.historyEnd.AddDate(0, 0, -56)) {
		a.volumes[day]++
	}
}

func (a *workforceActivity) observeTicket(id string, at time.Time) {
	if id != "" && !at.IsZero() && (a.touch[id].IsZero() || at.Before(a.touch[id])) {
		a.touch[id] = at
	}
}

func (a *workforceActivity) observeBot(marker core.Record) {
	if marker.ConversationID != "" && !marker.CreatedAt.IsZero() &&
		(marker.Status == "answered" || marker.Status == "handoff") {
		a.bot[marker.ConversationID] = append(a.bot[marker.ConversationID], marker.CreatedAt)
	}
}

func (a *workforceActivity) observePlan(plan core.Record) {
	a.observeTicket(plan.ConversationID, plan.CreatedAt)
	if plan.Name == "resolve_ticket" && (plan.Status == "applied" || plan.Status == "auto_applied") {
		a.agentResolved[plan.ConversationID] = append(a.agentResolved[plan.ConversationID], plan.UpdatedAt)
	}
}

func (a *workforceActivity) observeResolution(ticket core.Record, channel string, from time.Time) {
	if ticket.Status != core.StatusResolved || ticket.ResolvedAt == nil ||
		ticket.ResolvedAt.Before(from) || !ticket.ResolvedAt.Before(a.cutoff) {
		return
	}
	closed := *ticket.ResolvedAt
	touched := ticket.AutomationID == "ai_handoff" ||
		!a.touch[ticket.ID].IsZero() && !a.touch[ticket.ID].After(closed)
	if !touched && ticket.ConversationID != "" {
		for _, at := range a.bot[ticket.ConversationID] {
			if !at.Before(ticket.CreatedAt) && !at.After(closed) {
				touched = true
				break
			}
		}
	}
	if !touched {
		return
	}
	isAgent := false
	for _, at := range a.agentResolved[ticket.ID] {
		// An approved AI plan sets ResolvedBy to the approver, so provenance wins.
		if at.Equal(closed) {
			isAgent = true
			break
		}
	}
	category := ticket.Priority
	if category == "" {
		category = "unknown"
	}
	key := [2]string{channel, category}
	row := a.byQueue[key]
	row.Channel, row.Priority = channel, category
	for _, metric := range []*HandoffResolution{&a.total, &row} {
		metric.Touched++
		switch {
		case isAgent:
			metric.Automated++
		case ticket.ResolvedBy != "" && ticket.ResolvedBy != "system":
			metric.Human++
		default:
			metric.Unknown++
		}
	}
	a.byQueue[key] = row
}

func (a *workforceActivity) report() WorkforceAnalytics {
	result := WorkforceAnalytics{AIToHuman: a.total, Forecast: []VolumeForecast{}, ByQueueType: []HandoffResolution{}}
	for _, row := range a.byQueue {
		result.ByQueueType = append(result.ByQueueType, row)
	}
	sort.Slice(result.ByQueueType, func(i, j int) bool {
		left, right := result.ByQueueType[i], result.ByQueueType[j]
		if left.Channel == right.Channel {
			return left.Priority < right.Priority
		}
		return left.Channel < right.Channel
	})
	for offset := range 7 {
		day := a.forecastStart.AddDate(0, 0, offset)
		point := VolumeForecast{Day: day.Format(time.DateOnly)}
		total := 0
		for week := 1; week <= 8; week++ {
			history := day.AddDate(0, 0, -7*week)
			if !history.Before(a.historyEnd) {
				continue
			}
			if a.firstDay.IsZero() || history.Before(a.firstDay) {
				break
			}
			total += a.volumes[history]
			point.Samples++
		}
		if point.Samples >= 2 {
			value := math.Round(10*float64(total)/float64(point.Samples)) / 10
			point.Expected = &value
		}
		result.Forecast = append(result.Forecast, point)
	}
	return result
}
