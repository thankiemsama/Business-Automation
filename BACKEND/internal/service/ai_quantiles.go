package service

import (
	"math"
	"sort"
	"time"

	"businessautomation/internal/core"
)

type slaFeatures struct {
	channel, priority string
	hour, workload    int
}

func (f slaFeatures) values() [4]float64 {
	hour := 2 * math.Pi * float64(f.hour) / 24
	return [4]float64{1, math.Sin(hour), math.Cos(hour), math.Log1p(float64(f.workload)) / 5}
}

type slaSample struct {
	slaFeatures
	duration time.Duration
}

type slaQuantile struct {
	weights              [4]float64
	channels, priorities map[string]float64
}

func (m slaQuantile) estimate(f slaFeatures) float64 {
	values := f.values()
	value := m.channels[f.channel] + m.priorities[f.priority]
	for i := range values {
		value += values[i] * m.weights[i]
	}
	return value
}

// Fit pinball loss on log-minutes with regularized channel and priority effects.
func fitSLAQuantile(samples []slaSample, q float64) slaQuantile {
	model := slaQuantile{channels: map[string]float64{}, priorities: map[string]float64{}}
	if len(samples) < 20 {
		return model
	}
	targets := make([]float64, len(samples))
	for i, sample := range samples {
		targets[i] = math.Log1p(sample.duration.Minutes())
	}
	sorted := append([]float64{}, targets...)
	sort.Float64s(sorted)
	model.weights[0] = sorted[int(math.Ceil(q*float64(len(sorted))))-1]
	for pass := 0; pass < 60; pass++ {
		step := .08 / math.Sqrt(1+float64(pass)/10)
		for i, sample := range samples {
			f := sample.slaFeatures
			gradient := q
			if targets[i] < model.estimate(f) {
				gradient--
			}
			values := f.values()
			for j := range values {
				model.weights[j] += step * (gradient*values[j] - .02*model.weights[j]*float64(min(j, 1)))
			}
			model.channels[f.channel] += step * (gradient - .05*model.channels[f.channel])
			model.priorities[f.priority] += step * (gradient - .05*model.priorities[f.priority])
		}
	}
	return model
}

type slaForecast struct {
	p50, p90 time.Duration
}

func forecastSLA(median, tail slaQuantile, f slaFeatures) slaForecast {
	duration := func(model slaQuantile) time.Duration {
		minutes := math.Expm1(model.estimate(f))
		return time.Duration(math.Max(1, math.Min(30*24*60, minutes)) * float64(time.Minute))
	}
	p50, p90 := duration(median), duration(tail)
	return slaForecast{p50, max(p50, p90)}
}

type workloadHistory struct {
	starts, ends []time.Time
}

func slaWorkloads(tickets []core.Record) map[string]*workloadHistory {
	histories := map[string]*workloadHistory{}
	for _, ticket := range tickets {
		if histories[ticket.AssigneeID] == nil {
			histories[ticket.AssigneeID] = &workloadHistory{}
		}
		history := histories[ticket.AssigneeID]
		history.starts = append(history.starts, ticket.CreatedAt)
		if ticket.ResolvedAt != nil && !ticket.ResolvedAt.Before(ticket.CreatedAt) {
			history.ends = append(history.ends, *ticket.ResolvedAt)
		}
	}
	for _, history := range histories {
		sort.Slice(history.starts, func(i, j int) bool { return history.starts[i].Before(history.starts[j]) })
		sort.Slice(history.ends, func(i, j int) bool { return history.ends[i].Before(history.ends[j]) })
	}
	return histories
}

func (h *workloadHistory) at(when time.Time) int {
	started := sort.Search(len(h.starts), func(i int) bool { return !h.starts[i].Before(when) })
	ended := sort.Search(len(h.ends), func(i int) bool { return h.ends[i].After(when) })
	return max(0, started-ended)
}
