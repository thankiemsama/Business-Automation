package service

import (
	"log"
	"net/url"
	"strings"
	"time"

	"businessautomation/internal/core"
)

func (s *AuthService) ConfigureEmail(mailer *EmailMailer, origin string) {
	s.mailer, s.origin = mailer, strings.TrimRight(origin, "/")
}

func (s *AuthService) ChangePassword(user core.User, old, next string) error {
	if len(old) > maximumPasswordBytes || len(next) < minimumPasswordBytes || len(next) > maximumPasswordBytes {
		return core.Fail(400, "Mật khẩu mới phải từ 12 đến 128 ký tự")
	}
	if ok, _ := verifyStoredHash(s.hasher, old, user.Salt, user.PasswordHash); !ok {
		return core.Fail(400, "Mật khẩu hiện tại không đúng")
	}
	salt := s.ids.NewID()
	hash := s.hasher.Hash(next, salt)
	return s.repo.Update(func(store core.Writer) error {
		current, ok := store.UserByID(user.ID)
		if !ok || current.Disabled || current.PasswordHash != user.PasswordHash {
			return core.Fail(409, "Thông tin tài khoản đã thay đổi")
		}
		current.Salt, current.PasswordHash = salt, hash
		current.LoginFailures, current.LockedUntil = 0, time.Time{}
		store.PutUser(current)
		store.RevokeSessions(user.ID)
		return nil
	})
}

const accountLinkKind = "account_links"

func (s *AuthService) issueLink(store core.Writer, tenant, subject, email, name, userID string, lifetime time.Duration) string {
	token := s.ids.NewID()
	now := s.clock.Now().UTC()
	store.PutRecord(core.Record{ID: tokenHash(token), TenantID: tenant, Kind: accountLinkKind, Subject: subject, Email: email, Name: name, AuthorID: userID, Enabled: true, CreatedAt: now, UpdatedAt: now.Add(lifetime)})
	return token
}

