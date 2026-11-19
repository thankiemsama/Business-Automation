package service

import (
	"crypto/subtle"
	"net/mail"
	"strings"

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

func (s *AuthService) createUser(credentials Credentials, tenantID, role string) (core.User, error) {
	credentials, err := normalizeCredentials(credentials)
	if err != nil {
		return core.User{}, err
	}
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
		ID:       s.ids.NewID(),
		TenantID: tenantID,
		Name:     credentials.Name,
		Email:    credentials.Email,
		Role:     role,
		Salt:     s.ids.NewID(),
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
	return s.createUser(credentials, "", core.RoleOwner)
}

func (s *AuthService) AddAgent(owner core.User, credentials Credentials) (core.User, error) {
	if owner.Role != core.RoleOwner {
		return core.User{},
			core.Fail(403, "Chỉ owner được tạo agent")
	}
	return s.createUser(credentials, owner.TenantID, core.RoleAgent)
}

func (s *AuthService) Login(credentials Credentials) (string, error) {
	var user core.User
	if err := s.repo.View(func(store core.Reader) error {
		user, _ = store.UserByEmail(strings.ToLower(strings.TrimSpace(credentials.Email)))
		return nil
	}); err != nil {
		return "", err
	}
	if len(credentials.Password) > maximumPasswordBytes {
		return "", core.Fail(401, "Thông tin đăng nhập không đúng")
	}
	salt := user.Salt
	if salt == "" {
		salt = "dummy-login-salt"
	}
	hash := s.hasher.Hash(credentials.Password, salt)
	if subtle.ConstantTimeCompare([]byte(hash), []byte(user.PasswordHash)) != 1 || user.Disabled {
		return "", core.Fail(401, "Thông tin đăng nhập không đúng")
	}
	token := s.ids.NewID()
	err := s.repo.Update(func(store core.Writer) error {
		current, ok := store.UserByID(user.ID)
		if !ok || current.Disabled || current.PasswordHash != user.PasswordHash {
			return core.Fail(401, "Thông tin đăng nhập không đúng")
		}
		if current.TOTPSecret != "" {
			if credentials.Code == "" {
				return core.Fail(409, "Cần mã xác thực hai lớp")
			}
			if err := s.useSecondFactor(&current, credentials.Code); err != nil {
				return err
			}
			store.PutUser(current)
		}
		store.ExpireSessions(s.clock.Now())
		store.PutSession(tokenHash(token), core.Session{UserID: user.ID, CreatedAt: s.clock.Now(), ExpiresAt: s.clock.Now().Add(core.SessionLifetime)})
		return nil
	})
	return token, err
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
