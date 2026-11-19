package repository

import (
	"errors"
	"time"

	"businessautomation/internal/core"
)

var errTenantScope = errors.New("operation is outside the business transaction tenant scope")

type tenantReader struct {
	store core.Reader
	id    string
	err   error
}

type tenantWriter struct {
	tenantReader
	store core.Writer
}

func withTenantReader(store core.Reader, tenantID string, run func(core.Reader) error) error {
	scoped := &tenantReader{store: store, id: tenantID}
	callbackErr := run(scoped)
	return errors.Join(scoped.err, callbackErr)
}

func withTenantWriter(store core.Writer, tenantID string, run func(core.Writer) error) error {
	scoped := &tenantWriter{tenantReader: tenantReader{store: store, id: tenantID}, store: store}
	callbackErr := run(scoped)
	return errors.Join(scoped.err, callbackErr)
}

func (w *tenantReader) allows(tenantID string) bool {
	if tenantID != w.id {
		w.err = errTenantScope
	}
	return w.err == nil
}

func (w *tenantReader) UserByID(id string) (core.User, bool) {
	if w.err != nil {
		return core.User{}, false
	}
	if sql, ok := w.store.(*sqlStore); ok {
		return readJSON[core.User](sql, "SELECT data FROM platform_users WHERE tenant_id = $1 AND id = $2", w.id, id)
	}
	user, ok := w.store.UserByID(id)
	if !ok || user.TenantID != w.id {
		return core.User{}, false
	}
	return user, true
}

func (w *tenantReader) Users(tenantID string) []core.User {
	if !w.allows(tenantID) {
		return nil
	}
	return w.store.Users(tenantID)
}

func (w *tenantReader) Record(tenantID, id, kind string) (core.Record, bool) {
	if !w.allows(tenantID) {
		return core.Record{}, false
	}
	return w.store.Record(tenantID, id, kind)
}

func (w *tenantReader) Records(tenantID, kind string) []core.Record {
	if !w.allows(tenantID) {
		return nil
	}
	return w.store.Records(tenantID, kind)
}

func (w *tenantReader) ConversationMessages(tenantID, id string) []core.Record {
	if !w.allows(tenantID) {
		return nil
	}
	return w.store.ConversationMessages(tenantID, id)
}

func (w *tenantReader) IdleTickets(tenantID string, cutoff time.Time) []core.Record {
	if !w.allows(tenantID) {
		return nil
	}
	return w.store.IdleTickets(tenantID, cutoff)
}

func (w *tenantWriter) PutRecord(record core.Record) {
	if !w.allows(record.TenantID) {
		return
	}
	// The on-disk JSON format keys records by ID. Protect an existing
	// record belonging to another tenant from a same-ID write.
	if file, ok := w.store.(snapshot); ok {
		if previous, found := file.state.Records[record.ID]; found && previous.TenantID != w.id {
			w.err = errTenantScope
			return
		}
	}
	w.store.PutRecord(record)
}

func (w *tenantWriter) LockWorkflowJob(tenantID, id string) (core.Record, bool) {
	if !w.allows(tenantID) {
		return core.Record{}, false
	}
	return w.store.LockWorkflowJob(tenantID, id)
}

func (w *tenantWriter) Savepoint(run func(core.Writer) error) error {
	if w.err != nil {
		return w.err
	}
	return w.store.Savepoint(func(store core.Writer) error { return withTenantWriter(store, w.id, run) })
}

func (w *tenantReader) Tenants() []string {
	w.err = errTenantScope
	return nil
}

func (w *tenantReader) PendingWorkflowJobs(int) []core.Record {
	w.err = errTenantScope
	return nil
}

func (w *tenantReader) DueEmailReplies(time.Time, int) []core.Record {
	w.err = errTenantScope
	return nil
}

func (w *tenantReader) UserByEmail(string) (core.User, bool) {
	w.err = errTenantScope
	return core.User{}, false
}

func (w *tenantReader) Session(string) (core.Session, bool) {
	w.err = errTenantScope
	return core.Session{}, false
}

func (w *tenantReader) Sessions(string) map[string]core.Session {
	w.err = errTenantScope
	return nil
}

func (w *tenantWriter) PutUser(core.User)               { w.err = errTenantScope }
func (w *tenantWriter) PutSession(string, core.Session) { w.err = errTenantScope }
func (w *tenantWriter) DeleteSession(string)            { w.err = errTenantScope }
func (w *tenantWriter) RevokeSessions(string)           { w.err = errTenantScope }
func (w *tenantWriter) ExpireSessions(time.Time)        { w.err = errTenantScope }
