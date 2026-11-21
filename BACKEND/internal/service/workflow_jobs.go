package service

import (
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"businessautomation/internal/core"
)

const aiJobLease = 2 * time.Minute
const maxAIJobAttempts = 3

// workflowJobBatch bounds how many jobs a tick claims; workflowJobsPerTenant
// caps the share of that batch for one tenant so a noisy tenant cannot
// starve the others. Leftover jobs remain pending and are picked up on
// later ticks.
const workflowJobBatch = 25
const workflowJobsPerTenant = 5

// capJobsPerTenant keeps the original due-time order while limiting how
// many jobs from a single tenant appear in one batch.
func capJobsPerTenant(jobs []core.Record) []core.Record {
	if len(jobs) <= workflowJobsPerTenant {
		return jobs
	}
	picked := make([]core.Record, 0, len(jobs))
	count := map[string]int{}
	for _, job := range jobs {
		if count[job.TenantID] < workflowJobsPerTenant {
			picked = append(picked, job)
			count[job.TenantID]++
		}
	}
	return picked
}

func idleEligible(ticket, conversation core.Record, rule WorkflowRule, now time.Time) bool {
	if ticket.Status == core.StatusResolved {
		return false
	}
	activity := ticket.UpdatedAt
	if activity.IsZero() {
		activity = ticket.CreatedAt
	}
	if conversation.UpdatedAt.After(activity) {
		activity = conversation.UpdatedAt
	}
	return !activity.After(now.UTC().AddDate(0, 0, -rule.IdleDays))
}

func (o *Operations) scheduleIdle() {
	now := o.clock.Now().UTC()
	type candidate struct {
		rule   WorkflowRule
		ticket core.Record
	}
	jobs := []candidate{}
	if err := o.repo.View(func(store core.Reader) error {
		if indexed, ok := store.(interface {
			IdleWorkflowCandidates(time.Time, core.IdleWorkflowCandidate, int) []core.IdleWorkflowCandidate
		}); ok {
			enabled := map[string]bool{}
			const batchSize = 256
			for range 8 {
				page := indexed.IdleWorkflowCandidates(now, o.idleAfter, batchSize)
				for _, item := range page {
					o.idleAfter = core.IdleWorkflowCandidate{Activity: item.Activity, Ticket: core.Record{ID: item.Ticket.ID, TenantID: item.Ticket.TenantID}, Rule: core.Record{ID: item.Rule.ID}}
					tenant := item.Ticket.TenantID
					active, checked := enabled[tenant]
					if !checked {
						active = featureEnabled(store, tenant, "workflows")
						enabled[tenant] = active
					}
					if !active {
						continue
					}
					var rule WorkflowRule
					if json.Unmarshal([]byte(item.Rule.Content), &rule) == nil &&
						conditionsMatch(rule.Conditions, item.Channel, item.Ticket.Priority, item.Ticket.Status, item.Ticket.Content) {
						jobs = append(jobs, candidate{rule, item.Ticket})
						if len(jobs) == 100 {
							return nil
						}
					}
				}
				if len(page) < batchSize {
					o.idleAfter = core.IdleWorkflowCandidate{}
					return nil
				}
			}
			return nil
		}
		for _, tenant := range store.Tenants() {
			if !featureEnabled(store, tenant, "workflows") {
				continue
			}
			rules := workflowConfig[WorkflowRule](store, tenant, workflowRulesKind)
			for _, rule := range rules {
				if !rule.Enabled || rule.Trigger != "ticket_idle" {
					continue
				}
				for _, ticket := range store.IdleTickets(tenant, now.AddDate(0, 0, -rule.IdleDays)) {
					conversation, _ := store.Record(tenant, ticket.ConversationID, core.KindConversations)
					if !conditionsMatch(rule.Conditions, conversation.Channel, ticket.Priority, ticket.Status, ticket.Content) {
						continue
					}
					if job, exists := store.Record(tenant, tokenHash("workflow:"+rule.ID+":"+ticket.ID), workflowJobsKind); !exists || job.Status == "postponed" {
						jobs = append(jobs, candidate{rule, ticket})
					}
					if len(jobs) >= 100 {
						return nil
					}
				}
			}
		}
		return nil
	}); err != nil {
		slog.Error("workflow idle candidates query failed", "error", err)
		return
	}
	if len(jobs) == 0 {
		return
	}
	if err := o.repo.Update(func(store core.Writer) error {
		writer := store.(*workspaceWriter)
		for _, job := range jobs {
			tenant := job.ticket.TenantID
			ticket, found := store.Record(tenant, job.ticket.ID, core.KindTickets)
			config, exists := store.Record(tenant, job.rule.ID, workflowRulesKind)
			var rule WorkflowRule
			if !found || !exists || !config.Enabled || config.Status == "removed" ||
				!featureEnabled(store, tenant, "workflows") || json.Unmarshal([]byte(config.Content), &rule) != nil || rule.Trigger != "ticket_idle" {
				continue
			}
			conversation, _ := store.Record(tenant, ticket.ConversationID, core.KindConversations)
			if idleEligible(ticket, conversation, rule, now) &&
				conditionsMatch(rule.Conditions, conversation.Channel, ticket.Priority, ticket.Status, ticket.Content) {
				writer.queueRule(rule, ticket, now)
			}
		}
		return nil
	}); err != nil {
		slog.Error("workflow idle enqueue failed", "error", err)
	}
}

