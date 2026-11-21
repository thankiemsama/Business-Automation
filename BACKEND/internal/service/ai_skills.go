package service

import (
	"encoding/json"
	"strings"

	"businessautomation/internal/core"
)

// aiSkillsKind stores the tenant's skill catalog: named conversational
// behaviors (intent + instructions + tone) the AI agent applies before its
// generic answer path. This is the Gorgias-style "Skills" primitive that makes
// non-engineers tune agent behavior without touching the model.
const aiSkillsKind = "ai_skills"

// aiFeedbackKind records owner ratings of individual AI replies so the agent
// can learn which patterns worked. This is the "direct feedback loop".
const aiFeedbackKind = "ai_feedback"

// AISkill bundles an intent trigger, free-text instructions, and an optional
// tone that the answer path folds into its SOP.
type AISkill struct {
	ID           string   `json:"id,omitempty"`
	Name         string   `json:"name"`
	Enabled      bool     `json:"enabled"`
	Trigger      []string `json:"trigger,omitempty"`
	Instructions string   `json:"instructions,omitempty"`
	Tone         string   `json:"tone,omitempty"`
	Channel      string   `json:"channel,omitempty"`
}

func validSkillTone(tone string) bool {
	switch tone {
	case "", "formal", "friendly", "playful", "concise", "empathetic":
		return true
	}
	return false
}

// NewAISkills builds the owner-facing skill + feedback service.
func NewAISkills(repo *Workspace, clock Clock) *AISkills {
	return &AISkills{repo: repo, clock: clock}
}

// AISkills manages the tenant's skill catalog and reply-feedback loop.
type AISkills struct {
	repo  *Workspace
	clock Clock
}

// Skills returns the tenant's skills for management.
func (s *AISkills) Skills(user core.User) ([]AISkill, error) {
	if user.Role != core.RoleOwner {
		return nil, core.Fail(403, "Chỉ owner quản lý kỹ năng AI")
	}
	result := []AISkill{}
	err := s.repo.ViewTenant(user.TenantID, func(store core.Reader) error {
		for _, record := range store.Records(user.TenantID, aiSkillsKind) {
			if record.Status == "removed" {
				continue
			}
			var item AISkill
			if json.Unmarshal([]byte(record.Content), &item) == nil {
				result = append(result, item)
			}
		}
		return nil
	})
	return result, err
}

// SaveSkill creates or updates a skill.
func (s *AISkills) SaveSkill(user core.User, item AISkill) (AISkill, error) {
	if user.Role != core.RoleOwner {
		return item, core.Fail(403, "Chỉ owner quản lý kỹ năng AI")
	}
	item.Name = strings.TrimSpace(item.Name)
	item.Instructions = strings.TrimSpace(item.Instructions)
	item.Tone = strings.TrimSpace(item.Tone)
	if item.Name == "" || len([]rune(item.Name)) > 80 {
		return item, core.Fail(400, "Tên kỹ năng không hợp lệ")
	}
	if len([]rune(item.Instructions)) > 2000 {
		return item, core.Fail(400, "Hướng dẫn kỹ năng tối đa 2.000 ký tự")
	}
	if !validSkillTone(item.Tone) {
		return item, core.Fail(400, "Giọng điệu kỹ năng không hợp lệ")
	}
	if len(item.Trigger) > 30 {
		return item, core.Fail(400, "Tối đa 30 từ khóa kích hoạt")
	}
	if item.Channel != "" && !contains(core.PublicChannels, item.Channel) &&
		!contains([]string{core.ChannelWeb, core.ChannelEmail, core.ChannelPhone}, item.Channel) {
		return item, core.Fail(400, "Kênh kỹ năng không hợp lệ")
	}
	err := s.repo.UpdateTenant(user.TenantID, func(store core.Writer) error {
		if item.ID == "" {
			item.ID = s.repo.IDs.NewID()
			count := 0
			for _, record := range store.Records(user.TenantID, aiSkillsKind) {
				if record.Status != "removed" {
					count++
				}
			}
			if count >= 30 {
				return core.Fail(409, "Tối đa 30 kỹ năng AI đang lưu")
			}
		} else {
			old, ok := store.Record(user.TenantID, item.ID, aiSkillsKind)
			if !ok || old.Status == "removed" {
				return core.Fail(404, "Không tìm thấy kỹ năng")
			}
		}
		payload, err := json.Marshal(item)
		if err != nil {
			return err
		}
		store.PutRecord(core.Record{ID: item.ID, TenantID: user.TenantID, Kind: aiSkillsKind,
			Name: item.Name, Content: string(payload), Enabled: item.Enabled, UpdatedAt: s.clock.Now().UTC()})
		return nil
	})
	return item, err
}

