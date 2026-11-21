package service

import (
	"time"

	"businessautomation/internal/core"
)

// meterAIResolution records one AI-driven resolution (a customer conversation or
// ticket completed without a human agent taking ownership) and charges the
// per-resolution billing metric when the tenant's plan caps it. This mirrors how
// ticket and AI-call metrics are metered in billing_quota, but keyed to outcomes
// instead of effort, so a tenant is billed for value delivered, not for calls.
//
// The marker record is idempotent on `key`: the same resolution event must never
// be charged twice (e.g. if the confirming message is reprocessed). Callers pass
// a stable key such as conversationID+":bot" or ticketID+messageID+":confirm".
func meterAIResolution(store core.Writer, tenant, key, source string, now time.Time) {
	id := tokenHash("ai-resolution:" + key)
	if _, ok := store.Record(tenant, id, "ai_resolutions"); !ok {
		store.PutRecord(core.Record{ID: id, TenantID: tenant, Kind: "ai_resolutions", AutomationID: source, CreatedAt: now, UpdatedAt: now})
	}
	_ = reserveBillingQuota(store, tenant, billingResolutions, now)
}
