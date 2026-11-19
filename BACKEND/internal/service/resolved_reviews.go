package service

import (
	"encoding/json"
	"log/slog"
	"sort"
	"strings"
	"time"

	"businessautomation/internal/core"
)

const qaReviewsKind = "qa_reviews"

type QAReview struct {
	Status     string   `json:"status"`
	Score      *int     `json:"score,omitempty"`
	Findings   []string `json:"findings,omitempty"`
	Tags       []string `json:"tags,omitempty"`
	Impression string   `json:"customer_impression,omitempty"`
}

func reviewEnabled(store core.Reader, tenant string) bool {
	return aiOptIn(store, tenant, "ai_quality_review")
}

func reviewID(ticket core.Record) string {
	if ticket.ResolvedAt == nil {
		return tokenHash("resolved-review:" + ticket.ID + ":" + ticket.UpdatedAt.UTC().Format(time.RFC3339Nano))
	}
	return tokenHash("resolved-review:" + ticket.ID + ":" + ticket.ResolvedAt.UTC().Format(time.RFC3339Nano))
}

func (w *workspaceWriter) queueResolvedReview(ticket core.Record) {
	id := reviewID(ticket)
	if _, exists := w.Record(ticket.TenantID, id, workflowJobsKind); exists {
		return
	}
	now := w.workspace.Clock.Now().UTC()
	w.Writer.PutRecord(core.Record{
		ID: id, TenantID: ticket.TenantID, Kind: workflowJobsKind, Subject: core.WorkflowAIReview,
		ConversationID: ticket.ID, CustomerID: ticket.CustomerID, Enabled: true, Status: "pending",
		CreatedAt: now, UpdatedAt: now,
	})
}

func currentReviewTicket(store core.Reader, job core.Record) (core.Record, bool) {
	ticket, exists := store.Record(job.TenantID, job.ConversationID, core.KindTickets)
	return ticket, exists && ticket.Status == core.StatusResolved && reviewID(ticket) == job.ID
}

func (o *Operations) runResolvedReview(job core.Record) bool {
	type line struct{ Role, Text string }
	var ticket core.Record
	var lines []line
	var attempt uint64
	err := o.repo.UpdateTenant(job.TenantID, func(store core.Writer) error {
		current, ok := store.LockWorkflowJob(job.TenantID, job.ID)
		now := o.clock.Now().UTC()
		if !ok || current.EmailRetryAt != nil && current.EmailRetryAt.After(now) ||
			current.Status == "running" && current.UpdatedAt.Add(aiJobLease).After(now) {
			return nil
		}
		ticket, ok = currentReviewTicket(store, current)
		switch {
		case !ok:
			return finishReview(store, current, QAReview{Status: "superseded"}, now)
		case !reviewEnabled(store, job.TenantID):
			return finishReview(store, current, QAReview{Status: "disabled"}, now)
		case current.Sequence >= maxAIJobAttempts:
			return finishReview(store, current, QAReview{Status: "failed"}, now)
		}
		if ticket.ConversationID != "" {
			messages := store.ConversationMessages(job.TenantID, ticket.ConversationID)
			sort.Slice(messages, func(i, j int) bool { return laterMessage(messages[j], messages[i]) })
			for _, msg := range messages {
				if (msg.Sender == core.SenderAgent || msg.Sender == core.SenderCustomer) &&
					!msg.CreatedAt.After(current.CreatedAt) && strings.TrimSpace(msg.Content) != "" {
					lines = append(lines, line{msg.Sender, clipped(msg.Content, 500)})
				}
			}
			if len(lines) > 40 {
				lines = append(lines[:8:8], lines[len(lines)-32:]...)
			}
		}
		current.Sequence++
		lease := now.Add(aiJobLease)
		current.EmailRetryAt = &lease
		current.Status, current.UpdatedAt = "running", now
		store.PutRecord(current)
		attempt = current.Sequence
		return nil
	})
	if err != nil {
		slog.Error("AI review claim failed", "job", job.ID, "error", err)
	}
	if err != nil || attempt == 0 {
		return false
	}

	var review QAReview
	err = o.aiModel.complete(job.TenantID, `Kiểm tra chất lượng xử lý phiếu hỗ trợ bằng tiếng Việt.
Trả JSON gồm score (số nguyên 0-100, null nếu thiếu căn cứ đánh giá cách xử lý),
findings (tối đa 4 nhận xét ngắn dựa trên bằng chứng), tags (tối đa 4 nhãn chủ đề, mỗi nhãn tối đa 40 ký tự)
và customer_impression (tối đa 240 ký tự về nhu cầu hoặc phản hồi thể hiện rõ, rỗng nếu thiếu căn cứ).
Đánh giá độ chính xác, lời giải và giao tiếp. Không suy diễn đặc điểm cá nhân, không chép thông tin định danh.
Số điểm phải là null nếu không có tin nhắn của nhân viên.`, map[string]any{
		"subject": clipped(ticket.Subject, 400), "description": clipped(ticket.Content, 2500), "messages": lines,
	}, 600, &review)
	if o.deferAIJobForQuota(job, attempt, err) {
		return true
	}
	if err == nil {
		hasAgent := false
		for _, message := range lines {
			hasAgent = hasAgent || message.Role == core.SenderAgent
		}
		if !hasAgent {
			review.Score = nil
		}
		err = validateQAReview(&review)
	}
	if err != nil {
		slog.Warn("AI review attempt failed", "job", job.ID, "attempt", attempt)
		if attempt < maxAIJobAttempts {
			return true
		}
		review = QAReview{Status: "failed"}
	} else {
		review.Status = "completed"
		if review.Score == nil {
			review.Status = "insufficient_evidence"
		}
	}
	if err := o.repo.UpdateTenant(job.TenantID, func(store core.Writer) error {
		current, ok := store.LockWorkflowJob(job.TenantID, job.ID)
		if !ok || current.Sequence != attempt {
			return nil
		}
		fresh, valid := currentReviewTicket(store, current)
		now := o.clock.Now().UTC()
		if !valid {
			return finishReview(store, current, QAReview{Status: "superseded"}, now)
		}
		if !reviewEnabled(store, job.TenantID) {
			return finishReview(store, current, QAReview{Status: "disabled"}, now)
		}
		if tags := appendReviewTags(fresh.Tags, review.Tags); len(tags) > len(fresh.Tags) {
			fresh.Tags, fresh.UpdatedAt = tags, now
			store.PutRecord(fresh)
		}
		return finishReview(store, current, review, now)
	}); err != nil {
		slog.Error("AI review commit failed", "job", job.ID, "error", err)
	}
	return true
}