func (o *Operations) runJobs() {
	var jobs []core.Record
	if err := o.repo.View(func(store core.Reader) error {
		jobs = store.PendingWorkflowJobs(workflowJobBatch)
		return nil
	}); err != nil {
		slog.Error("workflow queue read failed", "error", err)
		return
	}
	jobs = capJobsPerTenant(jobs)
	for _, job := range jobs {
		err := o.repo.UpdateTenant(job.TenantID, func(store core.Writer) error {
			current, ok := store.LockWorkflowJob(job.TenantID, job.ID)
			if !ok {
				return nil
			}
			if current.Subject == "ticket_idle" {
				ticket, valid := store.Record(current.TenantID, current.ConversationID, core.KindTickets)
				ruleRecord, exists := store.Record(current.TenantID, current.AutomationID, workflowRulesKind)
				var rule WorkflowRule
				conversation, _ := store.Record(current.TenantID, ticket.ConversationID, core.KindConversations)
				if valid && exists && ruleRecord.Enabled && json.Unmarshal([]byte(ruleRecord.Content), &rule) == nil && !idleEligible(ticket, conversation, rule, o.clock.Now()) {
					current.Enabled = false
					current.Status = "postponed"
					store.PutRecord(current)
					return nil
				}
			}
			err := store.Savepoint(func(inner core.Writer) error {
				o.runJob(inner.(*workspaceWriter), current)
				return nil
			})
			if err != nil && !errors.Is(err, errBillingQuota) {
				return err
			}
			current.Enabled = false
			current.Status = "processed"
			if err != nil {
				current.Status = "blocked_quota"
			}
			current.UpdatedAt = o.clock.Now().UTC()
			store.PutRecord(current)
			return nil
		})
		if err != nil {
			slog.Error("workflow job transaction failed", "tenant", job.TenantID, "job", job.ID, "error", err)
			return
		}
	}
}

func (o *Operations) runAIJobs() {
	var jobs []core.Record
	if err := o.repo.View(func(store core.Reader) error {
		jobs = store.PendingAIWorkflowJobs(workflowJobBatch, o.clock.Now())
		return nil
	}); err != nil {
		slog.Error("AI review queue read failed", "error", err)
		return
	}
	jobs = capJobsPerTenant(jobs)
	for _, job := range jobs {
		var ran bool
		switch job.Subject {
		case core.WorkflowAIKnowledgeDraft:
			ran = o.runKnowledgeDraft(job)
		case core.WorkflowAIExtractFields:
			ran = o.runFieldExtraction(job)
		case core.WorkflowAIReview:
			ran = o.runResolvedReview(job)
		case core.WorkflowSparringAssessment:
			ran = o.runSparringAssessment(job)
		case core.WorkflowRealtimeQA:
			ran = o.runRealtimeQA(job)
		case core.WorkflowMultimodalQA, core.WorkflowSalesReview, core.WorkflowOpsReview:
			ran = o.runConversationReview(job)
		}
		if ran {
			return
		}
	}
}

func (o *Operations) deferAIJobForQuota(job core.Record, attempt uint64, cause error) bool {
	if !errors.Is(cause, errAIQuota) {
		return false
	}
	err := o.repo.UpdateTenant(job.TenantID, func(store core.Writer) error {
		current, found := store.LockWorkflowJob(job.TenantID, job.ID)
		if !found || current.Sequence != attempt {
			return nil
		}
		now := o.clock.Now().UTC()
		nextDay := reportDay(now).AddDate(0, 0, 1)
		current.Sequence--
		if current.Subject == core.WorkflowSparringAssessment && current.EmailAttempts > 0 {
			current.EmailAttempts--
		}
		current.EmailRetryAt = &nextDay
		current.Status, current.UpdatedAt = "pending", now
		store.PutRecord(current)
		return nil
	})
	if err != nil {
		slog.Error("AI quota deferral failed", "job", job.ID, "error", err)
	}
	return true
}

