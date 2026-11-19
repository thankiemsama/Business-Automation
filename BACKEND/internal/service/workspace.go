package service

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"businessautomation/internal/core"
)

const notificationKind = "notifications"

type Workspace struct {
	core.Repository
	Clock      Clock
	IDs        IDGenerator
	mu         sync.Mutex
	epoch      string
	revision   uint64
	onResolved func(core.Record)
	watchers   map[string]map[chan string]struct{}
}

func NewWorkspace(repo core.Repository, clock Clock, ids IDGenerator) *Workspace {
	return &Workspace{Repository: repo, Clock: clock, IDs: ids, epoch: ids.NewID(), watchers: make(map[string]map[chan string]struct{})}
}

// SetOnResolved registers a callback fired when a ticket transitions to resolved.
func (s *Workspace) SetOnResolved(fn func(core.Record)) { s.onResolved = fn }
func (s *Workspace) Watch(tenant string) (string, <-chan string, func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	channel := make(chan string, 1)
	if s.watchers[tenant] == nil {
		s.watchers[tenant] = make(map[chan string]struct{})
	}
	s.watchers[tenant][channel] = struct{}{}
	return s.cursor(), channel, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.watchers[tenant], channel)
		if len(s.watchers[tenant]) == 0 {
			delete(s.watchers, tenant)
		}
	}
}

func guestScope(tenant, id string) string { return "chat:" + tenant + ":" + id }
func (s *Workspace) GuestScope(tenant, token string) (string, error) {
	if token == "" {
		return "", core.Fail(401, "Phiên chat không hợp lệ")
	}
	var scope string
	err := s.View(func(store core.Reader) error {
		session, ok := store.Record(tenant, tokenHash(token), "chat_sessions")
		if !ok {
			return core.Fail(401, "Phiên chat không hợp lệ")
		}
		scope = guestScope(tenant, session.ConversationID)
		return nil
	})
	return scope, err
}

func (s *Workspace) cursor() string { return s.epoch + "-" + strconv.FormatUint(s.revision, 10) }

func (s *Workspace) Update(run func(core.Writer) error) error { return s.updateAs("system", run) }

func (s *Workspace) ViewTenant(tenantID string, run func(core.Reader) error) error {
	return viewTenant(s.Repository, tenantID, run)
}

func (s *Workspace) UpdateTenant(tenantID string, run func(core.Writer) error) error {
	if tenantID == "" {
		return core.Fail(400, "Thiếu phạm vi doanh nghiệp")
	}
	return s.updateAsTenant(tenantID, "system", run)
}

func (s *Workspace) updateAs(actor string, run func(core.Writer) error) error {
	return s.updateAsTenant("", actor, run)
}

func (s *Workspace) updateAsTenant(tenantID, actor string, run func(core.Writer) error) error {
	tracked := &workspaceWriter{workspace: s, actor: actor, changed: make(map[string]bool)}
	apply := func(store core.Writer) error { tracked.Writer = store; return run(tracked) }
	var err error
	if tenantID == "" {
		err = s.Repository.Update(apply)
	} else {
		err = updateTenant(s.Repository, tenantID, apply)
	}
	if err != nil {
		return err
	}
	s.mu.Lock()
	for tenant := range tracked.changed {
		s.revision++
		for subscriber := range s.watchers[tenant] {
			select {
			case <-subscriber:
			default:
			}
			subscriber <- s.cursor()
		}
	}
	s.mu.Unlock()
	// Emit resolved-ticket hooks only after the transaction has committed and the
	// revision lock is released, so the hook's own nested transaction cannot
	// deadlock on s.mu.
	for _, ticket := range tracked.pendingResolved {
		if s.onResolved != nil {
			s.onResolved(ticket)
		}
	}
	return nil
}

type workspaceWriter struct {
	core.Writer
	workspace *Workspace
	actor     string
	changed   map[string]bool
	// pendingResolved collects tickets that transitioned to resolved during this
	// transaction. They are emitted after the commit so the hook can open its own
	// nested transaction without deadlocking on the outer write lock.
	pendingResolved []core.Record
}

