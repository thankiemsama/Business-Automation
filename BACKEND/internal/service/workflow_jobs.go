package service

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"businessautomation/internal/core"
)

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
	return !activity.AddDate(0, 0, rule.IdleDays).After(now)
}

func (o *Operations) scheduleIdle() {
	now := o.clock.Now().UTC()
	type candidate struct {
		rule   WorkflowRule
		ticket core.Record
	}
	jobs := []candidate{}
	_ = o.repo.View(func(store core.Reader) error {
		for _, tenant := range store.Tenants() {
			rules := workflowConfig[WorkflowRule](store, tenant, workflowRulesKind)
			tickets := store.Records(tenant, core.KindTickets)
			for _, rule := range rules {
				if !rule.Enabled || rule.Trigger != "ticket_idle" {
					continue
				}
				for _, ticket := range tickets {
					conversation, _ := store.Record(tenant, ticket.ConversationID, core.KindConversations)
					if !idleEligible(ticket, conversation, rule, now) {
						continue
					}
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
	})
	if len(jobs) == 0 {
		return
	}
	_ = o.repo.Update(func(store core.Writer) error {
		writer := store.(*workspaceWriter)
		for _, job := range jobs {
			writer.queueRule(job.rule, job.ticket, now)
		}
		return nil
	})
}

func (o *Operations) runJobs() {
	jobs := []core.Record{}
	_ = o.repo.View(func(store core.Reader) error {
		for _, tenant := range store.Tenants() {
			for _, job := range store.Records(tenant, workflowJobsKind) {
				if job.Enabled {
					jobs = append(jobs, job)
				}
			}
		}
		return nil
	})
	sort.Slice(jobs, func(i, j int) bool {
		if jobs[i].CreatedAt.Equal(jobs[j].CreatedAt) {
			return jobs[i].ID < jobs[j].ID
		}
		return jobs[i].CreatedAt.Before(jobs[j].CreatedAt)
	})
	for _, job := range jobs[:min(25, len(jobs))] {
		err := o.repo.Update(func(store core.Writer) error {
			current, ok := store.Record(job.TenantID, job.ID, workflowJobsKind)
			if !ok || !current.Enabled {
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
			o.runJob(store.(*workspaceWriter), current)
			current.Enabled = false
			current.Status = "processed"
			current.UpdatedAt = o.clock.Now().UTC()
			store.PutRecord(current)
			return nil
		})
		if err != nil {
			return
		}
	}
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
