package service

import (
	"encoding/json"
	"log/slog"
	"math"
	"os"

	"businessautomation/internal/core"
)

const aiQuotaKind = "ai_daily_quota"

var errAIQuota = core.Fail(429, "Doanh nghiệp đã hết quota AI hôm nay")

type providerUsage struct {
	PromptTokens     int     `json:"prompt_tokens"`
	CompletionTokens int     `json:"completion_tokens"`
	InputTokens      int     `json:"input_tokens"`
	OutputTokens     int     `json:"output_tokens"`
	Seconds          float64 `json:"seconds"`
}

type aiRate struct {
	InputPerMillion  float64 `json:"input_per_million"`
	OutputPerMillion float64 `json:"output_per_million"`
	PerMinute        float64 `json:"per_minute"`
}

func (p *OpenAIKnowledge) ConfigureUsage(repo core.Repository, clock Clock, ids IDGenerator) {
	p.reserve = func(tenant string) error {
		now := clock.Now().UTC()
		day := reportDay(now).Format("2006-01-02")
		return updateTenant(repo, tenant, func(store core.Writer) error {
			settings, err := tenantSettings(store, tenant)
			if err != nil || settings.DailyAIRequests <= 0 {
				return err
			}
			id := tokenHash("ai-quota:" + day)
			record, found := store.Record(tenant, id, aiQuotaKind)
			if !found {
				record = core.Record{ID: id, TenantID: tenant, Kind: aiQuotaKind, CreatedAt: now}
			}
			if record.Sequence >= uint64(settings.DailyAIRequests) {
				return errAIQuota
			}
			record.Sequence++
			record.UpdatedAt = now
			store.PutRecord(record)
			return nil
		})
	}
	rates := map[string]aiRate{}
	if err := json.Unmarshal([]byte(os.Getenv("AI_USD_RATES")), &rates); err != nil && os.Getenv("AI_USD_RATES") != "" {
		slog.Warn("AI_USD_RATES không đúng định dạng")
		rates = nil
	}
	p.meter = func(tenant, model string, usage providerUsage) {
		rate, configured := rates[model]
		input, output := max(usage.PromptTokens, usage.InputTokens), max(usage.CompletionTokens, usage.OutputTokens)
		var cost *float64
		if configured {
			value := 0.0
			switch {
			case usage.Seconds > 0 && rate.PerMinute > 0:
				value = usage.Seconds * (rate.PerMinute / 60)
			case input > 0 && rate.InputPerMillion > 0 && output >= 0 && (output == 0 || rate.OutputPerMillion > 0):
				value = float64(input)*(rate.InputPerMillion/1_000_000) + float64(output)*(rate.OutputPerMillion/1_000_000)
			}
			if value > 0 && !math.IsInf(value, 0) {
				cost = &value
			}
		}
		now := clock.Now().UTC()
		err := updateTenant(repo, tenant, func(store core.Writer) error {
			store.PutRecord(core.Record{ID: ids.NewID(), TenantID: tenant, Kind: "ai_usage_samples", Name: model,
				AICostUSD: cost, CreatedAt: now, UpdatedAt: now})
			return nil
		})
		if err != nil {
			slog.Warn("Không ghi được chi phí AI", "tenant", tenant, "model", model, "error", err)
		}
	}
}

func (p *OpenAIKnowledge) reserveRequest(tenant string) error {
	if p.reserve != nil && tenant != "" {
		return p.reserve(tenant)
	}
	return nil
}

func (p *OpenAIKnowledge) recordUsage(tenant, model string, usage providerUsage) {
	if p.meter != nil && tenant != "" {
		p.meter(tenant, model, usage)
	}
}
