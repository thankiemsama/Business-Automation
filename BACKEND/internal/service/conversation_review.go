package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"businessautomation/internal/core"
)

const voiceTranscriptMarker = "voice_import_transcript"

type ConversationReview struct {
	Modality string        `json:"modality,omitempty"`
	Labels   []QARiskLabel `json:"labels,omitempty"`
	Advice   string        `json:"advice,omitempty"`
}

func (w *workspaceWriter) queueConversationReview(source core.Record, subject string) {
	now := w.workspace.Clock.Now().UTC()
	id := tokenHash(subject + ":" + source.TenantID + ":" + source.ID)
	if subject == core.WorkflowOpsReview {
		id = tokenHash(id + ":" + source.RiskReason + ":" + now.Format(time.RFC3339Nano))
	}
	if _, exists := w.Record(source.TenantID, id, workflowJobsKind); exists {
		return
	}
	w.Writer.PutRecord(core.Record{ID: id, TenantID: source.TenantID, Kind: workflowJobsKind,
		Subject: subject, MessageID: source.ID, ConversationID: source.ConversationID,
		Name: source.RiskReason, Status: "pending", Enabled: true, CreatedAt: now, UpdatedAt: now})
}

func (o *Operations) runConversationReview(job core.Record) bool {
	sales := job.Subject == core.WorkflowSalesReview
	ops := job.Subject == core.WorkflowOpsReview
	flag, kind := "ai_multimodal_qa", core.KindMessages
	if sales {
		flag, kind = "ai_sales_persona", campaignLeadKind
	} else if ops {
		flag, kind = "ai_ops_persona", core.KindTickets
	}
	var source, thread core.Record
	var settings TenantSettings
	var threadMessages []core.Record
	claimed := false
	err := o.repo.UpdateTenant(job.TenantID, func(store core.Writer) error {
		current, ok := store.LockWorkflowJob(job.TenantID, job.ID)
		now := o.clock.Now().UTC()
		if !ok || current.Subject != job.Subject || current.EmailRetryAt != nil && current.EmailRetryAt.After(now) {
			return nil
		}
		var exists bool
		source, exists = store.Record(job.TenantID, current.MessageID, kind)
		thread, _ = store.Record(job.TenantID, current.ConversationID, core.KindConversations)
		if !exists || (!ops && thread.ID == "") || !aiOptIn(store, job.TenantID, flag) ||
			(sales && salesReviewBlocked(store, source, thread)) || (ops && (source.Status == core.StatusResolved || source.RiskReason != current.Name)) {
			current.Enabled, current.Status = false, "cancelled"
			store.PutRecord(current)
			return nil
		}
		settings, _ = tenantSettings(store, job.TenantID)
		if !ops {
			threadMessages = store.ConversationMessages(job.TenantID, thread.ID)
		}
		lease := now.Add(aiJobLease)
		current.Status, current.EmailRetryAt = "running", &lease
		current.Sequence++
		store.PutRecord(current)
		job, claimed = current, true
		return nil
	})
	if err != nil {
		slog.Error("Conversation review claim failed", "job", job.ID, "error", err)
	}
	if err != nil || !claimed {
		return false
	}
	result, status, cause := o.reviewConversation(job, source, threadMessages, settings)
	err = o.repo.UpdateTenant(job.TenantID, func(store core.Writer) error {
		current, ok := store.LockWorkflowJob(job.TenantID, job.ID)
		if !ok || current.Sequence != job.Sequence || current.Status != "running" {
			return nil
		}
		now := o.clock.Now().UTC()
		current.Enabled, current.Status, current.UpdatedAt = false, status, now
		current.EmailRetryAt = nil
		if cause != nil && current.Sequence < maxAIJobAttempts {
			retry := now.Add(time.Minute * time.Duration(current.Sequence))
			current.Enabled, current.Status, current.EmailRetryAt = true, "pending", &retry
		}
		if errors.Is(cause, errAIQuota) || errors.Is(cause, errBillingQuota) {
			retry := reportDay(now).AddDate(0, 0, 1)
			if errors.Is(cause, errBillingQuota) {
				local := now.In(ReportZone)
				retry = time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, ReportZone).AddDate(0, 1, 0)
			}
			current.Enabled, current.Status, current.EmailRetryAt = true, "quota_wait", &retry
			current.Sequence--
		}
		fresh, _ := store.Record(job.TenantID, source.ID, kind)
		thread, _ = store.Record(job.TenantID, thread.ID, core.KindConversations)
		if !aiOptIn(store, job.TenantID, flag) || fresh.ID == "" || (!ops && thread.ID == "") ||
			(sales && salesReviewBlocked(store, fresh, thread)) || (ops && (fresh.Status == core.StatusResolved || fresh.RiskReason != current.Name)) {
			current.Enabled, current.Status = false, "cancelled"
			current.EmailRetryAt = nil
			result = ConversationReview{}
		}
		payload, err := json.Marshal(result)
		if err != nil {
			return err
		}
		current.Content = string(payload)
		store.PutRecord(current)
		if !current.Enabled && len(result.Labels) > 0 && current.Status != "cancelled" {
			store.(*workspaceWriter).notify(thread, "multimodal_qa", "Phát hiện rủi ro trong hội thoại")
		}
		if ops && current.Status == "completed" && result.Advice != "" {
			store.(*workspaceWriter).notify(fresh, "ai_ops_sla", "AI Ops · "+clipped(result.Advice, 200))
		}
		return nil
	})
	if err != nil {
		slog.Error("Conversation review result write failed", "job", job.ID, "error", err)
	}
	return true
}

