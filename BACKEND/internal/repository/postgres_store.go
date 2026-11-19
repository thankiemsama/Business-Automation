package repository

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"businessautomation/internal/core"
)

// Reader and Writer have no error results. Keep the first SQL error so a callback
// can never commit on incomplete reads or failed writes.
type sqlStore struct {
	tx  *sql.Tx
	ctx context.Context
	err error
}

func (s *sqlStore) exec(query string, args ...any) {
	if s.err == nil {
		_, s.err = s.tx.ExecContext(s.ctx, query, args...)
	}
}

func scanOne[T any](s *sqlStore, query string, args ...any) (T, bool) {
	var value T
	if s.err != nil {
		return value, false
	}
	err := s.tx.QueryRowContext(s.ctx, query, args...).Scan(&value)
	if !errors.Is(err, sql.ErrNoRows) {
		s.err = err
	}
	return value, err == nil
}

func readJSON[T any](s *sqlStore, query string, args ...any) (T, bool) {
	raw, found := scanOne[[]byte](s, query, args...)
	var value T
	if found {
		s.err = json.Unmarshal(raw, &value)
	}
	return value, found && s.err == nil
}

func scanRows(s *sqlStore, read func(*sql.Rows) error, query string, args ...any) {
	if s.err != nil {
		return
	}
	rows, err := s.tx.QueryContext(s.ctx, query, args...)
	if err != nil {
		s.err = err
		return
	}
	defer rows.Close()
	for rows.Next() {
		if s.err = read(rows); s.err != nil {
			return
		}
	}
	s.err = rows.Err()
}

func readList[T any](s *sqlStore, query string, args ...any) []T {
	result := []T{}
	scanRows(s, func(rows *sql.Rows) error {
		var value T
		if err := decodeRow(rows, &value); err != nil {
			return err
		}
		result = append(result, value)
		return nil
	}, query, args...)
	return result
}

func decodeRow(rows *sql.Rows, value any) error {
	var raw []byte
	if err := rows.Scan(&raw); err != nil {
		return err
	}
	return json.Unmarshal(raw, value)
}

func encodeJSON(s *sqlStore, value any) string {
	if s.err != nil {
		return ""
	}
	raw, err := json.Marshal(value)
	s.err = err
	return string(raw)
}

func (s *sqlStore) UserByEmail(email string) (core.User, bool) {
	return readJSON[core.User](s, "SELECT data FROM platform_users WHERE email = $1 LIMIT 1", email)
}

func (s *sqlStore) UserByID(id string) (core.User, bool) {
	return readJSON[core.User](s, "SELECT data FROM platform_users WHERE id = $1", id)
}

func (s *sqlStore) Tenants() []string {
	return readList[string](s, "SELECT to_json(tenant_id) FROM (SELECT DISTINCT tenant_id FROM platform_users) AS tenants")
}

func (s *sqlStore) Users(tenantID string) []core.User {
	return readList[core.User](s, "SELECT data FROM platform_users WHERE tenant_id = $1", tenantID)
}

func (s *sqlStore) Session(id string) (core.Session, bool) {
	return readJSON[core.Session](s, "SELECT data FROM platform_sessions WHERE id = $1", id)
}

func (s *sqlStore) Sessions(userID string) map[string]core.Session {
	result := map[string]core.Session{}
	scanRows(s, func(rows *sql.Rows) error {
		var id string
		var raw []byte
		var session core.Session
		if err := rows.Scan(&id, &raw); err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &session); err != nil {
			return err
		}
		result[id] = session
		return nil
	}, "SELECT id, data FROM platform_sessions WHERE user_id = $1", userID)
	return result
}

func (s *sqlStore) Record(tenantID, id, kind string) (core.Record, bool) {
	return readJSON[core.Record](s, "SELECT data FROM platform_records WHERE id = $1 AND tenant_id = $2 AND kind = $3", id, tenantID, kind)
}

func (s *sqlStore) Records(tenantID, kind string) []core.Record {
	if kind == "" {
		return readList[core.Record](s, "SELECT data FROM platform_records WHERE tenant_id = $1", tenantID)
	}
	return readList[core.Record](s, "SELECT data FROM platform_records WHERE tenant_id = $1 AND kind = $2", tenantID, kind)
}

func (s *sqlStore) DueEmailReplies(now time.Time, limit int) []core.Record {
	return readList[core.Record](s, `SELECT data FROM platform_records
		WHERE pending_email_at <= $1 ORDER BY pending_email_at, id LIMIT $2`, now, limit)
}

func (s *sqlStore) ConversationMessages(tenantID, conversationID string) []core.Record {
	return readList[core.Record](s, `SELECT data FROM platform_records
		WHERE tenant_id = $1 AND kind = 'messages' AND data->>'conversation_id' = $2`, tenantID, conversationID)
}

