package service

import (
	"encoding/json"
	"log/slog"
	"sort"
	"strings"
	"time"

	"businessautomation/internal/core"
)

const fieldExtractionBatchesKind = "ai_extraction_batches"

type FieldExtractionRequest struct {
	RequestID string   `json:"request_id"`
	TicketIDs []string `json:"ticket_ids"`
	FieldIDs  []string `json:"field_ids"`
}

type FieldExtractionBatch struct {
	ID        string    `json:"id"`
	CreatedAt time.Time `json:"created_at"`
	Total     int       `json:"total"`
	Applied   int       `json:"applied"`
	Skipped   int       `json:"skipped"`
	Failed    int       `json:"failed"`
	Pending   int       `json:"pending"`
}

func extractionFields(store core.Reader, tenant string, ids []string) map[string]CustomField {
	fields := make(map[string]CustomField, len(ids))
	for _, id := range ids {
		config, found := store.Record(tenant, id, "custom_fields")
		var field CustomField
		if found && config.Enabled && json.Unmarshal([]byte(config.Content), &field) == nil && field.Kind == core.KindTickets {
			fields[id] = field
		}
	}
	return fields
}

func (o *Operations) QueueFieldExtraction(user core.User, req FieldExtractionRequest) error {
	if user.Role != core.RoleOwner {
		return core.Fail(403, "Chỉ owner được trích xuất trường hàng loạt")
	}
	if o.aiModel == nil {
		return core.Fail(503, "Chưa cấu hình OpenAI")
	}
	if len(req.RequestID) < 16 || len(req.RequestID) > 80 || len(req.TicketIDs) == 0 ||
		len(req.TicketIDs) > 50 || len(req.FieldIDs) == 0 || len(req.FieldIDs) > 10 {
		return core.Fail(400, "Chọn 1–50 phiếu và 1–10 trường tùy chỉnh")
	}
	id := tokenHash("field-extraction:" + req.RequestID)
	return o.repo.updateAsTenant(user.TenantID, user.ID, func(store core.Writer) error {
		if !featureEnabled(store, user.TenantID, "ai") {
			return core.Fail(403, "AI đã tắt cho doanh nghiệp")
		}
		if _, exists := store.Record(user.TenantID, id, fieldExtractionBatchesKind); exists {
			return nil
		}
		if len(extractionFields(store, user.TenantID, req.FieldIDs)) != len(req.FieldIDs) {
			return core.Fail(400, "Trường trích xuất phải đang bật và thuộc phiếu hỗ trợ")
		}
		tickets := make(map[string]bool, len(req.TicketIDs))
		for _, ticketID := range req.TicketIDs {
			if _, err := findOwnedRecord(store, user, ticketID, core.KindTickets); err != nil {
				return err
			}
			tickets[ticketID] = true
		}
		pending := 0
		for _, job := range store.Records(user.TenantID, workflowJobsKind) {
			if job.Subject == core.WorkflowAIExtractFields && job.Enabled {
				pending++
			}
		}
		if pending+len(tickets) > 100 {
			return core.Fail(409, "Doanh nghiệp có tối đa 100 phiếu chờ trích xuất")
		}
		now := o.clock.Now().UTC()
		store.PutRecord(core.Record{
			ID: id, TenantID: user.TenantID, Kind: fieldExtractionBatchesKind,
			AuthorID: user.ID, CreatedAt: now, UpdatedAt: now,
		})
		for ticketID := range tickets {
			store.PutRecord(core.Record{
				ID: tokenHash("extract-ticket:" + id + ":" + ticketID), TenantID: user.TenantID,
				Kind: workflowJobsKind, Subject: core.WorkflowAIExtractFields, AutomationID: id,
				ConversationID: ticketID, Tags: req.FieldIDs, AuthorID: user.ID, Enabled: true,
				Status: "pending", CreatedAt: now, UpdatedAt: now,
			})
		}
		return nil
	})
}

func (o *Operations) FieldExtractionBatches(user core.User) ([]FieldExtractionBatch, error) {
	items := []FieldExtractionBatch{}
	if user.Role != core.RoleOwner {
		return items, core.Fail(403, "Chỉ owner được xem các đợt trích xuất")
	}
	err := o.repo.ViewTenant(user.TenantID, func(store core.Reader) error {
		batches := store.Records(user.TenantID, fieldExtractionBatchesKind)
		sort.Slice(batches, func(i, j int) bool { return laterMessage(batches[i], batches[j]) })
		index := map[string]int{}
		for _, batch := range batches[:min(len(batches), 30)] {
			index[batch.ID] = len(items)
			items = append(items, FieldExtractionBatch{ID: batch.ID, CreatedAt: batch.CreatedAt})
		}
		for _, job := range store.Records(user.TenantID, workflowJobsKind) {
			i, found := index[job.AutomationID]
			if job.Subject != core.WorkflowAIExtractFields || !found {
				continue
			}
			batch := &items[i]
			batch.Total++
			switch job.Status {
			case "completed":
				batch.Applied++
			case "pending", "running":
				batch.Pending++
			case "failed":
				batch.Failed++
			default:
				batch.Skipped++
			}
		}
		return nil
	})
	return items, err
}

