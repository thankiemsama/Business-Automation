package service

import (
	"log/slog"
	"strings"

	"businessautomation/internal/core"
)

func knowledgeDraftEnabled(store core.Reader, tenant string) bool {
	return aiOptIn(store, tenant, "ai_knowledge_drafts")
}

func (w *workspaceWriter) queueKnowledgeDraft(ticket core.Record) {
	var gap core.Record
	for _, marker := range w.Records(ticket.TenantID, "ai_bot_markers") {
		if marker.ConversationID == ticket.ConversationID && marker.Name == "knowledge_gap" &&
			marker.MessageID != "" && laterMessage(marker, gap) {
			gap = marker
		}
	}
	if gap.ID == "" {
		return
	}
	id := tokenHash("knowledge-draft-job:" + gap.MessageID)
	if _, exists := w.Record(ticket.TenantID, id, workflowJobsKind); exists {
		return
	}
	now := w.workspace.Clock.Now().UTC()
	w.Writer.PutRecord(core.Record{
		ID: id, TenantID: ticket.TenantID, Kind: workflowJobsKind, Subject: core.WorkflowAIKnowledgeDraft,
		ConversationID: ticket.ID, MessageID: gap.MessageID, Enabled: true, Status: "pending",
		CreatedAt: now, UpdatedAt: now,
	})
}

func (o *Operations) runKnowledgeDraft(job core.Record) bool {
	var question core.Record
	var replies []string
	var attempt uint64
	err := o.repo.UpdateTenant(job.TenantID, func(store core.Writer) error {
		current, found := store.LockWorkflowJob(job.TenantID, job.ID)
		now := o.clock.Now().UTC()
		if !found || current.EmailRetryAt != nil && current.EmailRetryAt.After(now) ||
			current.Status == "running" && current.UpdatedAt.Add(aiJobLease).After(now) {
			return nil
		}
		stop := func(status string) {
			current.Enabled, current.Status, current.UpdatedAt = false, status, now
			store.PutRecord(current)
		}
		if !knowledgeDraftEnabled(store, job.TenantID) {
			stop("disabled")
			return nil
		}
		if current.Sequence >= maxAIJobAttempts {
			stop("failed")
			return nil
		}
		ticket, found := store.Record(job.TenantID, current.ConversationID, core.KindTickets)
		question, _ = store.Record(job.TenantID, current.MessageID, core.KindMessages)
		if !found || ticket.Status != core.StatusResolved || question.Sender != core.SenderCustomer ||
			question.ConversationID != ticket.ConversationID || strings.TrimSpace(question.Content) == "" {
			stop("superseded")
			return nil
		}
		if _, exists := store.Record(job.TenantID, gapArticleID(question), core.KindKnowledge); exists {
			stop("duplicate")
			return nil
		}
		for _, msg := range transcript(store, job.TenantID, ticket.ConversationID) {
			if msg.Sender == core.SenderAgent && laterMessage(msg, question) &&
				!msg.CreatedAt.After(current.CreatedAt) && msg.Status != "failed" && msg.Status != "pending" {
				replies = append(replies, clipped(msg.Content, 600))
			}
		}
		if len(replies) == 0 {
			stop("insufficient_evidence")
			return nil
		}
		replies = replies[max(0, len(replies)-12):]
		current.Sequence++
		lease := now.Add(aiJobLease)
		current.EmailRetryAt = &lease
		current.Status, current.UpdatedAt = "running", now
		store.PutRecord(current)
		attempt = current.Sequence
		return nil
	})
	if err != nil {
		slog.Error("Knowledge draft claim failed", "job", job.ID, "error", err)
	}
	if err != nil || attempt == 0 {
		return false
	}
	var draft struct{ Title, Body string }
	err = o.aiModel.complete(job.TenantID, `Soạn bản nháp FAQ tiếng Việt. Chỉ dùng thông tin tổng quát có căn cứ từ câu trả lời nhân viên.
Loại tên, số điện thoại, email, mã đơn, tài khoản và chi tiết riêng tư. Không suy diễn chính sách hoặc cam kết.
Trả JSON title (tối đa 150 ký tự) và body (tối đa 2500 ký tự), cả hai để rỗng nếu không có lời giải áp dụng được.`,
		map[string]any{"question": clipped(question.Content, 1200), "replies": replies}, 900, &draft)
	if o.deferAIJobForQuota(job, attempt, err) {
		return true
	}
	draft.Title, draft.Body = strings.TrimSpace(draft.Title), strings.TrimSpace(draft.Body)
	status := "completed"
	switch {
	case err != nil || len([]rune(draft.Title)) > 150 || len([]rune(draft.Body)) > 2500:
		if attempt < maxAIJobAttempts {
			return true
		}
		status = "failed"
	case draft.Title == "" || draft.Body == "":
		status = "insufficient_evidence"
	}
	if err := o.repo.UpdateTenant(job.TenantID, func(store core.Writer) error {
		current, found := store.LockWorkflowJob(job.TenantID, job.ID)
		if !found || current.Sequence != attempt {
			return nil
		}
		now := o.clock.Now().UTC()
		ticket, valid := store.Record(job.TenantID, current.ConversationID, core.KindTickets)
		fresh, found := store.Record(job.TenantID, question.ID, core.KindMessages)
		switch {
		case !knowledgeDraftEnabled(store, job.TenantID):
			status = "disabled"
		case !valid || ticket.Status != core.StatusResolved || !found || fresh.Content != question.Content:
			status = "superseded"
		case status == "completed":
			id := gapArticleID(question)
			if _, exists := store.Record(job.TenantID, id, core.KindKnowledge); exists {
				status = "duplicate"
				break
			}
			article := core.Record{
				ID: id, TenantID: job.TenantID, Kind: core.KindKnowledge, Status: "draft",
				Subject: draft.Title, Content: draft.Body, CreatedAt: now, UpdatedAt: now,
			}
			assignKnowledgeSlug(store, &article)
			store.PutRecord(article)
		}
		current.Enabled, current.Status, current.UpdatedAt = false, status, now
		store.PutRecord(current)
		return nil
	}); err != nil {
		slog.Error("Knowledge draft commit failed", "job", job.ID, "error", err)
	}
	return true
}

func gapArticleID(question core.Record) string {
	return tokenHash("knowledge-gap:" + strings.ToLower(strings.Join(strings.Fields(question.Content), " ")))
}
