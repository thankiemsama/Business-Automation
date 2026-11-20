package service

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"unicode/utf8"

	"businessautomation/internal/core"
)

func voiceImportEnabled(store core.Reader, tenant string) bool {
	return aiOptIn(store, tenant, "ai_voice_import")
}

func (s *AIService) VoiceImportEnabled(user core.User) bool {
	enabled := false
	if s.TranscriptionEnabled() && user.Role == core.RoleOwner {
		_ = s.repo.ViewTenant(user.TenantID, func(store core.Reader) error {
			enabled = voiceImportEnabled(store, user.TenantID)
			return nil
		})
	}
	return enabled
}

func (s *AIService) ImportVoice(user core.User, customerID string, data []byte) (core.Record, error) {
	if user.Role != core.RoleOwner {
		return core.Record{}, core.Fail(403, "Chỉ owner được nhập ghi âm cuộc gọi")
	}
	if !s.TranscriptionEnabled() {
		return core.Record{}, core.Fail(503, "Nhà cung cấp AI chưa hỗ trợ chép lời")
	}
	fingerprint := sha256.Sum256(data)
	id := tokenHash("voice-import:" + user.TenantID + ":" + customerID + ":" + hex.EncodeToString(fingerprint[:]))
	var conversation core.Record
	validate := func(store core.Reader) error {
		if !voiceImportEnabled(store, user.TenantID) {
			return core.Fail(403, "Doanh nghiệp chưa bật nhập ghi âm cuộc gọi")
		}
		customer, err := findOwnedRecord(store, user, customerID, core.KindCustomers)
		if err != nil {
			return err
		}
		if customer.Status == core.CustomerMerged {
			return core.Fail(409, "Hồ sơ khách hàng đã được gộp")
		}
		conversation, _ = store.Record(user.TenantID, id, core.KindConversations)
		return nil
	}
	if err := s.repo.ViewTenant(user.TenantID, validate); err != nil || conversation.ID != "" {
		return conversation, err
	}
	text, err := s.transcribeAudio(user.TenantID, data)
	if err != nil {
		return conversation, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return conversation, core.Fail(422, "Bản ghi âm không có lời thoại")
	}
	if len(text) > maximumMessageBytes*20 {
		return conversation, core.Fail(422, "Bản chép lời quá dài, hãy chia nhỏ tệp ghi âm")
	}
	err = updateAsTenant(s.repo, user, func(store core.Writer) error {
		if err := validate(store); err != nil || conversation.ID != "" {
			return err
		}
		now := s.clock.Now().UTC()
		conversation = core.Record{
			ID: id, TenantID: user.TenantID, Kind: core.KindConversations, CustomerID: customerID,
			Subject: "Bản chép lời cuộc gọi", Status: core.StatusOpen, Channel: core.ChannelPhone,
			CreatedAt: now, UpdatedAt: now,
		}
		store.PutRecord(conversation)
		for sequence := uint64(1); text != ""; sequence++ {
			end := min(len(text), maximumMessageBytes)
			for end < len(text) && !utf8.RuneStart(text[end]) {
				end--
			}
			store.PutRecord(core.Record{
				ID: s.repo.IDs.NewID(), TenantID: user.TenantID, Kind: core.KindMessages,
				ConversationID: id, Sender: "note", AuthorID: user.ID, Content: text[:end],
				AutomationID: voiceTranscriptMarker,
				Sequence: sequence, CreatedAt: now, UpdatedAt: now,
			})
			text = text[end:]
		}
		return nil
	})
	return conversation, err
}