func (s *sqlStore) PutUser(user core.User) {
	s.exec(`INSERT INTO platform_users (id, tenant_id, email, data) VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET tenant_id = EXCLUDED.tenant_id, email = EXCLUDED.email, data = EXCLUDED.data`,
		user.ID, user.TenantID, user.Email, encodeJSON(s, user))
}

func (s *sqlStore) PutSession(id string, session core.Session) {
	s.exec(`INSERT INTO platform_sessions (id, user_id, expires_at, data) VALUES ($1, $2, $3, $4)
		ON CONFLICT (id) DO UPDATE SET user_id = EXCLUDED.user_id, expires_at = EXCLUDED.expires_at, data = EXCLUDED.data`,
		id, session.UserID, session.ExpiresAt, encodeJSON(s, session))
}

func (s *sqlStore) DeleteSession(id string) {
	s.exec("DELETE FROM platform_sessions WHERE id = $1", id)
}

func (s *sqlStore) RevokeSessions(userID string) {
	s.exec("DELETE FROM platform_sessions WHERE user_id = $1", userID)
}

func (s *sqlStore) ExpireSessions(now time.Time) {
	s.exec("DELETE FROM platform_sessions WHERE expires_at <= $1", now)
}

func pendingEmailAt(record core.Record) any {
	if record.Kind != core.KindMessages || record.Status != "pending" || record.Sender != core.SenderAgent || record.Email == "" {
		return nil
	}
	if record.EmailRetryAt != nil {
		return *record.EmailRetryAt
	}
	return record.CreatedAt
}

func (s *sqlStore) PutRecord(record core.Record) {
	versioned := core.VersionedResource(record.Kind)
	if versioned {
		record.Version = 1
	}
	s.exec(`INSERT INTO platform_records (id, tenant_id, kind, pending_email_at, data, workflow_activity_at, pending_workflow_at)
		VALUES ($1, $2, $3, $4, $5, $7, $8)
		ON CONFLICT (tenant_id, id) DO UPDATE SET kind = EXCLUDED.kind,
			pending_email_at = EXCLUDED.pending_email_at,
			workflow_activity_at = EXCLUDED.workflow_activity_at,
			pending_workflow_at = EXCLUDED.pending_workflow_at,
			data = CASE WHEN $6 THEN jsonb_set(EXCLUDED.data, '{version}',
				to_jsonb(COALESCE((platform_records.data->>'version')::bigint, 0) + 1))
				ELSE EXCLUDED.data END`,
		record.ID, record.TenantID, record.Kind, pendingEmailAt(record), encodeJSON(s, record), versioned, workflowActivityAt(record), pendingWorkflowAt(record))
}

func (s *sqlStore) IdleTickets(tenantID string, cutoff time.Time) []core.Record {
	return readList[core.Record](s, `SELECT ticket.data FROM platform_records AS ticket
		LEFT JOIN platform_records AS conversation
		  ON conversation.id = ticket.data->>'conversation_id'
		 AND conversation.tenant_id = ticket.tenant_id AND conversation.kind = 'conversations'
		WHERE ticket.tenant_id = $1 AND ticket.kind = 'tickets'
		  AND ticket.workflow_activity_at <= $2
		  AND (conversation.workflow_activity_at IS NULL OR conversation.workflow_activity_at <= $2)
		ORDER BY ticket.workflow_activity_at, ticket.id`, tenantID, cutoff)
}

func (s *sqlStore) PendingWorkflowJobs(limit int) []core.Record {
	if limit <= 0 {
		return nil
	}
	return readList[core.Record](s, `SELECT job.data FROM platform_records AS job
		WHERE job.pending_workflow_at IS NOT NULL
		  AND EXISTS (SELECT 1 FROM platform_users WHERE tenant_id = job.tenant_id)
		ORDER BY job.pending_workflow_at, job.id, job.tenant_id
		LIMIT $1`, limit)
}

func (s *sqlStore) LockWorkflowJob(tenantID, id string) (core.Record, bool) {
	return readJSON[core.Record](s, `SELECT data FROM platform_records
		WHERE tenant_id = $1 AND id = $2 AND pending_workflow_at IS NOT NULL
		FOR UPDATE SKIP LOCKED`, tenantID, id)
}

func (s *sqlStore) Savepoint(run func(core.Writer) error) (result error) {
	s.exec("SAVEPOINT platform_uow")
	if s.err != nil {
		return s.err
	}
	success := false
	defer func() {
		if !success {
			s.err = nil
			s.exec("ROLLBACK TO SAVEPOINT platform_uow")
		}
		s.exec("RELEASE SAVEPOINT platform_uow")
		result = errors.Join(result, s.err)
	}()
	callbackErr := run(s)
	result = errors.Join(callbackErr, s.err)
	success = result == nil
	return result
}