func salesReviewBlocked(store core.Reader, lead, thread core.Record) bool {
	_, optedOut := store.Record(lead.TenantID, campaignOptOutID(lead), "campaign_opt_outs")
	campaign, active := store.Record(lead.TenantID, lead.AutomationID, leadCampaignKind)
	return optedOut || thread.Status == core.StatusClosed || !active || !campaign.Enabled
}

type MultimodalCoverage struct {
	Statuses   map[string]int    `json:"statuses"`
	Modalities map[string]int    `json:"modalities"`
	Risks      map[string]int    `json:"risks"`
	Alerts     []MultimodalAlert `json:"alerts"`
}

type MultimodalAlert struct {
	ConversationID string        `json:"conversation_id"`
	MessageID      string        `json:"message_id"`
	At             time.Time     `json:"at"`
	Labels         []QARiskLabel `json:"labels"`
}

func multimodalCoverage(store core.Reader, tenant string, from, to time.Time) MultimodalCoverage {
	report := MultimodalCoverage{Statuses: map[string]int{}, Modalities: map[string]int{}, Risks: map[string]int{}, Alerts: []MultimodalAlert{}}
	for _, job := range store.Records(tenant, workflowJobsKind) {
		if job.Subject != core.WorkflowMultimodalQA || job.CreatedAt.Before(from) || !job.CreatedAt.Before(to) {
			continue
		}
		report.Statuses[job.Status]++
		if job.Status != "completed" && job.Status != "partial" {
			continue
		}
		var result ConversationReview
		if json.Unmarshal([]byte(job.Content), &result) != nil {
			continue
		}
		report.Modalities[result.Modality]++
		for _, label := range result.Labels {
			report.Risks[label.Kind]++
		}
		if len(result.Labels) != 0 {
			report.Alerts = append(report.Alerts, MultimodalAlert{job.ConversationID, job.MessageID, job.UpdatedAt, result.Labels})
		}
	}
	sort.Slice(report.Alerts, func(i, j int) bool {
		if report.Alerts[i].At.Equal(report.Alerts[j].At) {
			return report.Alerts[i].MessageID < report.Alerts[j].MessageID
		}
		return report.Alerts[i].At.After(report.Alerts[j].At)
	})
	report.Alerts = report.Alerts[:min(len(report.Alerts), 30)]
	return report
}

