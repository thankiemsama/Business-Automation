package service

import (
	"sort"
	"strings"

	"businessautomation/internal/core"
)

type ResourceService struct {
	repo     core.Repository
	clock    Clock
	ids      IDGenerator
	policies map[string]ResourcePolicy
}

func NewResources(repo core.Repository, clock Clock, ids IDGenerator, policies map[string]ResourcePolicy) *ResourceService {
	policyRegistry := make(map[string]ResourcePolicy, len(policies))
	for k, v := range policies {
		policyRegistry[k] = v
	}
	return &ResourceService{repo: repo, clock: clock, ids: ids, policies: policyRegistry}
}

func findOwnedRecord(store core.Reader, user core.User, id, kind string) (core.Record, error) {
	record, ok := store.Record(user.TenantID, id, kind)
	if !ok || record.TenantID != user.TenantID || record.Kind != kind {
		return record, core.Fail(404, "Không tìm thấy dữ liệu")
	}
	return record, nil
}

func (s *ResourceService) Save(user core.User, kind, recordID string, input core.Record) (core.Record, error) {
	var result core.Record
	policy, ok := s.policies[kind]
	if !ok {
		return result, core.Fail(404, "Tài nguyên không tồn tại")
	}
	if !policy.CanWrite(user) {
		return result, core.Fail(403, "Chỉ owner được chỉnh sửa cấu hình")
	}
	err := s.repo.Update(func(store core.Writer) error {
		prepared, err := s.prepareRecord(store, user, kind, recordID, input)
		if err != nil {
			return err
		}
		input = prepared
		if err := policy.Validate(store, user, &input); err != nil {
			return err
		}
		store.PutRecord(input)
		result = input
		return nil
	})
	return result, err
}

func (s *ResourceService) List(user core.User, kind, recordID string) ([]core.Record, error) {
	if _, ok := s.policies[kind]; !ok && kind != core.KindMessages {
		return nil, core.Fail(404, "Tài nguyên không tồn tại")
	}
	result := []core.Record{}
	err := s.repo.View(func(store core.Reader) error {
		if kind == core.KindMessages {
			if _, err := findOwnedRecord(store, user, recordID, core.KindConversations); err != nil {
				return err
			}
		}
		for _, record := range store.Records(user.TenantID, kind) {
			if record.TenantID == user.TenantID && record.Kind == kind && (recordID == "" || record.ID == recordID || (kind == core.KindMessages && record.ConversationID == recordID)) {
				result = append(result, record)
			}
		}
		if kind != core.KindMessages && recordID != "" && len(result) == 0 {
			return core.Fail(404, "Không tìm thấy dữ liệu")
		}
		return nil
	})
	sort.Slice(result, func(i, j int) bool {
		if kind == core.KindMessages && result[i].Sequence != result[j].Sequence {
			return result[i].Sequence < result[j].Sequence
		}
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, err
}

func (s *ResourceService) prepareRecord(store core.Reader, user core.User, kind, recordID string, input core.Record) (core.Record, error) {
	now := s.clock.Now().UTC()
	input.Name = strings.TrimSpace(input.Name)
	input.Subject = strings.TrimSpace(input.Subject)
	input.Content = strings.TrimSpace(input.Content)
	input.AutomationID = ""
	input.ID = s.ids.NewID()
	input.TenantID = user.TenantID
	input.Kind = kind
	input.CreatedAt = now
	input.UpdatedAt = now
	input.Sender = ""
	if recordID != "" {
		existing, err := findOwnedRecord(store, user, recordID, kind)
		if err != nil {
			return input, err
		}
		if kind == core.KindConversations && input.CustomerID != existing.CustomerID {
			return input, core.Fail(409, "Không được đổi khách hàng của hội thoại")
		}
		input.AutomationID = existing.AutomationID
		input.ID = recordID
		input.CreatedAt = existing.CreatedAt
	}
	return input, nil
}
