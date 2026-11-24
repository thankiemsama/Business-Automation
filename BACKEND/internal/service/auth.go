package service

import (
	"fmt"
	"log/slog"
	"net/mail"
	"os"
	"strings"
	"time"

	"businessautomation/internal/core"
)

const (
	maximumEmailBytes    = 254
	maximumUserNameBytes = 200
	minimumPasswordBytes = 12
	maximumPasswordBytes = 128
)

type AuthService struct {
	repo   core.Repository
	clock  Clock
	ids    IDGenerator
	hasher PasswordHasher
	mailer *EmailMailer
	origin string
}

func NewAuth(repo core.Repository, clock Clock, ids IDGenerator, hasher PasswordHasher) *AuthService {
	return &AuthService{repo: repo, clock: clock, ids: ids, hasher: hasher}
}

const (
	// maximumLoginFailures is the consecutive-failure streak after which an
	// account is locked for a growing window (brute-force mitigation, G2).
	maximumLoginFailures = 5
	initialLockout       = 5 * time.Minute
	maximumLockout       = time.Hour
)

// lockoutBackoff grows the lockout window per failing streak:
// 5 failures -> 5m, 6 -> 10m, 7 -> 20m, 8 -> 40m, 9+ -> 1h.
func lockoutBackoff(failures int) time.Duration {
	d := initialLockout
	for i := maximumLoginFailures; i < failures; i++ {
		d *= 2
		if d > maximumLockout {
			return maximumLockout
		}
	}
	return d
}

func normalizeCredentials(credentials Credentials) (Credentials, error) {
	credentials.Email = strings.ToLower(strings.TrimSpace(credentials.Email))
	credentials.Name = strings.TrimSpace(credentials.Name)
	address, err := mail.ParseAddress(credentials.Email)
	if err != nil || address.Address != credentials.Email || len(credentials.Email) > maximumEmailBytes {
		return credentials, core.Fail(400, "Email không hợp lệ")
	}
	if credentials.Name == "" || len(credentials.Name) > maximumUserNameBytes || len(credentials.Password) < minimumPasswordBytes || len(credentials.Password) > maximumPasswordBytes {
		return credentials, core.Fail(400, "Tên bắt buộc; mật khẩu từ 12 đến 128 ký tự")
	}
	return credentials, nil
}

func (s *AuthService) createUser(credentials Credentials, tenantID, role string, verifyEmail bool) (core.User, error) {
	credentials, err := normalizeCredentials(credentials)
	if err != nil {
		return core.User{}, err
	}
	// Self-serve signups are held in pending-verification while a mail
	// provider is available; owner-created agents are vouched for (G1).
	// Two skips keep scripted/dev flows frictionless:
	//   - NEXA_AUTO_CONFIRM_EMAIL=true (dev/seed only) bypasses the hold;
	//   - the dev sink only writes mail to the backend log, so there is no
	//     real channel for a user to click through — auto-confirm instead.
	pendingEmail := verifyEmail && s.mailer != nil && s.origin != "" &&
		!s.mailer.IsDevSink() &&
		os.Getenv("NEXA_AUTO_CONFIRM_EMAIL") != "true"
	var starter industryStarter
	if tenantID == "" {
		if credentials.Industry != "" {
			var ok bool
			starter, ok = industryStarters[credentials.Industry]
			if !ok {
				return core.User{}, core.Fail(400, "Mẫu ngành không hợp lệ")
			}
		}
		tenantID = s.ids.NewID()
	}
	user := core.User{
		ID:                       s.ids.NewID(),
		TenantID:                 tenantID,
		Name:                     credentials.Name,
		Email:                    credentials.Email,
		Role:                     role,
		Salt:                     s.ids.NewID(),
		EmailVerificationPending: pendingEmail,
	}
	user.PasswordHash = s.hasher.Hash(credentials.Password, user.Salt)
	err = s.repo.Update(func(store core.Writer) error {
		if _, exists := store.UserByEmail(credentials.Email); exists {
			return core.Fail(409, "Email đã tồn tại")
		}
		store.PutUser(user)
		if starter.subject != "" {
			return s.seedIndustryStarter(store, user, starter)
		}
		return nil
	})
	return user, err
}

func (s *AuthService) Register(credentials Credentials) (core.User, error) {
	user, err := s.createUser(credentials, "", core.RoleOwner, true)
	if err != nil {
		return core.User{}, err
	}
	if user.EmailVerificationPending {
		// A delivery failure must not block registration: the owner can
		// resend the link from the pending-verification UI.
		if err := s.SendVerification(user.Email); err != nil {
			slog.Error("Không gửi được email xác nhận", "error", err)
		}
	}
	return user, nil
}

func (s *AuthService) AddAgent(owner core.User, credentials Credentials) (core.User, error) {
	if owner.Role != core.RoleOwner {
		return core.User{},
			core.Fail(403, "Chỉ owner được tạo agent")
	}
	return s.createUser(credentials, owner.TenantID, core.RoleAgent, false)
}