// RemoveSkill disables a skill.
func (s *AISkills) RemoveSkill(user core.User, id string) error {
	if user.Role != core.RoleOwner {
		return core.Fail(403, "Chỉ owner quản lý kỹ năng AI")
	}
	return s.repo.UpdateTenant(user.TenantID, func(store core.Writer) error {
		record, ok := store.Record(user.TenantID, id, aiSkillsKind)
		if !ok || record.Status == "removed" {
			return core.Fail(404, "Không tìm thấy kỹ năng")
		}
		record.Enabled, record.Status = false, "removed"
		record.UpdatedAt = s.clock.Now().UTC()
		store.PutRecord(record)
		return nil
	})
}

// MatchedSkill returns the first enabled skill whose trigger keywords match the
// message (and channel, if set), so the answer path can fold its instructions
// and tone into the SOP. Returns ok=false when no skill applies.
func MatchedSkill(store core.Reader, tenant, channel, message string) (AISkill, bool) {
	for _, record := range store.Records(tenant, aiSkillsKind) {
		if !record.Enabled || record.Status == "removed" {
			continue
		}
		var item AISkill
		if json.Unmarshal([]byte(record.Content), &item) != nil {
			continue
		}
		if item.Channel != "" && item.Channel != channel {
			continue
		}
		if len(item.Trigger) > 0 && !matchesAnyKeyword(message, item.Trigger) {
			continue
		}
		return item, true
	}
	return AISkill{}, false
}

// skillSOP builds the SOP fragment a matched skill contributes: its
// instructions plus an explicit tone directive, ready to pass to answerWithSOP.
func (skill AISkill) SOP() string {
	parts := []string{strings.TrimSpace(skill.Instructions)}
	if skill.Tone != "" {
		parts = append(parts, "Trả lời với giọng điệu: "+skill.Tone)
	}
	joined := strings.Join(parts, "\n")
	if joined == "" {
		joined = "Áp dụng kỹ năng: " + skill.Name
	}
	return clipped(joined, 2000)
}

// RecordFeedback stores an owner rating of one AI reply for the feedback loop.
// Rating is 1 (poor) to 5 (good). This is what the agent reads to improve.
func (s *AISkills) RecordFeedback(user core.User, messageID string, rating int, comment string) error {
	if user.Role != core.RoleOwner {
		return core.Fail(403, "Chỉ owner đánh giá phản hồi AI")
	}
	if rating < 1 || rating > 5 {
		return core.Fail(400, "Điểm phản hồi phải từ 1 đến 5")
	}
	comment = strings.TrimSpace(comment)
	if len([]rune(comment)) > 500 {
		return core.Fail(400, "Nhận xét tối đa 500 ký tự")
	}
	return s.repo.UpdateTenant(user.TenantID, func(store core.Writer) error {
		message, ok := store.Record(user.TenantID, messageID, core.KindMessages)
		if !ok || message.Sender != core.SenderAssistant {
			return core.Fail(404, "Không tìm thấy phản hồi AI để đánh giá")
		}
		payload, err := json.Marshal(map[string]any{"rating": rating, "comment": comment})
		if err != nil {
			return err
		}
		store.PutRecord(core.Record{ID: tokenHash("ai-feedback:" + messageID), TenantID: user.TenantID,
			Kind: aiFeedbackKind, Name: messageID, Content: string(payload),
			ConversationID: message.ConversationID, CreatedAt: s.clock.Now().UTC(), UpdatedAt: s.clock.Now().UTC()})
		return nil
	})
}

// FeedbackStats aggregates the tenant's stored feedback so the owner can see
// which replies were rated and the average.
func (s *AISkills) FeedbackStats(user core.User) (map[string]any, error) {
	if user.Role != core.RoleOwner {
		return nil, core.Fail(403, "Chỉ owner xem phản hồi AI")
	}
	stats := map[string]any{"count": 0, "average": 0.0, "improve": 0}
	err := s.repo.ViewTenant(user.TenantID, func(store core.Reader) error {
		sum := 0
		for _, record := range store.Records(user.TenantID, aiFeedbackKind) {
			var item struct {
				Rating int `json:"rating"`
			}
			if json.Unmarshal([]byte(record.Content), &item) != nil {
				continue
			}
			stats["count"] = any(stats["count"].(int) + 1)
			sum += item.Rating
			if item.Rating <= 2 {
				stats["improve"] = any(stats["improve"].(int) + 1)
			}
		}
		if stats["count"].(int) > 0 {
			stats["average"] = float64(sum) / float64(stats["count"].(int))
		}
		return nil
	})
	return stats, err
}
