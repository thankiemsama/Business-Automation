package service

import (
	"sort"
	"strings"
	"time"

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
	if !ok || record.TenantID != user.TenantID || record.Kind != kind || (kind == core.KindCustomers && record.Status == core.CustomerMerged) {
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
	err := updateAsTenant(s.repo, user, func(store core.Writer) error {
		prepared, err := s.prepareRecord(store, user, kind, recordID, input)
		if err != nil {
			return err
		}
		input = prepared
		input.Tags, err = normalizeTags(input.Tags)
		if err != nil {
			return err
		}
		if err := policy.Validate(store, user, &input); err != nil {
			return err
		}
		input.Unread = false
		input.Overdue = false
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
	err := viewTenant(s.repo, user.TenantID, func(store core.Reader) error {
		if kind == core.KindMessages {
			if _, err := findOwnedRecord(store, user, recordID, core.KindConversations); err != nil {
				return err
			}
		}
		latest := map[string]uint64{}
		if kind == core.KindConversations {
			var messages []core.Record
			if recordID == "" {
				messages = store.Records(user.TenantID, core.KindMessages)
			} else {
				messages = store.ConversationMessages(user.TenantID, recordID)
			}
			for _, message := range messages {
				if message.Sender == core.SenderCustomer {
					latest[message.ConversationID] = max(latest[message.ConversationID], message.Sequence)
				}
			}
		}
		var records []core.Record
		switch {
		case kind == core.KindMessages:
			records = store.ConversationMessages(user.TenantID, recordID)
		case recordID != "":
			record, err := findOwnedRecord(store, user, recordID, kind)
			if err != nil {
				return err
			}
			records = []core.Record{record}
		default:
			records = store.Records(user.TenantID, kind)
		}
		for _, record := range records {
			if record.TenantID == user.TenantID && record.Kind == kind {
				if kind == core.KindCustomers && record.Status == core.CustomerMerged {
					continue
				}
				if kind == core.KindConversations {
					marker, _ := store.Record(user.TenantID, user.ID+":"+record.ID, "read_markers")
					record.Unread = latest[record.ID] > marker.Sequence
				}
				if kind == core.KindTickets {
					response, resolution := ticketDeadlines(record)
					record.ResponseDueAt, record.ResolutionDueAt = &response, &resolution
					now := s.clock.Now()
					record.Overdue = ticketOverdue(record, now)
					record.DueSoon = !record.Overdue && ticketOverdue(record, now.Add(30*time.Minute))
				}
				result = append(result, record)
			}
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
	if core.VersionedResource(kind) && recordID == "" {
		input.Version = 1
	}
	if recordID != "" {
		existing, err := findOwnedRecord(store, user, recordID, kind)
		if err != nil {
			return input, err
		}
		if core.VersionedResource(kind) {
			if input.Version == 0 {
				return input, core.Fail(428, "Cần phiên bản tài nguyên. Hãy tải lại dữ liệu trước khi lưu.")
			}
			if input.Version != existing.Version {
				return input, core.Fail(409, "Dữ liệu đã thay đổi. Hãy mở lại bản mới nhất và áp dụng các chỉnh sửa của bạn.")
			}
			input.Version = existing.Version + 1
		}
		if kind == core.KindConversations && input.CustomerID != existing.CustomerID {
			return input, core.Fail(409, "Không được đổi khách hàng của hội thoại")
		}
		input.AutomationID = existing.AutomationID
		input.AssigneeID = existing.AssigneeID
		input.CustomValues = existing.CustomValues
		input.Summary, input.SummarySource, input.RiskReason = existing.Summary, existing.SummarySource, existing.RiskReason
		input.Sentiment = existing.Sentiment
		input.ResponseP50, input.ResponseP90 = existing.ResponseP50, existing.ResponseP90
		input.ResolutionP50, input.ResolutionP90 = existing.ResolutionP50, existing.ResolutionP90
		if input.Tags == nil {
			input.Tags = existing.Tags
		}
		if kind == core.KindConversations {
			input.Email = existing.Email
		}
		input.FirstResponseAt = existing.FirstResponseAt
		input.ResolvedAt = existing.ResolvedAt
		input.ResolvedBy = existing.ResolvedBy
		input.EscalatedAt = existing.EscalatedAt
		input.RespondedBy = existing.RespondedBy
		input.Rating, input.RatedAt, input.RatingComment = existing.Rating, existing.RatedAt, existing.RatingComment
		if kind == core.KindKnowledge {
			input.Slug = existing.Slug
			if existing.Status == "draft" && !input.Enabled && !input.Public {
				input.Status = "draft"
			}
		}
		input.ID = recordID
		input.CreatedAt = existing.CreatedAt
	}
	if kind == core.KindTickets {
		if input.Status == core.StatusResolved && input.ResolvedAt == nil {
			input.ResolvedAt = &now
			input.ResolvedBy = user.ID
		}
		if input.Status != core.StatusResolved {
			input.ResolvedAt = nil
			input.ResolvedBy = ""
		}
	}
	return input, nil
}