func validateQAReview(review *QAReview) error {
	var err error
	review.Tags, err = normalizeTags(review.Tags)
	if err != nil || len(review.Tags) > 4 || len(review.Findings) > 4 ||
		len([]rune(review.Impression)) > 240 || review.Score != nil && (*review.Score < 0 || *review.Score > 100) {
		return core.Fail(502, "Kết quả kiểm tra chất lượng không hợp lệ")
	}
	for _, finding := range review.Findings {
		if strings.TrimSpace(finding) == "" || len([]rune(finding)) > 300 {
			return core.Fail(502, "Nhận xét kiểm tra chất lượng không hợp lệ")
		}
	}
	return nil
}

func appendReviewTags(existing, proposed []string) []string {
	tags := append([]string(nil), existing...)
	for _, tag := range proposed {
		if len(tags) >= 20 {
			break
		}
		seen := false
		for _, value := range tags {
			if strings.EqualFold(value, tag) {
				seen = true
				break
			}
		}
		if !seen {
			tags = append(tags, tag)
		}
	}
	return tags
}

func finishReview(store core.Writer, job core.Record, review QAReview, now time.Time) error {
	data, err := json.Marshal(review)
	if err != nil {
		return err
	}
	store.PutRecord(core.Record{
		ID: tokenHash("qa-result:" + job.ID), TenantID: job.TenantID, Kind: qaReviewsKind,
		ConversationID: job.ConversationID, CustomerID: job.CustomerID, Status: review.Status,
		Content: string(data), CreatedAt: job.CreatedAt, UpdatedAt: now,
	})
	job.Enabled, job.Status, job.UpdatedAt = false, review.Status, now
	store.PutRecord(job)
	return nil
}

func (s *AIService) TicketReview(user core.User, id string) (*QAReview, error) {
	var result *QAReview
	err := s.repo.ViewTenant(user.TenantID, func(store core.Reader) error {
		ticket, err := findOwnedRecord(store, user, id, core.KindTickets)
		if err != nil || ticket.Status != core.StatusResolved {
			return err
		}
		jobID := reviewID(ticket)
		if record, ok := store.Record(user.TenantID, tokenHash("qa-result:"+jobID), qaReviewsKind); ok {
			return json.Unmarshal([]byte(record.Content), &result)
		}
		if job, ok := store.Record(user.TenantID, jobID, workflowJobsKind); ok {
			result = &QAReview{Status: job.Status}
		}
		return nil
	})
	return result, err
}