func (s *AuthService) SendReset(email string) error {
	if s.mailer == nil || s.origin == "" {
		return core.Fail(503, "Chưa cấu hình email khôi phục")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	var token, tenant string
	err := s.repo.Update(func(store core.Writer) error {
		user, ok := store.UserByEmail(email)
		if !ok || user.Disabled {
			return nil
		}
		tenant = user.TenantID
		token = s.issueLink(store, tenant, "reset", user.Email, "", user.ID, 30*time.Minute)
		return nil
	})
	if err != nil {
		return err
	}
	if token != "" {
		if err := s.sendLink(email, tenant, token, "Đặt lại mật khẩu Nexa", "/account/setup"); err != nil {
			log.Print("Không gửi được email khôi phục")
		}
	}
	return nil
}

func (s *AuthService) Invite(owner core.User, name, email string) error {
	if owner.Role != core.RoleOwner {
		return core.Fail(403, "Chỉ owner được mời nhân viên")
	}
	if s.mailer == nil || s.origin == "" {
		return core.Fail(503, "Chưa cấu hình email mời")
	}
	credentials, err := normalizeCredentials(Credentials{Name: name, Email: email, Password: strings.Repeat("x", minimumPasswordBytes)})
	if err != nil {
		return err
	}
	var token string
	err = s.repo.Update(func(store core.Writer) error {
		if _, exists := store.UserByEmail(credentials.Email); exists {
			return core.Fail(409, "Email đã tồn tại")
		}
		token = s.issueLink(store, owner.TenantID, "invite", credentials.Email, credentials.Name, "", 48*time.Hour)
		return nil
	})
	if err != nil {
		return err
	}
	return s.sendLink(credentials.Email, owner.TenantID, token, "Lời mời tham gia Nexa", "/account/setup")
}

func (s *AuthService) sendLink(email, tenant, token, title, path string) error {
	link := s.origin + path + "#" + url.PathEscape(tenant+"."+token)
	if err := s.mailer.Send(email, title, "Mở liên kết để tiếp tục:\r\n"+link, tokenHash(token), "", ""); err != nil {
		return core.Fail(502, "Không gửi được email")
	}
	return nil
}

// SendVerification issues (or re-issues) the 30-minute email verification
// link for a freshly registered account and mails it (G1).
func (s *AuthService) SendVerification(email string) error {
	if s.mailer == nil || s.origin == "" {
		return core.Fail(503, "Chưa cấu hình email xác nhận")
	}
	email = strings.ToLower(strings.TrimSpace(email))
	var token, tenant string
	err := s.repo.Update(func(store core.Writer) error {
		user, ok := store.UserByEmail(email)
		if !ok || user.Disabled || !user.EmailVerificationPending {
			return core.Fail(404, "Không có tài khoản chờ xác nhận")
		}
		tenant = user.TenantID
		// Re-issue: disable older pending verify links for this user first.
		for _, pending := range store.Records(tenant, accountLinkKind) {
			if pending.Subject == "verify" && pending.AuthorID == user.ID && pending.Enabled {
				pending.Enabled = false
				store.PutRecord(pending)
			}
		}
		token = s.issueLink(store, tenant, "verify", user.Email, "", user.ID, 30*time.Minute)
		return nil
	})
	if err != nil {
		return err
	}
	if token == "" {
		return core.Fail(404, "Không có tài khoản chờ xác nhận")
	}
	if err := s.sendLink(email, tenant, token, "Xác nhận email Nexa", "/account/verify"); err != nil {
		log.Print("Không gửi được email xác nhận")
	}
	return nil
}

// ConfirmEmail validates a verification link and marks the mailbox confirmed.
// Reset/invite links are rejected here: those keep their own redeem path.
func (s *AuthService) ConfirmEmail(token string) error {
	tenant, secret, valid := strings.Cut(token, ".")
	invalid := core.Fail(400, "Liên kết hết hạn hoặc không hợp lệ")
	if !valid || tenant == "" || secret == "" {
		return invalid
	}
	key := tokenHash(secret)
	var link core.Record
	if err := s.repo.View(func(store core.Reader) error {
		var ok bool
		link, ok = store.Record(tenant, key, accountLinkKind)
		if !ok || !link.Enabled || link.Subject != "verify" || !link.UpdatedAt.After(s.clock.Now()) {
			return invalid
		}
		return nil
	}); err != nil {
		return err
	}
	return s.repo.Update(func(store core.Writer) error {
		current, ok := store.UserByID(link.AuthorID)
		if !ok || current.TenantID != tenant || current.Email != link.Email {
			return invalid
		}
		current.EmailVerificationPending = false
		store.PutUser(current)
		for _, pending := range store.Records(tenant, accountLinkKind) {
			if pending.Subject == "verify" && pending.AuthorID == current.ID && pending.Enabled {
				pending.Enabled = false
				store.PutRecord(pending)
			}
		}
		s.putAuthEvent(store, current, "", "verify", "ok")
		return nil
	})
}

// IssueStepUp verifies the current password and returns a single-use token
// that authorizes sensitive operations (export/backup) for 5 minutes (G8).
func (s *AuthService) IssueStepUp(user core.User, password string) (string, error) {
	if len(password) == 0 || len(password) > maximumPasswordBytes {
		return "", core.Fail(403, "Mật khẩu hiện tại không đúng")
	}
	if ok, _ := verifyStoredHash(s.hasher, password, user.Salt, user.PasswordHash); !ok {
		s.recordAuthEvent(user, "step_up", "", "invalid")
		return "", core.Fail(403, "Mật khẩu hiện tại không đúng")
	}
	token := s.ids.NewID()
	now := s.clock.Now()
	err := s.repo.Update(func(store core.Writer) error {
		store.PutRecord(core.Record{ID: tokenHash("step-up:" + user.ID + ":" + token), TenantID: user.TenantID, Kind: core.KindStepUp, AuthorID: user.ID, Enabled: true, CreatedAt: now, UpdatedAt: now.Add(5 * time.Minute)})
		s.putAuthEvent(store, user, "", "step_up", "ok")
		return nil
	})
	if err != nil {
		return "", err
	}
	return token, nil
}

// ConsumeStepUp validates and revokes a step-up token (single use, 5 minutes).
func (s *AuthService) ConsumeStepUp(user core.User, token string) error {
	id := tokenHash("step-up:" + user.ID + ":" + token)
	return s.repo.Update(func(store core.Writer) error {
		record, ok := store.Record(user.TenantID, id, core.KindStepUp)
		if !ok || !record.Enabled || !record.UpdatedAt.After(s.clock.Now()) {
			s.putAuthEvent(store, user, "", "step_up", "denied")
			return core.FailReason(403, "step_up_expired", "Phiên xác nhận mật khẩu không hợp lệ hoặc đã hết hạn.")
		}
		record.Enabled = false
		store.PutRecord(record)
		return nil
	})
}

func (s *AuthService) Redeem(token, password string) error {
	if len(password) < minimumPasswordBytes || len(password) > maximumPasswordBytes {
		return core.Fail(400, "Mật khẩu phải từ 12 đến 128 ký tự")
	}
	tenant, secret, valid := strings.Cut(token, ".")
	invalid := core.Fail(400, "Liên kết hết hạn hoặc không hợp lệ")
	if !valid || tenant == "" || secret == "" {
		return invalid
	}
	key := tokenHash(secret)
	var link core.Record
	if err := s.repo.View(func(store core.Reader) error {
		var ok bool
		link, ok = store.Record(tenant, key, accountLinkKind)
		if !ok || !link.Enabled || !link.UpdatedAt.After(s.clock.Now()) {
			return invalid
		}
		return nil
	}); err != nil {
		return err
	}
	salt := s.ids.NewID()
	hash := s.hasher.Hash(password, salt)
	return s.repo.Update(func(store core.Writer) error {
		link, ok := store.Record(tenant, key, accountLinkKind)
		if !ok || !link.Enabled || !link.UpdatedAt.After(s.clock.Now()) {
			return invalid
		}
		if link.Subject == "verify" {
			return core.Fail(400, "Liên kết không hợp lệ")
		}
		var user core.User
		if link.Subject == "invite" {
			if _, exists := store.UserByEmail(link.Email); exists {
				return invalid
			}
			user = core.User{ID: s.ids.NewID(), TenantID: tenant, Name: link.Name, Email: link.Email, Role: core.RoleAgent}
		} else {
			user, ok = store.UserByID(link.AuthorID)
			if !ok || user.TenantID != tenant || user.Disabled || user.Email != link.Email {
				return invalid
			}
		}
		user.Salt, user.PasswordHash = salt, hash
		store.PutUser(user)
		store.RevokeSessions(user.ID)
		for _, pending := range store.Records(tenant, accountLinkKind) {
			if pending.Email == link.Email && pending.Enabled {
				pending.Enabled = false
				store.PutRecord(pending)
			}
		}
		return nil
	})
}
