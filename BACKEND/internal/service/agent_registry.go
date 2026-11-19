package service

import "businessautomation/internal/core"

type agentPreset struct {
	ID   string
	Flag string
}

var agentRegistry = []agentPreset{
	{ID: "sla_watch", Flag: "ai_teammate_sla"},
	{ID: "inbox_handoff", Flag: "ai_teammate_inbox"},
}

func agentPresetEnabled(store core.Reader, tenant, presetID string) bool {
	if !aiOptIn(store, tenant, "ai_teammate") {
		return false
	}
	for _, preset := range agentRegistry {
		if preset.ID == presetID {
			return aiOptIn(store, tenant, preset.Flag)
		}
	}
	return false
}
