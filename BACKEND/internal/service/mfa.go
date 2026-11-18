package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"time"

	"businessautomation/internal/core"
)

func securityCipher() (cipher.AEAD, error) {
	key, err := base64.StdEncoding.DecodeString(os.Getenv("NEXA_SECURITY_KEY"))
	if err != nil || len(key) != 32 {
		return nil, core.Fail(503, "Chưa cấu hình NEXA_SECURITY_KEY")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
func protectSecret(clear, scope string) (string, error) {
	gcm, err := securityCipher()
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(clear), []byte(scope))), nil
}
func readSecret(sealed, scope string) (string, error) {
	gcm, err := securityCipher()
	if err != nil {
		return "", err
	}
	blob, err := base64.RawURLEncoding.DecodeString(sealed)
	if err != nil || len(blob) < gcm.NonceSize() {
		return "", core.Fail(503, "Dữ liệu bảo mật không hợp lệ")
	}
	plain, err := gcm.Open(nil, blob[:gcm.NonceSize()], blob[gcm.NonceSize():], []byte(scope))
	if err != nil {
		return "", core.Fail(503, "Không giải mã được dữ liệu bảo mật")
	}
	return string(plain), nil
}
func totpCode(raw []byte, step int64) string {
	block := [8]byte{}
	binary.BigEndian.PutUint64(block[:], uint64(step))
	mac := hmac.New(sha1.New, raw)
	mac.Write(block[:])
	digest := mac.Sum(nil)
	offset := int(digest[len(digest)-1] & 15)
	return fmt.Sprintf("%06d", (binary.BigEndian.Uint32(digest[offset:offset+4])&0x7fffffff)%1000000)
}

var noPadding = base32.StdEncoding.WithPadding(base32.NoPadding)

func (s *AuthService) totpStep(user core.User, sealed, code string) (int64, error) {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return 0, core.Fail(401, "Mã xác thực không hợp lệ")
	}
	clear, err := readSecret(sealed, "totp:"+user.ID)
	if err != nil {
		return 0, err
	}
	raw, err := noPadding.DecodeString(clear)
	if err != nil {
		return 0, err
	}
	now := s.clock.Now().Unix() / 30
	for _, step := range []int64{now, now - 1, now + 1} {
		if step > user.TOTPStep && subtle.ConstantTimeCompare([]byte(totpCode(raw, step)), []byte(code)) == 1 {
			return step, nil
		}
	}
	return 0, core.Fail(401, "Mã xác thực sai, hết hạn hoặc đã dùng")
}
func (s *AuthService) useSecondFactor(user *core.User, code string) error {
	value := strings.ToLower(strings.TrimSpace(code))
	if len(value) == 20 {
		hashes := strings.Fields(user.RecoveryHashes)
		candidate := tokenHash("recovery:" + value)
		for i, hash := range hashes {
			if subtle.ConstantTimeCompare([]byte(hash), []byte(candidate)) == 1 {
				user.RecoveryHashes = strings.Join(append(hashes[:i], hashes[i+1:]...), " ")
				return nil
			}
		}
		return core.Fail(401, "Mã khôi phục không hợp lệ hoặc đã dùng")
	}
	step, err := s.totpStep(*user, user.TOTPSecret, value)
	if err == nil {
		user.TOTPStep = step
	}
	return err
}

func (s *AuthService) MFA(user core.User, action, password, code, session string) (map[string]any, error) {
	data := map[string]any{"enabled": user.TOTPSecret != ""}
	if action == "status" {
		return data, nil
	}
	if (action == "setup" || action == "disable") && (password == "" || len(password) > 128 || subtle.ConstantTimeCompare([]byte(user.PasswordHash), []byte(s.hasher.Hash(password, user.Salt))) != 1) {
		return nil, core.Fail(403, "Mật khẩu hiện tại không đúng")
	}
	err := updateAs(s.repo, user.ID, func(store core.Writer) error {
		fresh, ok := store.UserByID(user.ID)
		if !ok || fresh.Disabled || fresh.PasswordHash != user.PasswordHash {
			return core.Fail(409, "Cài đặt tài khoản đã thay đổi")
		}
		id := tokenHash("totp-pending:" + user.ID)
		switch action {
		case "setup":
			if fresh.TOTPSecret != "" {
				return core.Fail(409, "TOTP đã bật")
			}
			raw := make([]byte, 20)
			if _, err := rand.Read(raw); err != nil {
				return err
			}
			clear := noPadding.EncodeToString(raw)
			sealed, err := protectSecret(clear, "totp:"+user.ID)
			if err != nil {
				return err
			}
			store.PutRecord(core.Record{ID: id, TenantID: user.TenantID, Kind: "mfa_setup", Content: sealed, Enabled: true, UpdatedAt: s.clock.Now().Add(10 * time.Minute)})
			data["secret"] = clear
			return nil
		case "enable":
			pending, ok := store.Record(user.TenantID, id, "mfa_setup")
			if fresh.TOTPSecret != "" || !ok || !pending.Enabled || !pending.UpdatedAt.After(s.clock.Now()) {
				return core.Fail(409, "Thiết lập đã hết hạn")
			}
			step, err := s.totpStep(fresh, pending.Content, code)
			if err != nil {
				return err
			}
			fresh.TOTPSecret, fresh.TOTPStep = pending.Content, step
			codes, hashes := []string{}, []string{}
			for i := 0; i < 8; i++ {
				raw := make([]byte, 10)
				if _, err := rand.Read(raw); err != nil {
					return err
				}
				value := hex.EncodeToString(raw)
				codes = append(codes, value)
				hashes = append(hashes, tokenHash("recovery:"+value))
			}
			fresh.RecoveryHashes = strings.Join(hashes, " ")
			data["recovery_codes"] = codes
			pending.Content, pending.Enabled = "", false
			store.PutRecord(pending)
		case "disable":
			if fresh.TOTPSecret == "" {
				return core.Fail(409, "TOTP chưa bật")
			}
			if err := s.useSecondFactor(&fresh, code); err != nil {
				return err
			}
			fresh.TOTPSecret, fresh.TOTPStep, fresh.RecoveryHashes = "", 0, ""
		default:
			return core.Fail(400, "Thao tác không hợp lệ")
		}
		store.PutUser(fresh)
		for id := range store.Sessions(user.ID) {
			if id != tokenHash(session) {
				store.DeleteSession(id)
			}
		}
		data["enabled"] = fresh.TOTPSecret != ""
		return nil
	})
	return data, err
}

type ActiveSession struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Current   bool      `json:"current"`
}

func (s *AuthService) ActiveSessions(user core.User, token string) ([]ActiveSession, error) {
	items := []ActiveSession{}
	err := s.repo.View(func(store core.Reader) error {
		for id, session := range store.Sessions(user.ID) {
			if session.ExpiresAt.After(s.clock.Now()) {
				items = append(items, ActiveSession{ID: id, CreatedAt: session.CreatedAt, ExpiresAt: session.ExpiresAt, Current: id == tokenHash(token)})
			}
		}
		return nil
	})
	return items, err
}
func (s *AuthService) RevokeSession(user core.User, id string) error {
	return updateAs(s.repo, user.ID, func(store core.Writer) error {
		session, ok := store.Session(id)
		if !ok || session.UserID != user.ID {
			return core.Fail(404, "Không tìm thấy phiên")
		}
		store.DeleteSession(id)
		return nil
	})
}
