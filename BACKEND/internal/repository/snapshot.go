package repository

import (
	"cmp"
	"slices"
	"time"

	"businessautomation/internal/core"
)

type databaseState struct {
	Users    map[string]core.User    `json:"users"`
	Sessions map[string]core.Session `json:"sessions"`
	Records  map[string]core.Record  `json:"records"`
}

// snapshot is private to the adapter; callers never receive storage maps.
type snapshot struct{ state *databaseState }

func (s snapshot) UserByEmail(email string) (core.User, bool) {
	for _, user := range s.state.Users {
		if user.Email == email {
			return user, true
		}
	}
	return core.User{}, false
}

func (s snapshot) UserByID(id string) (core.User, bool) {
	user, ok := s.state.Users[id]
	return user, ok
}

func (s snapshot) Tenants() []string {
	seen := map[string]bool{}
	tenants := []string{}
	for _, user := range s.state.Users {
		if !seen[user.TenantID] {
			seen[user.TenantID] = true
			tenants = append(tenants, user.TenantID)
		}
	}
	return tenants
}

func (s snapshot) Users(tenantID string) []core.User {
	users := []core.User{}
	for _, user := range s.state.Users {
		if user.TenantID == tenantID {
			users = append(users, user)
		}
	}
	return users
}

func (s snapshot) Sessions(user string) map[string]core.Session {
	result := map[string]core.Session{}
	for id, session := range s.state.Sessions {
		if session.UserID == user {
			result[id] = session
		}
	}
	return result
}
func (s snapshot) Session(key string) (core.Session, bool) {
	session, ok := s.state.Sessions[key]
	return session, ok
}

func (s snapshot) Record(tenantID, id, kind string) (core.Record, bool) {
	record, ok := s.state.Records[id]
	if !ok || record.TenantID != tenantID || record.Kind != kind {
		return core.Record{}, false
	}
	return detachRecord(record), true
}

func (s snapshot) Records(tenantID, kind string) []core.Record {
	records := []core.Record{}
	for _, record := range s.state.Records {
		if record.TenantID == tenantID && (kind == "" || record.Kind == kind) {
			records = append(records, detachRecord(record))
		}
	}
	return records
}

func (s snapshot) DueEmailReplies(now time.Time, limit int) []core.Record {
	records := []core.Record{}
	for _, record := range s.state.Records {
		if record.Kind == core.KindMessages && record.Status == "pending" && record.Sender == core.SenderAgent &&
			record.Email != "" && (record.EmailRetryAt == nil || !record.EmailRetryAt.After(now)) {
			records = append(records, record)
		}
	}
	slices.SortFunc(records, func(a, b core.Record) int {
		if order := a.CreatedAt.Compare(b.CreatedAt); order != 0 {
			return order
		}
		return cmp.Compare(a.ID, b.ID)
	})
	records = records[:min(len(records), limit)]
	for i := range records {
		records[i] = detachRecord(records[i])
	}
	return records
}

func (s snapshot) ConversationMessages(tenantID, conversationID string) []core.Record {
	records := []core.Record{}
	for _, record := range s.state.Records {
		if record.TenantID == tenantID && record.Kind == core.KindMessages && record.ConversationID == conversationID {
			records = append(records, detachRecord(record))
		}
	}
	return records
}

func detachRecord(record core.Record) core.Record {
	record.Keywords = slices.Clone(record.Keywords)
	record.Tags = slices.Clone(record.Tags)
	if record.AICostUSD != nil {
		value := *record.AICostUSD
		record.AICostUSD = &value
	}
	for _, field := range []**time.Time{&record.FirstResponseAt, &record.ResolvedAt, &record.RatedAt,
		&record.ResponseDueAt, &record.ResolutionDueAt, &record.EscalatedAt, &record.ReadAt, &record.EmailRetryAt} {
		if *field != nil {
			value := **field
			*field = &value
		}
	}
	return record
}

func (s snapshot) PutUser(user core.User)                      { s.state.Users[user.ID] = user }
func (s snapshot) PutSession(key string, session core.Session) { s.state.Sessions[key] = session }
func (s snapshot) DeleteSession(key string)                    { delete(s.state.Sessions, key) }
func (s snapshot) RevokeSessions(id string) {
	for key, session := range s.state.Sessions {
		if session.UserID == id {
			delete(s.state.Sessions, key)
		}
	}
}

