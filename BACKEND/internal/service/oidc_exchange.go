package service

import (
	"encoding/json"
	"os"
	"time"

	"businessautomation/internal/core"
)

func (s *AuthService) IssueOIDC(issuer, subject string) (string, error) {
	targets := map[string]string{}
	if json.Unmarshal([]byte(os.Getenv("NEXA_OIDC_BINDINGS")), &targets) != nil {
		return "", core.Fail(403, "Tài khoản SSO chưa được liên kết")
	}
	email := targets[issuer+"|"+subject]
	if email == "" {
		return "", core.Fail(403, "Tài khoản SSO chưa được liên kết")
	}
	token := s.ids.NewID()
	err := s.repo.Update(func(store core.Writer) error {
		user, ok := store.UserByEmail(email)
		if !ok || user.Disabled {
			return core.Fail(403, "Tài khoản SSO chưa được liên kết")
		}
		now := s.clock.Now()
		store.PutRecord(core.Record{ID: tokenHash("oidc:" + token), TenantID: user.TenantID, Kind: "oidc_exchange", AuthorID: user.ID, Email: user.Email, Enabled: true, CreatedAt: now, UpdatedAt: now.Add(2 * time.Minute)})
		return nil
	})
	return token, err
}
func (s *AuthService) ExchangeOIDC(ticket, code string) (string, error) {
	if len(ticket) != 48 {
		return "", core.Fail(401, "Phiên SSO không hợp lệ")
	}
	session := s.ids.NewID()
	denied := core.Fail(401, "Phiên SSO không hợp lệ hoặc hết hạn")
	err := s.repo.Update(func(store core.Writer) error {
		now := s.clock.Now()
		id := tokenHash("oidc:" + ticket)
		for _, tenant := range store.Tenants() {
			pending, ok := store.Record(tenant, id, "oidc_exchange")
			if !ok {
				continue
			}
			user, exists := store.UserByID(pending.AuthorID)
			if !exists || user.Disabled || !pending.Enabled || !pending.UpdatedAt.After(now) || user.TenantID != tenant || user.Email != pending.Email {
				return denied
			}
			if user.TOTPSecret != "" {
				if code == "" {
					return core.Fail(409, "Cần mã xác thực hai lớp")
				}
				if err := s.useSecondFactor(&user, code); err != nil {
					return err
				}
				store.PutUser(user)
			}
			pending.Enabled = false
			store.PutRecord(pending)
			store.ExpireSessions(now)
			store.PutSession(tokenHash(session), core.Session{UserID: user.ID, CreatedAt: now, ExpiresAt: now.Add(core.SessionLifetime)})
			return nil
		}
		return denied
	})
	return session, err
}