func (s *AuthService) Login(credentials Credentials) (string, error) {
	now := s.clock.Now()
	var user core.User
	if err := s.repo.View(func(store core.Reader) error {
		user, _ = store.UserByEmail(strings.ToLower(strings.TrimSpace(credentials.Email)))
		return nil
	}); err != nil {
		return "", err
	}
	if len(credentials.Password) > maximumPasswordBytes {
		return "", s.recordLoginFailure(user, credentials.IP, "password_too_long")
	}
	// G2: a failing streak locks the account for a growing window.
	if user.LockedUntil.After(now) {
		s.recordAuthEvent(user, "login", credentials.IP, "locked")
		return "", core.FailReason(401, "account_locked", fmt.Sprintf("Tài khoản tạm khóa do quá nhiều lần đăng nhập sai. Thử lại sau %s.", lockoutBackoff(user.LoginFailures).Round(time.Minute)))
	}
	salt := user.Salt
	if salt == "" {
		salt = "dummy-login-salt"
	}
	// verifyStoredHash upgrades legacy 210k-iteration hashes transparently (G6).
	match, upgradedHash := verifyStoredHash(s.hasher, credentials.Password, salt, user.PasswordHash)
	if user.ID == "" {
		// Unknown mailboxes still pay one full PBKDF2 run so a wrong login
		// never answers faster than a right one (anti-timing-enumeration).
		_ = s.hasher.Hash(credentials.Password, salt)
	}
	if !match || user.Disabled {
		return "", s.recordLoginFailure(user, credentials.IP, "credentials")
	}
	if user.EmailVerificationPending {
		s.recordAuthEvent(user, "login", credentials.IP, "email_unverified")
		return "", core.FailReason(403, "email_unverified", "Xác nhận email trước khi đăng nhập. Hệ thống sẽ gửi lại mã xác nhận.")
	}
	token := s.ids.NewID()
	err := s.repo.Update(func(store core.Writer) error {
		current, ok := store.UserByID(user.ID)
		if !ok || current.Disabled || current.PasswordHash != user.PasswordHash || current.LockedUntil.After(now) {
			return core.Fail(401, "Thông tin đăng nhập không đúng")
		}
		current.PasswordHash = upgradedHash
		if current.LoginFailures > 0 {
			current.LoginFailures, current.LockedUntil = 0, time.Time{}
		}
		if current.TOTPSecret != "" {
			if credentials.Code == "" {
				s.putAuthEvent(store, current, credentials.IP, "login", "needs_mfa")
				return core.Fail(409, "Cần mã xác thực hai lớp")
			}
			if err := s.useSecondFactor(&current, credentials.Code); err != nil {
				s.putAuthEvent(store, current, credentials.IP, "login", "mfa_invalid")
				return err
			}
		}
		store.PutUser(current)
		s.putAuthEvent(store, current, credentials.IP, "login", "ok")
		store.ExpireSessions(s.clock.Now())
		store.PutSession(tokenHash(token), core.Session{UserID: current.ID, CreatedAt: s.clock.Now(), ExpiresAt: s.clock.Now().Add(core.SessionLifetime)})
		return nil
	})
	return token, err
}

// recordLoginFailure persists the brute-force streak (G2): every failure grows
// the lockout window; the 5th consecutive failure locks for 5 minutes.
func (s *AuthService) recordLoginFailure(user core.User, clientIP, reason string) error {
	if user.ID == "" {
		// Unknown emails stay PII-free: nothing is persisted.
		s.recordAuthEvent(user, "login", clientIP, "failed:"+reason)
		return core.Fail(401, "Thông tin đăng nhập không đúng")
	}
	err := s.repo.Update(func(store core.Writer) error {
		current, ok := store.UserByID(user.ID)
		if !ok {
			return nil
		}
		current.LoginFailures++
		if current.LoginFailures >= maximumLoginFailures {
			current.LockedUntil = s.clock.Now().Add(lockoutBackoff(current.LoginFailures))
		}
		store.PutUser(current)
		s.putAuthEvent(store, current, clientIP, "login", "failed:"+reason)
		return nil
	})
	if err != nil {
		return err
	}
	return core.Fail(401, "Thông tin đăng nhập không đúng")
}

// recordAuthEvent writes an auth trail row (G4) in its own transaction.
func (s *AuthService) recordAuthEvent(user core.User, event, clientIP, outcome string) {
	if user.TenantID == "" {
		return
	}
	_ = s.repo.Update(func(store core.Writer) error {
		s.putAuthEvent(store, user, clientIP, event, outcome)
		return nil
	})
}

// putAuthEvent writes a PII-fingerprinted auth_event record (audited by the
// workspace writer; raw email/IP stay in the row, audit diffs use fingerprints).
func (s *AuthService) putAuthEvent(store core.Writer, user core.User, clientIP, event, outcome string) {
	if user.TenantID == "" {
		return
	}
	now := s.clock.Now().UTC()
	store.PutRecord(core.Record{ID: s.ids.NewID(), TenantID: user.TenantID, Kind: core.KindAuthEvent, Name: clientIP, Email: user.Email, AuthorID: user.ID, Status: event + ":" + outcome, CreatedAt: now, UpdatedAt: now})
}

func (s *AuthService) Authenticate(token string) (core.User, error) {
	var user core.User
	err := s.repo.View(func(store core.Reader) error {
		session, ok := store.Session(tokenHash(token))
		if !ok || !session.ExpiresAt.After(s.clock.Now()) {
			return core.Fail(401, "Phiên đăng nhập không hợp lệ")
		}
		user, ok = store.UserByID(session.UserID)
		if !ok || user.Disabled {
			return core.Fail(401, "Tài khoản không tồn tại")
		}
		return nil
	})
	return user, err
}

func (s *AuthService) Logout(token string) error {
	return s.repo.Update(func(store core.Writer) error { store.DeleteSession(tokenHash(token)); return nil })
}
