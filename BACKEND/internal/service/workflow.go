package service

import (
	"sort"
	"strings"
	"time"

	"businessautomation/internal/core"
)

func (s *ResourceService) Assign(user core.User, kind, id, assignee string, claim bool) (core.Record, error) {
	var result core.Record
	err := updateAs(s.repo, user.ID, func(store core.Writer) error {
		record, err := findOwnedRecord(store, user, id, kind)
		if err != nil {
			return err
		}
		if claim {
			assignee = user.ID
			if record.AssigneeID != "" && record.AssigneeID != user.ID {
				current, ok := store.UserByID(record.AssigneeID)
				if ok && !current.Disabled {
					return core.Fail(409, "Đã có người phụ trách")
				}
			}
		}
		if assignee != "" {
			target, ok := store.UserByID(assignee)
			if !ok || target.TenantID != user.TenantID || target.Disabled {
				return core.Fail(400, "Người phụ trách không hợp lệ")
			}
		}
		record.AssigneeID = assignee
		record.UpdatedAt = s.clock.Now().UTC()
		store.PutRecord(record)
		result = record
		return nil
	})
	return result, err
}

func latestInbound(store core.Reader, user core.User, id string) uint64 {
	var sequence uint64
	for _, message := range store.Records(user.TenantID, core.KindMessages) {
		if message.ConversationID == id && message.Sender == core.SenderCustomer {
			sequence = max(sequence, message.Sequence)
		}
	}
	return sequence
}

func (s *ResourceService) ReadConversation(user core.User, id string, seen uint64) error {
	return s.repo.Update(func(store core.Writer) error {
		if _, err := findOwnedRecord(store, user, id, core.KindConversations); err != nil {
			return err
		}
		key := user.ID + ":" + id
		marker, _ := store.Record(user.TenantID, key, "read_markers")
		seen = min(seen, latestInbound(store, user, id))
		if seen > marker.Sequence {
			store.PutRecord(core.Record{ID: key, TenantID: user.TenantID, Kind: "read_markers", Sequence: seen})
		}
		return nil
	})
}

func ticketDeadlines(ticket core.Record) (time.Time, time.Time) {
	response, resolution := 4*time.Hour, 48*time.Hour
	switch ticket.Priority {
	case core.PriorityUrgent:
		response, resolution = time.Hour, 8*time.Hour
	case core.PriorityHigh:
		response, resolution = 2*time.Hour, 24*time.Hour
	case core.PriorityLow:
		response, resolution = 8*time.Hour, 72*time.Hour
	}
	return ticket.CreatedAt.Add(response), ticket.CreatedAt.Add(resolution)
}

func ticketOverdue(ticket core.Record, now time.Time) bool {
	if ticket.Status == core.StatusResolved {
		return false
	}
	response, resolution := ticketDeadlines(ticket)
	return !now.Before(resolution) || (ticket.FirstResponseAt == nil && !now.Before(response))
}

func (s *ResourceService) RecordResponse(user core.User, id string) (core.Record, error) {
	var ticket core.Record
	err := s.repo.Update(func(store core.Writer) error {
		current, err := findOwnedRecord(store, user, id, core.KindTickets)
		if err != nil {
			return err
		}
		if current.Status == core.StatusResolved {
			return core.Fail(409, "Phiếu đã xử lý")
		}
		if current.FirstResponseAt == nil {
			recordTicketResponse(store, &current, user.ID, s.clock.Now().UTC())
		}
		ticket = current
		return nil
	})
	return ticket, err
}

func recordTicketResponse(store core.Writer, ticket *core.Record, author string, now time.Time) {
	ticket.FirstResponseAt, ticket.UpdatedAt, ticket.RespondedBy = &now, now, author
	store.PutRecord(*ticket)
}

func (s *ResourceService) Escalate(user core.User, id string) (core.Record, error) {
	var result core.Record
	err := s.repo.Update(func(store core.Writer) error {
		ticket, err := findOwnedRecord(store, user, id, core.KindTickets)
		if err != nil {
			return err
		}
		now := s.clock.Now().UTC()
		if ticket.EscalatedAt != nil {
			result = ticket
			return nil
		}
		if !ticketOverdue(ticket, now) {
			return core.Fail(409, "Phiếu chưa quá hạn")
		}
		for _, member := range store.Users(user.TenantID) {
			if member.Role == core.RoleOwner && !member.Disabled {
				ticket.AssigneeID = member.ID
				break
			}
		}
		ticket.EscalatedAt, ticket.UpdatedAt = &now, now
		store.PutRecord(ticket)
		result = ticket
		return nil
	})
	return result, err
}

func (s *AuthService) Members(user core.User) ([]core.User, error) {
	var members []core.User
	err := s.repo.View(func(store core.Reader) error { members = store.Users(user.TenantID); return nil })
	sort.Slice(members, func(i, j int) bool { return members[i].Email < members[j].Email })
	return members, err
}

func (s *AuthService) UpdateMember(owner core.User, id, name, email string, disabled bool) (core.User, error) {
	var result core.User
	if owner.Role != core.RoleOwner {
		return result, core.Fail(403, "Chỉ owner được quản lý đội ngũ")
	}
	creds, err := normalizeCredentials(Credentials{Name: name, Email: email, Password: strings.Repeat("x", minimumPasswordBytes)})
	if err != nil {
		return result, err
	}
	err = updateAs(s.repo, owner.ID, func(store core.Writer) error {
		member, ok := store.UserByID(id)
		if !ok || member.TenantID != owner.TenantID || member.Role != core.RoleAgent {
			return core.Fail(404, "Không tìm thấy nhân viên")
		}
		if other, exists := store.UserByEmail(creds.Email); exists && other.ID != id {
			return core.Fail(409, "Email đã tồn tại")
		}
		member.Name, member.Email, member.Disabled = creds.Name, creds.Email, disabled
		if disabled {
			store.RevokeSessions(id)
			for _, kind := range []string{core.KindConversations, core.KindTickets} {
				for _, record := range store.Records(owner.TenantID, kind) {
					if record.AssigneeID == id {
						record.AssigneeID = ""
						record.UpdatedAt = s.clock.Now().UTC()
						store.PutRecord(record)
					}
				}
			}
		}
		store.PutUser(member)
		result = member
		return nil
	})
	return result, err
}