func (w *workspaceWriter) Savepoint(run func(core.Writer) error) error {
	sub := &workspaceWriter{workspace: w.workspace, actor: w.actor, changed: make(map[string]bool)}
	if err := w.Writer.Savepoint(func(store core.Writer) error {
		sub.Writer = store
		return run(sub)
	}); err != nil {
		return err
	}
	for tenant := range sub.changed {
		w.changed[tenant] = true
	}
	w.pendingResolved = append(w.pendingResolved, sub.pendingResolved...)
	return nil
}

func (w *workspaceWriter) PutUser(user core.User) {
	previous, exists := w.UserByID(user.ID)
	w.Writer.PutUser(user)
	actor := w.actor
	if actor == "system" && !exists {
		actor = user.ID
	}
	w.audit(user.TenantID, actor, "users", user.ID, exists, auditUser(previous), auditUser(user))
	w.changed[user.TenantID] = true
}

func (w *workspaceWriter) PutRecord(record core.Record) {
	previous, exists := w.Writer.Record(record.TenantID, record.ID, record.Kind)
	w.Writer.PutRecord(record)
	if auditedKinds[record.Kind] {
		w.audit(record.TenantID, w.actor, record.Kind, record.ID, exists, auditRecord(previous), auditRecord(record))
	}
	if !exists && record.Kind == core.KindTickets && !strings.HasPrefix(record.AutomationID, "workflow:") {
		w.queueWorkflow("ticket_created", record)
	}
	if !exists && record.Kind == core.KindMessages && record.Sender == core.SenderCustomer {
		w.queueWorkflow("message_received", record)
	}
	if record.Kind == core.KindConversations && (!exists || record.Status != previous.Status) {
		w.changed[guestScope(record.TenantID, record.ID)] = true
	}
	if record.Kind == core.KindMessages && record.Sender != "note" {
		w.changed[guestScope(record.TenantID, record.ConversationID)] = true
	}
	switch record.Kind {
	case "ai_suggestions", core.KindCustomers, core.KindTickets, core.KindConversations, core.KindKnowledge, core.KindAutomations, core.KindMessages, notificationKind, "read_markers", "templates", "views":
		w.changed[record.TenantID] = true
	}
	if record.Kind == core.KindMessages && !exists && record.Sender == core.SenderCustomer {
		conversation, _ := w.Writer.Record(record.TenantID, record.ConversationID, core.KindConversations)
		w.notify(conversation, "message", "Tin nhắn mới")
	}
	if record.Kind == core.KindMessages && record.Email != "" && record.Status == "failed" && previous.Status == "pending" {
		if conversation, ok := w.Writer.Record(record.TenantID, record.ConversationID, core.KindConversations); ok {
			w.notify(conversation, "email_failed", "Email cần đối soát")
		}
	}
	if record.Kind == core.KindMessages && !exists && record.Sender != "note" {
		enqueueWebhooks(w, record.TenantID, core.EventMessageNew, map[string]any{
			"conversation": record.ConversationID, "message": record.ID,
		})
	}
	if record.Kind == core.KindTickets && !exists {
		enqueueWebhooks(w, record.TenantID, core.EventTicketCreated, map[string]any{
			"ticket": record.ID, "conversation": record.ConversationID, "priority": record.Priority,
		})
	}
	if (record.Kind == core.KindTickets || record.Kind == core.KindConversations) && record.AssigneeID != "" && record.AssigneeID != previous.AssigneeID {
		w.notify(record, "assigned", "Bạn được phân công")
	}
	if record.Kind == core.KindTickets && record.Status == core.StatusResolved && previous.Kind == core.KindTickets && previous.Status != core.StatusResolved {
		w.pendingResolved = append(w.pendingResolved, record)
	}
}