func (s snapshot) ExpireSessions(now time.Time) {
	for k, session := range s.state.Sessions {
		if !session.ExpiresAt.After(now) {
			delete(s.state.Sessions, k)
		}
	}
}

func (s snapshot) PutRecord(record core.Record) {
	if core.VersionedResource(record.Kind) {
		record.Version = s.state.Records[record.ID].Version + 1
	}
	s.state.Records[record.ID] = detachRecord(record)
}

var _ core.Repository = (*File)(nil)

func (s snapshot) IdleTickets(tenantID string, cutoff time.Time) []core.Record {
	records := []core.Record{}
	for _, ticket := range s.state.Records {
		if ticket.TenantID != tenantID || ticket.Kind != core.KindTickets || ticket.Status == core.StatusResolved {
			continue
		}
		activity := ticket.UpdatedAt
		if activity.IsZero() {
			activity = ticket.CreatedAt
		}
		if conversation, ok := s.Record(tenantID, ticket.ConversationID, core.KindConversations); ok && conversation.UpdatedAt.After(activity) {
			activity = conversation.UpdatedAt
		}
		if !activity.After(cutoff) {
			records = append(records, detachRecord(ticket))
		}
	}
	slices.SortFunc(records, func(a, b core.Record) int {
		first, second := workflowActivityAt(a), workflowActivityAt(b)
		if order := first.Compare(*second); order != 0 {
			return order
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return records
}

func workflowActivityAt(record core.Record) *time.Time {
	switch record.Kind {
	case core.KindConversations:
		return &record.UpdatedAt
	case core.KindTickets:
		if record.Status == core.StatusResolved {
			return nil
		}
		if record.UpdatedAt.IsZero() {
			return &record.CreatedAt
		}
		return &record.UpdatedAt
	default:
		return nil
	}
}

func pendingWorkflowAt(record core.Record) *time.Time {
	if record.Kind != core.KindWorkflowJobs || !record.Enabled {
		return nil
	}
	if core.IsAIWorkflowJob(record.Subject) && record.EmailRetryAt != nil {
		return record.EmailRetryAt
	}
	return &record.CreatedAt
}

func (s snapshot) PendingWorkflowJobs(limit int) []core.Record {
	return s.pendingWorkflowJobs(limit, time.Time{}, false)
}

func (s snapshot) PendingAIWorkflowJobs(limit int, now time.Time) []core.Record {
	return s.pendingWorkflowJobs(limit, now, true)
}

func (s snapshot) pendingWorkflowJobs(limit int, now time.Time, ai bool) []core.Record {
	if limit <= 0 {
		return nil
	}
	tenants := map[string]bool{}
	for _, tenant := range s.Tenants() {
		tenants[tenant] = true
	}
	jobs := []core.Record{}
	for _, job := range s.state.Records {
		due := pendingWorkflowAt(job)
		if tenants[job.TenantID] && due != nil && core.IsAIWorkflowJob(job.Subject) == ai && (!ai || !due.After(now)) {
			jobs = append(jobs, job)
		}
	}
	slices.SortFunc(jobs, func(a, b core.Record) int {
		if order := pendingWorkflowAt(a).Compare(*pendingWorkflowAt(b)); order != 0 {
			return order
		}
		if order := cmp.Compare(a.ID, b.ID); order != 0 {
			return order
		}
		return cmp.Compare(a.TenantID, b.TenantID)
	})
	jobs = jobs[:min(limit, len(jobs))]
	for i := range jobs {
		jobs[i] = detachRecord(jobs[i])
	}
	return jobs
}

func (s snapshot) LockWorkflowJob(tenantID, id string) (core.Record, bool) {
	job, ok := s.Record(tenantID, id, core.KindWorkflowJobs)
	return job, ok && job.Enabled
}

func (s snapshot) Savepoint(run func(core.Writer) error) error {
	next := clone(*s.state)
	if err := run(snapshot{state: &next}); err != nil {
		return err
	}
	*s.state = next
	return nil
}
