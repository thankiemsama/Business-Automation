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
	items := []core.SurveyEmailJob{}
	_ = s.repo.View(func(store core.Reader) error {
		for _, tenant := range store.Tenants() {
			for _, record := range store.Records(tenant, core.KindSurveyEmailJobs) {
				item := core.SurveyEmailJobFromRecord(record)
				if item.Pending && !item.NextAttemptAt.After(now) {
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
			ticket, found := store.Record(item.TenantID, item.TicketID, core.KindTickets)
			valid = ok && found && strings.EqualFold(customer.Email, item.Recipient) && ticket.Status == core.StatusResolved && ticket.Rating == 0 && now.Sub(item.CreatedAt) < 23*time.Hour
			return nil
		})
		var deliveryErr error
		if valid {
			deliveryErr = s.mailer.Send(item.Recipient, "Vui lòng đánh giá dịch vụ hỗ trợ", "Cảm ơn bạn đã sử dụng dịch vụ:\r\n"+item.SurveyURL, item.ID, "", "")
		}
		_ = s.repo.Update(func(store core.Writer) error {
			record, ok := store.Record(item.TenantID, item.ID, core.KindSurveyEmailJobs)
			if !ok {
				return nil
			}
			current := core.SurveyEmailJobFromRecord(record)
			if deliveryErr == nil {
				current.Pending = false
				current.SurveyURL = ""
			} else {
				current.Attempts++
				current.NextAttemptAt = now.Add(time.Minute * time.Duration(1<<min(current.Attempts, 6)))
			}
			store.PutRecord(current.ApplyTo(record))
			return nil
		})
	}
}
