package service

import (
	"context"
	"strings"
	"time"

	"businessautomation/internal/core"
)

func (s *CSATService) RunEmail(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for ctx.Err() == nil {
		s.deliverSurveys(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *CSATService) deliverSurveys(ctx context.Context) {
	now := s.clock.Now()
	items := []core.Record{}
	_ = s.repo.View(func(store core.Reader) error {
		for _, tenant := range store.Tenants() {
			for _, item := range store.Records(tenant, "csat_mail") {
				if item.Enabled && !item.UpdatedAt.After(now) {
					items = append(items, item)
				}
				if len(items) >= 8 {
					return nil
				}
			}
		}
		return nil
	})
	for _, item := range items {
		if ctx.Err() != nil {
			return
		}
		valid := false
		_ = s.repo.View(func(store core.Reader) error {
			customer, ok := store.Record(item.TenantID, item.CustomerID, core.KindCustomers)
			ticket, found := store.Record(item.TenantID, item.ConversationID, core.KindTickets)
			valid = ok && found && strings.EqualFold(customer.Email, item.Email) && ticket.Status == core.StatusResolved && ticket.Rating == 0 && now.Sub(item.CreatedAt) < 23*time.Hour
			return nil
		})
		var deliveryErr error
		if valid {
			deliveryErr = s.mailer.Send(item.Email, "Vui lòng đánh giá dịch vụ hỗ trợ", "Cảm ơn bạn đã sử dụng dịch vụ:\r\n"+item.Content, item.ID, "", "")
		}
		_ = s.repo.Update(func(store core.Writer) error {
			current, ok := store.Record(item.TenantID, item.ID, "csat_mail")
			if !ok {
				return nil
			}
			if deliveryErr == nil {
				current.Enabled = false
				current.Content = ""
			} else {
				current.Sequence++
				current.UpdatedAt = now.Add(time.Minute * time.Duration(1<<min(current.Sequence, 6)))
			}
			store.PutRecord(current)
			return nil
		})
	}
}