func (w *workspaceWriter) notify(record core.Record, category, title string) {
	for _, user := range w.Users(record.TenantID) {
		if user.Disabled || (record.AssigneeID != "" && user.ID != record.AssigneeID) {
			continue
		}
		now := w.workspace.Clock.Now().UTC()
		w.PutRecord(core.Record{ID: w.workspace.IDs.NewID(), TenantID: record.TenantID, Kind: notificationKind, AuthorID: user.ID, Status: category, Subject: title, Content: record.Subject, ConversationID: record.ID, Name: record.Kind, CreatedAt: now, UpdatedAt: now})
	}
}

func (s *Workspace) Notifications(user core.User) ([]core.Record, error) {
	items := []core.Record{}
	err := s.View(func(store core.Reader) error {
		for _, item := range store.Records(user.TenantID, notificationKind) {
			if item.AuthorID == user.ID {
				items = append(items, item)
			}
		}
		return nil
	})
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	return items, err
}

func (s *Workspace) ReadNotifications(user core.User, id string) error {
	return s.Update(func(store core.Writer) error {
		if id != "" {
			item, ok := store.Record(user.TenantID, id, notificationKind)
			if !ok || item.AuthorID != user.ID {
				return core.Fail(404, "Không tìm thấy thông báo")
			}
		}
		now := s.Clock.Now().UTC()
		for _, item := range store.Records(user.TenantID, notificationKind) {
			if item.AuthorID == user.ID && item.ReadAt == nil && (id == "" || item.ID == id) {
				item.ReadAt = &now
				store.PutRecord(item)
			}
		}
		return nil
	})
}

func (s *Workspace) Run(ctx context.Context) {
	if s.PushKey() != "" {
		go s.runPush(ctx)
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		s.sweepSLA()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Workspace) sweepSLA() {
	type expired struct{ tenant, id, stage string }
	var due []expired
	now := s.Clock.Now()
	err := s.View(func(store core.Reader) error {
		for _, tenant := range store.Tenants() {
			for _, ticket := range store.Records(tenant, core.KindTickets) {
				stage := slaStage(ticket, now)
				if stage != "" {
					if _, ok := store.Record(tenant, ticket.ID+":"+stage, "sla_markers"); !ok {
						due = append(due, expired{tenant, ticket.ID, stage})
					}
				}
			}
		}
		return nil
	})
	if err != nil || len(due) == 0 {
		return
	}
	_ = s.Update(func(store core.Writer) error {
		w := store.(*workspaceWriter)
		for _, key := range due {
			ticket, ok := store.Record(key.tenant, key.id, core.KindTickets)
			if !ok || slaStage(ticket, now) != key.stage {
				continue
			}
			marker := ticket.ID + ":" + key.stage
			if _, ok := store.Record(key.tenant, marker, "sla_markers"); ok {
				continue
			}
			w.PutRecord(core.Record{ID: marker, TenantID: key.tenant, Kind: "sla_markers"})
			w.changed[ticket.TenantID] = true
			if key.stage == "sla" {
				w.notify(ticket, "sla", "Phiếu quá hạn SLA")
				w.queueWorkflow("sla_breached", ticket)
				enqueueWebhooks(store, key.tenant, core.EventSLABreached, map[string]any{
					"ticket": ticket.ID,
				})
			}
		}
		return nil
	})
}

func slaStage(ticket core.Record, now time.Time) string {
	if ticketOverdue(ticket, now) {
		return "sla"
	}
	if ticketOverdue(ticket, now.Add(30*time.Minute)) {
		return "sla-warning"
	}
	return ""
}

func (s *Workspace) runPush(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		s.sendPush(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func normalizeTags(tags []string) ([]string, error) {
	if len(tags) > 20 {
		return nil, core.Fail(400, "Tối đa 20 nhãn")
	}
	result := make([]string, 0, len(tags))
	seen := map[string]bool{}
	for _, tag := range tags {
		tag = strings.ToLower(strings.Join(strings.Fields(tag), " "))
		if tag == "" || len([]rune(tag)) > 40 {
			return nil, core.Fail(400, "Nhãn phải có từ 1 đến 40 ký tự")
		}
		if !seen[tag] {
			seen[tag] = true
			result = append(result, tag)
		}
	}
	return result, nil
}
