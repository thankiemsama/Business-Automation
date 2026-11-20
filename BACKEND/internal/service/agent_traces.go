package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"time"

	"businessautomation/internal/core"
)

type AgentEvalSource struct {
	ConversationID string    `json:"conversation_id"`
	Kind           string    `json:"kind"`
	At             time.Time `json:"at"`
}

func agentEvalSources(store core.Reader, tenant string, now time.Time) ([]AgentEvalSource, error) {
	type candidate struct {
		ticket, kind string
		at           time.Time
	}
	var candidates []candidate
	for _, row := range store.Records(tenant, qaReviewsKind) {
		if row.Status == "completed" || row.Status == "insufficient_evidence" {
			candidates = append(candidates, candidate{row.ConversationID, "qa", row.UpdatedAt})
		}
	}
	for _, row := range store.Records(tenant, aiTriageEventsKind) {
		candidates = append(candidates, candidate{row.ConversationID, "triage", row.CreatedAt})
	}
	traces, err := agentTraceBuffer(store, tenant, now)
	if err != nil {
		return nil, err
	}
	for _, row := range traces {
		if row.Outcome == "applied" || row.Outcome == "auto_applied" || row.Outcome == "handed_off" {
			if plan, found := store.Record(tenant, row.RunID, ticketAgentPlansKind); found {
				candidates = append(candidates, candidate{plan.ConversationID, "trace", row.At})
			}
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].at.After(candidates[j].at) })
	results := []AgentEvalSource{}
	seen := map[string]bool{}
	for _, row := range candidates {
		ticket, found := store.Record(tenant, row.ticket, core.KindTickets)
		if !found || ticket.ConversationID == "" || seen[ticket.ConversationID] {
			continue
		}
		seen[ticket.ConversationID] = true
		results = append(results, AgentEvalSource{ticket.ConversationID, row.kind, row.at})
		if len(results) == 20 {
			break
		}
	}
	return results, nil
}

// Replays use approved scenarios and start only for tenants that previously certified this model.
func (s *AIService) queueAgentRegression(ctx context.Context) {
	type replay struct {
		owner core.User
		cases []string
	}
	var replays []replay
	if err := s.repo.View(func(store core.Reader) error {
		currentID := sparringCertificateID(s.model.model)
		for _, tenant := range store.Tenants() {
			if !aiOptIn(store, tenant, "ai_sparring") {
				continue
			}
			if _, current := store.Record(tenant, currentID, sparringCertificatesKind); current {
				continue
			}
			var previous core.Record
			for _, cert := range store.Records(tenant, sparringCertificatesKind) {
				if cert.Name == s.model.model && len(cert.Tags) == 2 && laterMessage(cert, previous) {
					previous = cert
				}
			}
			if previous.ID == "" {
				continue
			}
			for _, user := range store.Users(tenant) {
				if user.Role == core.RoleOwner && !user.Disabled {
					replays = append(replays, replay{user, previous.Tags})
					break
				}
			}
		}
		return nil
	}); err != nil {
		slog.Error("Agent regression discovery failed", "error", err)
		return
	}
	request := tokenHash("agent-regression:" + sparringCertificateID(s.model.model))
	for _, item := range replays {
		if ctx.Err() != nil {
			return
		}
		if _, err := s.startSparring(item.owner, "", request, true, item.cases); err != nil {
			slog.Warn("Agent regression could not be queued", "tenant", item.owner.TenantID, "error", err)
		}
	}
}

const agentTracesKind = "agent_traces"
const agentTraceLimit = 200
const agentTraceTTL = 7 * 24 * time.Hour

type AgentToolTrace struct {
	RunID   string    `json:"run_id"`
	Tool    string    `json:"tool"`
	Outcome string    `json:"outcome"`
	Model   string    `json:"model,omitempty"`
	Policy  string    `json:"policy"`
	At      time.Time `json:"at"`
}

func agentTraceBuffer(store core.Reader, tenant string, now time.Time) ([]AgentToolTrace, error) {
	rows := []AgentToolTrace{}
	item, ok := store.Record(tenant, tokenHash("agent-traces:"+tenant), agentTracesKind)
	if ok {
		if err := json.Unmarshal([]byte(item.Content), &rows); err != nil {
			return nil, err
		}
	}
	active := rows[:0]
	for _, row := range rows {
		if now.Before(row.At.Add(agentTraceTTL)) {
			active = append(active, row)
		}
	}
	return active, nil
}

func writeAgentToolTrace(store core.Writer, tenant, run, tool, outcome, model string, now time.Time) error {
	return writeAgentTrace(store, tenant, AgentToolTrace{RunID: run, Tool: tool, Outcome: outcome,
		Model: model, Policy: agentPlannerPolicyID()[:12], At: now})
}

func writeAgentTrace(store core.Writer, tenant string, trace AgentToolTrace) error {
	active, err := agentTraceBuffer(store, tenant, trace.At)
	if err != nil {
		return err
	}
	active = append(active, trace)
	raw, err := json.Marshal(active[max(0, len(active)-agentTraceLimit):])
	if err != nil {
		return err
	}
	store.PutRecord(core.Record{ID: tokenHash("agent-traces:" + tenant), TenantID: tenant,
		Kind: agentTracesKind, Content: string(raw), UpdatedAt: trace.At})
	return nil
}

func (s *AIService) appendAgentToolTrace(tenant, run, tool, outcome string) {
	model := ""
	if s.model != nil {
		model = s.model.model
	}
	if err := s.repo.UpdateTenant(tenant, func(store core.Writer) error {
		return writeAgentToolTrace(store, tenant, run, tool, outcome, model, s.clock.Now().UTC())
	}); err != nil {
		slog.Error("Agent tool trace write failed", "error", err)
	}
}

func (s *AIService) AgentToolTraces(user core.User) ([]AgentToolTrace, error) {
	if user.Role != core.RoleOwner {
		return nil, core.Fail(403, "Chỉ owner được xem dấu vết công cụ AI")
	}
	var rows []AgentToolTrace
	err := s.repo.ViewTenant(user.TenantID, func(store core.Reader) error {
		var err error
		rows, err = agentTraceBuffer(store, user.TenantID, s.clock.Now())
		return err
	})
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	return rows, err
}