func (o *Operations) reviewConversation(job, source core.Record, messages []core.Record, settings TenantSettings) (ConversationReview, string, error) {
	result := ConversationReview{Modality: "text"}
	if job.Subject == core.WorkflowOpsReview {
		var output struct{ Advice string }
		err := o.aiModel.complete(job.TenantID, "Đóng vai AI Ops. Dựa trên dự báo SLA và SOP, đề xuất một bước xử lý nội bộ để nhân viên duyệt. Không thực hiện hành động. Trả JSON {\"advice\":\"...\"}.",
			map[string]any{"ops_sop": settings.PersonaSOP["ops"], "risk": source.RiskReason, "priority": source.Priority,
				"response_p90": source.ResponseP90, "resolution_p90": source.ResolutionP90}, 350, &output)
		if err != nil {
			return result, "failed", err
		}
		result.Advice = clipped(strings.TrimSpace(output.Advice), 800)
		return result, "completed", nil
	}
	sort.Slice(messages, func(i, j int) bool { return laterMessage(messages[i], messages[j]) })
	contextLines := []agentMessage{}
	sales := job.Subject == core.WorkflowSalesReview
	for _, item := range messages {
		if item.Sender == "note" || item.AutomationID == "lead_campaign_command" || item.ID == source.ID ||
			(!sales && laterMessage(item, source)) {
			continue
		}
		contextLines = append(contextLines, agentMessage{Role: item.Sender, Text: clipped(item.Content, 600)})
		if len(contextLines) == 6 {
			break
		}
	}
	if sales {
		var output struct{ Advice string }
		err := o.aiModel.complete(job.TenantID, "Đóng vai AI Sales nội bộ. Đề xuất một bước tư vấn để nhân viên duyệt theo sales_sop. Khi chưa rõ nhu cầu, đề xuất hỏi làm rõ. Không gửi tin, không dự đoán dữ liệu cá nhân, không tự bịa giá hoặc cam kết. Trả JSON {\"advice\":\"...\"}.",
			map[string]any{"sales_sop": settings.PersonaSOP["sales"], "channel": source.Channel, "intent": source.Name, "context": contextLines}, 400, &output)
		result.Advice = clipped(strings.TrimSpace(output.Advice), 800)
		if err != nil {
			return result, "failed", err
		}
		return result, "completed", nil
	}
	status, image := "completed", ""
	if source.AutomationID == voiceTranscriptMarker {
		result.Modality = "voice_transcript"
	} else if source.AttachmentName != "" || source.AttachmentKey != "" || source.AttachmentData != "" {
		result.Modality = "attachment"
		if source.AttachmentKey != "" {
			if err := o.repo.HydrateAttachment(context.Background(), &source); err != nil {
				return result, "failed", err
			}
		}
		if data, err := base64.StdEncoding.DecodeString(source.AttachmentData); err == nil {
			switch mime := http.DetectContentType(data); mime {
			case "image/png", "image/jpeg":
				image = "data:" + mime + ";base64," + source.AttachmentData
				result.Modality = "image"
			}
		}
		if image == "" {
			status = "partial"
		}
	}
	if image == "" && strings.TrimSpace(source.Content) == "" {
		return result, "unsupported", nil
	}
	var output struct{ Labels []QARiskLabel }
	err := o.aiModel.completeWithImage(job.TenantID, "Kiểm tra rủi ro trong tin nhắn, hình ảnh hoặc bản chép lời. Chỉ gán mỗi loại khi có căn cứ. Không suy diễn cảm xúc từ ngoại hình và không chép thông tin định danh. Trả JSON {\"labels\":[{\"kind\":\"procedure|prohibited_language|complaint|unaddressed_emotion|cancellation|unfulfilled_promise\",\"message\":\"giải thích ngắn\"}]}.",
		map[string]any{"modality": result.Modality, "text": source.Content, "speaker": source.Sender, "context": contextLines, "quality_policy": settings.RealtimeQAPolicy}, image, 550, &output)
	if err != nil {
		return result, "failed", err
	}
	result.Labels = normalizeQARisks(output.Labels)
	return result, status, nil
}
