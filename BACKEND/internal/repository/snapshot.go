package repository

import (
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
	record.Tags = append([]string(nil), record.Tags...)
	record.Keywords = append([]string(nil), record.Keywords...)
	return record, true
}

func (s snapshot) Records(tenantID, kind string) []core.Record {
	records := []core.Record{}
	for _, record := range s.state.Records {
		if record.TenantID == tenantID && (kind == "" || record.Kind == kind) {
			record.Keywords = append([]string(nil), record.Keywords...)
			records = append(records, record)
		}
	}
	return records
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
	record.Keywords = append([]string(nil), record.Keywords...)
	s.state.Records[record.ID] = record
}

var _ core.Repository = (*File)(nil)
