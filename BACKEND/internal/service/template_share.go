package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"businessautomation/internal/core"
)

const templateSharePrefix = "nexa-template-v1."
const templateShareMaxCode = 48 * 1024
const templateShareSaltSize = 16

type sharedReplyTemplate struct {
	Subject string `json:"subject"`
	Content string `json:"content"`
}

func templateCipher(password string, salt []byte) (cipher.AEAD, error) {
	if utf8.RuneCountInString(password) < 12 || len(password) > 256 || strings.TrimSpace(password) == "" {
		return nil, core.Fail(400, "Mật khẩu chia sẻ phải có ít nhất 12 ký tự và tối đa 256 byte")
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, 600_000, 32)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCMWithRandomNonce(block)
}

func (s *Workspace) ExportReplyTemplate(user core.User, id, password string) (string, error) {
	if !(KnowledgePolicy{}).CanWrite(user) {
		return "", core.Fail(403, "Chỉ owner được chia sẻ mẫu trả lời")
	}
	var template sharedReplyTemplate
	if err := s.ViewTenant(user.TenantID, func(store core.Reader) error {
		record, err := findOwnedRecord(store, user, id, "templates")
		template = sharedReplyTemplate{Subject: record.Subject, Content: record.Content}
		return err
	}); err != nil {
		return "", err
	}
	data, err := json.Marshal(template)
	if err != nil {
		return "", err
	}
	if base64.RawURLEncoding.EncodedLen(len(data)+templateShareSaltSize+28)+len(templateSharePrefix) > templateShareMaxCode {
		return "", core.Fail(400, "Mẫu trả lời quá dài để chia sẻ")
	}
	salt := make([]byte, templateShareSaltSize)
	rand.Read(salt)
	encryption, err := templateCipher(password, salt)
	if err != nil {
		return "", err
	}
	sealed := encryption.Seal(salt, nil, data, []byte(templateSharePrefix))
	return templateSharePrefix + base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (s *Workspace) ImportReplyTemplate(user core.User, code, password string) (core.Record, error) {
	if !(KnowledgePolicy{}).CanWrite(user) {
		return core.Record{}, core.Fail(403, "Chỉ owner được nhập mẫu trả lời")
	}
	invalid := core.Fail(400, "Mã chia sẻ hoặc mật khẩu không đúng")
	code = strings.TrimSpace(code)
	encoded, ok := strings.CutPrefix(code, templateSharePrefix)
	if !ok || len(code) > templateShareMaxCode {
		return core.Record{}, invalid
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || len(data) < templateShareSaltSize+28 {
		return core.Record{}, invalid
	}
	encryption, err := templateCipher(password, data[:templateShareSaltSize])
	if err != nil {
		return core.Record{}, err
	}
	cleartext, err := encryption.Open(nil, nil, data[templateShareSaltSize:], []byte(templateSharePrefix))
	if err != nil {
		return core.Record{}, invalid
	}
	var input sharedReplyTemplate
	if err := json.Unmarshal(cleartext, &input); err != nil {
		return core.Record{}, invalid
	}
	if utf8.RuneCountInString(input.Subject) > 200 || utf8.RuneCountInString(input.Content) > 16000 {
		return core.Record{}, core.Fail(400, "Tên hoặc nội dung mẫu quá dài")
	}
	var result core.Record
	err = s.updateAsTenant(user.TenantID, user.ID, func(store core.Writer) error {
		id := tokenHash("template-import:" + string(data))
		if existing, found := store.Record(user.TenantID, id, "templates"); found {
			result = existing
			return nil
		}
		now := s.Clock.Now().UTC()
		result = core.Record{
			ID: id, TenantID: user.TenantID, Kind: "templates",
			Subject: strings.TrimSpace(input.Subject), Content: strings.TrimSpace(input.Content),
			CreatedAt: now, UpdatedAt: now, Enabled: true,
		}
		if err := (KnowledgePolicy{}).Validate(store, user, &result); err != nil {
			return err
		}
		store.PutRecord(result)
		result, _ = store.Record(user.TenantID, id, "templates")
		return nil
	})
	return result, err
}
