package service

import (
	"encoding/json"
	"errors"
	"reflect"
	"sort"

	"businessautomation/internal/core"
)

var auditedKinds = map[string]bool{
	"customers": true, "tickets": true, "conversations": true, "messages": true, "knowledge": true, "automations": true,
	"workflow_rules": true, "report_schedules": true, "tenant_settings": true, "webhooks": true, "api_keys": true, "channel_config": true,
	"custom_fields": true, "crm_sync": true, "billing_plan": true,
	skillRoutingKind:     true,
	ticketAgentPlansKind: true, leadCampaignKind: true, qaReviewsKind: true,
	trainingScenariosKind: true, sparringCertificatesKind: true, evalSetsKind: true,
	governanceKind: true,
}

type auditChange struct {
	Before any `json:"before"`
	After  any `json:"after"`
}

func auditUser(u core.User) map[string]any {
	return map[string]any{"name": u.Name, "role": u.Role, "disabled": u.Disabled, "email_fingerprint": tokenHash(u.Email), "mfa_enabled": u.TOTPSecret != ""}
}
func auditRecord(r core.Record) map[string]any {
	result := map[string]any{"status": r.Status, "enabled": r.Enabled, "priority": r.Priority, "channel": r.Channel, "assignee": r.AssigneeID,
		"public": r.Public, "title_fingerprint": tokenHash(r.Name + "\x00" + r.Subject), "content_fingerprint": tokenHash(r.Content),
		"pii_fingerprint": tokenHash(r.Email + "\x00" + r.Phone), "tags": append([]string{}, r.Tags...), "keywords_fingerprint": tokenHash(joinForAudit(r.Keywords)),
		"attachment_changed": tokenHash(r.AttachmentName + "\x00" + r.AttachmentData), "custom_fields_fingerprint": tokenHash(r.CustomValues),
		"rating": r.Rating,
	}
	if r.Kind == ticketAgentPlansKind {
		result["tool"], result["ticket_id"], result["crm_lookup"] = r.Name, r.ConversationID, r.Enabled
	}
	if r.Kind == sparringCertificatesKind {
		result["model"], result["assessment_id"], result["valid_until"] = r.Name, r.ConversationID, r.EmailRetryAt
	}
	return result
}
func joinForAudit(v []string) string {
	raw, _ := json.Marshal(v)
	return string(raw)
}
func updateAs(repo core.Repository, actor string, fn func(core.Writer) error) error {
	if workspace, ok := repo.(*Workspace); ok {
		return workspace.updateAs(actor, fn)
	}
	return repo.Update(fn)
}
func viewTenant(repo core.Repository, tenantID string, fn func(core.Reader) error) error {
	if scoped, ok := repo.(core.TenantViewer); ok {
		return scoped.ViewTenant(tenantID, fn)
	}
	return errors.New("repository does not support tenant-scoped reads")
}

func updateTenant(repo core.Repository, tenantID string, fn func(core.Writer) error) error {
	if scoped, ok := repo.(core.TenantUpdater); ok {
		return scoped.UpdateTenant(tenantID, fn)
	}
	return errors.New("repository does not support tenant-scoped writes")
}

func updateAsTenant(repo core.Repository, user core.User, fn func(core.Writer) error) error {
	if user.TenantID == "" {
		return core.Fail(400, "Thiếu phạm vi doanh nghiệp")
	}
	if workspace, ok := repo.(*Workspace); ok {
		return workspace.updateAsTenant(user.TenantID, user.ID, fn)
	}
	return updateTenant(repo, user.TenantID, fn)
}

func (w *workspaceWriter) PutSession(id string, session core.Session) {
	w.Writer.PutSession(id, session)
	if user, ok := w.UserByID(session.UserID); ok {
		w.audit(user.TenantID, user.ID, "sessions", id[:min(12, len(id))], false, nil, map[string]any{"active": true, "expires_at": session.ExpiresAt})
	}
}
func (w *workspaceWriter) DeleteSession(id string) {
	if session, ok := w.Session(id); ok {
		if user, found := w.UserByID(session.UserID); found {
			w.audit(user.TenantID, user.ID, "sessions", id[:min(12, len(id))], true, map[string]any{"active": true}, map[string]any{"active": false})
		}
	}
	w.Writer.DeleteSession(id)
}
func (w *workspaceWriter) audit(tenant, actor, kind, id string, exists bool, before, after map[string]any) {
	diff := map[string]auditChange{}
	for field, value := range after {
		prior := before[field]
		if !exists {
			prior = nil
		}
		if !reflect.DeepEqual(prior, value) {
			diff[field] = auditChange{Before: prior, After: value}
		}
	}
	if len(diff) == 0 {
		return
	}
	payload, err := json.Marshal(diff)
	if err != nil {
		return
	}
	action := "updated"
	if !exists {
		action = "created"
	}
	now := w.workspace.Clock.Now()
	w.Writer.PutRecord(core.Record{ID: w.workspace.IDs.NewID(), TenantID: tenant, Kind: "audit_log", AuthorID: actor, Name: kind, ConversationID: id, Status: action, Content: string(payload), CreatedAt: now, UpdatedAt: now})
}
func (a *Admin) Audit(user core.User) ([]core.Record, error) {
	result := []core.Record{}
	err := a.repo.View(func(store core.Reader) error { result = store.Records(user.TenantID, "audit_log"); return nil })
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID > result[j].ID
		}
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result, err
}