func (o *Operations) runFieldExtraction(job core.Record) bool {
	var ticket core.Record
	var fields map[string]CustomField
	var attempt uint64
	err := o.repo.UpdateTenant(job.TenantID, func(store core.Writer) error {
		current, found := store.LockWorkflowJob(job.TenantID, job.ID)
		now := o.clock.Now().UTC()
		if !found || current.EmailRetryAt != nil && current.EmailRetryAt.After(now) ||
			current.Status == "running" && current.UpdatedAt.Add(aiJobLease).After(now) {
			return nil
		}
		ticket, found = store.Record(job.TenantID, current.ConversationID, core.KindTickets)
		fields = extractionFields(store, job.TenantID, current.Tags)
		owner, authorized := store.UserByID(current.AuthorID)
		outcome := ""
		switch {
		case !featureEnabled(store, job.TenantID, "ai"), !found, len(fields) == 0,
			!authorized || owner.Disabled || owner.Role != core.RoleOwner:
			outcome = "skipped"
		case current.Sequence >= maxAIJobAttempts:
			outcome = "failed"
		}
		if outcome != "" {
			current.Enabled, current.Status, current.UpdatedAt = false, outcome, now
		} else {
			current.Sequence++
			lease := now.Add(aiJobLease)
			current.EmailRetryAt = &lease
			current.Status, current.UpdatedAt = "running", now
			attempt = current.Sequence
		}
		store.PutRecord(current)
		return nil
	})
	if err != nil {
		slog.Error("Field extraction claim failed", "job", job.ID, "error", err)
	}
	if err != nil || attempt == 0 {
		return false
	}
	var result struct{ Values map[string]string }
	err = o.aiModel.complete(job.TenantID, `Trích xuất giá trị có bằng chứng trực tiếp trong tiêu đề hoặc mô tả phiếu.
Trả JSON values là object dùng ID trường đã cho làm key và chuỗi làm value. Chỉ dùng "true"/"false" cho checkbox,
chuỗi số cho number. Bỏ qua trường thiếu dữ liệu. Không suy diễn hay sao chép thông tin định danh không liên quan.`,
		map[string]any{"fields": fields, "subject": clipped(ticket.Subject, 400), "description": clipped(ticket.Content, 8000)}, 1400, &result)
	if o.deferAIJobForQuota(job, attempt, err) {
		return true
	}
	if err != nil && attempt < maxAIJobAttempts {
		return true
	}
	providerFailed := err != nil
	actor := core.User{ID: job.AuthorID, TenantID: job.TenantID}
	if err := updateAsTenant(o.repo, actor, func(store core.Writer) error {
		current, found := store.LockWorkflowJob(job.TenantID, job.ID)
		if !found || current.Sequence != attempt {
			return nil
		}
		fresh, valid := store.Record(job.TenantID, current.ConversationID, core.KindTickets)
		owner, authorized := store.UserByID(current.AuthorID)
		now := o.clock.Now().UTC()
		status := "skipped"
		switch {
		case providerFailed:
			status = "failed"
		case valid && fresh.Subject == ticket.Subject && fresh.Content == ticket.Content &&
			authorized && !owner.Disabled && owner.Role == core.RoleOwner && featureEnabled(store, job.TenantID, "ai"):
			values := map[string]string{}
			if fresh.CustomValues != "" && json.Unmarshal([]byte(fresh.CustomValues), &values) != nil {
				status = "failed"
				break
			}
			if values == nil {
				values = map[string]string{}
			}
			for id, field := range extractionFields(store, job.TenantID, current.Tags) {
				input := strings.TrimSpace(result.Values[id])
				if _, exists := values[id]; !exists && fields[id] == field && input != "" && validCustomValue(field, input) {
					values[id], status = input, "completed"
				}
			}
			if status == "completed" {
				data, err := json.Marshal(values)
				if err != nil {
					return err
				}
				fresh.CustomValues, fresh.UpdatedAt = string(data), now
				store.PutRecord(fresh)
			}
		}
		current.Enabled, current.Status, current.UpdatedAt = false, status, now
		store.PutRecord(current)
		return nil
	}); err != nil {
		slog.Error("Field extraction commit failed", "job", job.ID, "error", err)
	}
	return true
}