func (o *Operations) runJob(w *workspaceWriter, job core.Record) {
	if !featureEnabled(w, job.TenantID, "workflows") {
		return
	}
	config, exists := w.Record(job.TenantID, job.AutomationID, workflowRulesKind)
	var rule WorkflowRule
	if !exists || !config.Enabled || json.Unmarshal([]byte(config.Content), &rule) != nil || rule.Trigger != job.Subject {
		return
	}
	source, exists := w.Record(job.TenantID, job.ConversationID, job.Name)
	if !exists {
		return
	}
	now := o.clock.Now().UTC()
	conversation, _ := w.Record(job.TenantID, source.ConversationID, core.KindConversations)
	target := source
	priority, status := source.Priority, source.Status
	if source.Kind == core.KindMessages {
		if conversation.ID == "" || conversation.Status != core.StatusOpen {
			return
		}
		target = conversation
		status = conversation.Status
		var newest core.Record
		for _, ticket := range w.Records(job.TenantID, core.KindTickets) {
			if ticket.ConversationID == conversation.ID && ticket.Status != core.StatusResolved && (newest.ID == "" || ticket.CreatedAt.After(newest.CreatedAt) || ticket.CreatedAt.Equal(newest.CreatedAt) && ticket.ID > newest.ID) {
				newest = ticket
			}
		}
		priority = newest.Priority
	}
	if rule.Trigger == "ticket_idle" && !idleEligible(source, conversation, rule, now) {
		return
	}
	if rule.Trigger == "sla_breached" && !ticketOverdue(source, now) {
		return
	}
	if !conditionsMatch(rule.Conditions, conversation.Channel, priority, status, source.Content) {
		return
	}
	var owner core.User
	for _, user := range w.Users(job.TenantID) {
		if user.Role == core.RoleOwner && !user.Disabled {
			owner = user
			break
		}
	}
	actionIndex := 0
	apply := func(actions []WorkflowAction) {
		for _, action := range actions {
			i := actionIndex
			actionIndex++
			switch action.Type {
			case "notify_owner":
				if owner.ID != "" {
					title := strings.TrimSpace(action.Value)
					if title == "" {
						title = rule.Name
					}
					w.PutRecord(core.Record{ID: o.repo.IDs.NewID(), TenantID: job.TenantID, Kind: notificationKind, AuthorID: owner.ID, Name: target.Kind, ConversationID: target.ID, Subject: title, Content: target.Subject, Status: "workflow", CreatedAt: now, UpdatedAt: now})
				}
			case "email_owner":
				if owner.Email != "" {
					o.queueMail(w, job.TenantID, tokenHash(job.ID+":email:"+strconv.Itoa(i)), owner.ID, "Workflow Nexa · "+rule.Name, "Workflow "+rule.Name+" đã chạy cho "+target.Subject+" (#"+target.ID+").", now)
				}
			case "assign", "assign_owner":
				assignee := action.Value
				if action.Type == "assign_owner" {
					assignee = owner.ID
				}
				member, ok := w.UserByID(assignee)
				if ok && !member.Disabled && member.TenantID == job.TenantID && target.Status != core.StatusResolved {
					target.AssigneeID, target.UpdatedAt = assignee, now
					if action.Type == "assign_owner" && target.Kind == core.KindTickets && (rule.Trigger == "sla_breached" || rule.Trigger == "ticket_idle") {
						target.EscalatedAt = &now
					}
					w.PutRecord(target)
				}
			case "resolve_ticket":
				if target.Kind == core.KindTickets && target.Status != core.StatusResolved {
					target.Status, target.UpdatedAt, target.ResolvedAt = core.StatusResolved, now, &now
					w.PutRecord(target)
				}
			case "create_ticket":
				if strings.HasPrefix(source.AutomationID, "workflow:") {
					continue
				}
				origin := "workflow:" + rule.ID + ":" + strconv.Itoa(i)
				if source.Kind == core.KindMessages {
					tickets := w.Records(job.TenantID, core.KindTickets)
					if hasOpenAutomationTicket(tickets, conversation.ID, origin) || rule.MigratedFrom != "" && hasOpenAutomationTicket(tickets, conversation.ID, rule.MigratedFrom) {
						continue
					}
				}
				subject := strings.TrimSpace(action.Value)
				if subject == "" {
					subject = rule.Name
				}
				w.PutRecord(core.Record{ID: o.repo.IDs.NewID(), TenantID: job.TenantID, Kind: core.KindTickets, AutomationID: origin, Subject: subject, Content: source.Content, CustomerID: target.CustomerID, ConversationID: conversation.ID, Status: core.StatusOpen, Priority: core.PriorityNormal, CreatedAt: now, UpdatedAt: now})
			}
		}
	}
	apply(rule.Actions)
	if rule.Branch != nil {
		if conditionsMatch(rule.Branch.Conditions, conversation.Channel, priority, target.Status, source.Content) {
			apply(rule.Branch.Then)
		} else {
			apply(rule.Branch.Otherwise)
		}
	}
}
