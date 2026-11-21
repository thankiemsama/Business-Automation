package service

import (
	"encoding/json"
	"strings"
	"time"

	"businessautomation/internal/core"
)

const ticketAgentPlansKind = "ai_agent_plans"
const ticketAgentPlanTTL = 15 * time.Minute
const ticketActionRule = `Chỉ chọn resolve_ticket khi khách xác nhận vấn đề đã xử lý và không còn việc chờ.
Chọn send_csat khi phiếu đã giải quyết và đủ điều kiện khảo sát. Nếu có nghi ngờ, chọn handoff.`
const ticketAgentCRMPrompt = `Chọn công cụ đọc CRM nếu cần ngữ cảnh lịch sử hỗ trợ.
Trả JSON tool là "read_crm" hoặc "none". Không truy vấn khách hàng khác.`
const ticketAgentPlannerPrompt = `Đề xuất một công cụ trong Allowed cho phiếu đã cho.
Trả JSON action và reason (tiếng Việt, tối đa 300 ký tự). Không thực hiện hành động hoặc bịa thông tin trong lý do.
` + ticketActionRule

func agentPlannerPolicyID() string {
	return tokenHash(ticketAgentCRMPrompt + "\x00" + ticketAgentPlannerPrompt)
}

func planAgentAction(model *OpenAIKnowledge, tenant string, input ticketAgentContext) (TicketAgentPlan, error) {
	var result TicketAgentPlan
	if err := model.complete(tenant, ticketAgentPlannerPrompt, input, 250, &result); err != nil {
		return TicketAgentPlan{}, err
	}
	result.Reason = strings.TrimSpace(result.Reason)
	if !contains(input.Allowed, result.Action) || result.Reason == "" || len([]rune(result.Reason)) > 300 {
		return TicketAgentPlan{}, core.Fail(502, "Đề xuất của agent không hợp lệ")
	}
	return result, nil
}

type TicketAgentPlan struct {
	ID        string    `json:"id"`
	Action    string    `json:"action"`
	Reason    string    `json:"reason"`
	Status    string    `json:"status"`
	ExpiresAt time.Time `json:"expires_at"`
}

type ticketAgentContext struct {
	Subject, Description, Status, Priority string
	ConversationID, MessageID              string `json:"-"`
	CRM                                    map[string]int
	Messages                               []agentMessage
	Allowed                                []string
}

type agentMessage struct{ Role, Text string }

func aiOptIn(store core.Reader, tenant, flag string) bool {
	settings, err := tenantSettings(store, tenant)
	ai, configured := settings.Flags["ai"]
	return err == nil && settings.Flags[flag] && (!configured || ai)
}

func (s *AIService) ticketAgentContext(store core.Reader, ticket core.Record) ticketAgentContext {
	context := ticketAgentContext{
		Subject: clipped(ticket.Subject, 400), Description: clipped(ticket.Content, 2500),
		Status: ticket.Status, Priority: ticket.Priority, ConversationID: ticket.ConversationID,
		Allowed: []string{"handoff"},
	}
	if ticket.ConversationID != "" {
		messages := transcript(store, ticket.TenantID, ticket.ConversationID)
		for _, msg := range messages[max(0, len(messages)-16):] {
			context.Messages = append(context.Messages, agentMessage{msg.Sender, clipped(msg.Content, 400)})
			context.MessageID = msg.ID
		}
	}
	if ticket.Status != core.StatusResolved && context.MessageID != "" &&
		s.model != nil && sparringPermitsResolution(store, ticket.TenantID, s.model.model, s.clock.Now()) {
		context.Allowed = append(context.Allowed, "resolve_ticket")
	}
	if ticket.Status == core.StatusResolved && ticket.Rating == 0 && s.channels != nil {
		thread, ok := store.Record(ticket.TenantID, ticket.ConversationID, core.KindConversations)
		_, surveyed := store.Record(ticket.TenantID, tokenHash("channel-survey:"+thread.ID), "channel_surveys")
		if ok && isLiveChannel(thread.Channel) && thread.Rating == 0 && !surveyed && s.canSendAgentSurvey(store, thread) {
			context.Allowed = append(context.Allowed, "send_csat")
		}
	}
	return context
}

func (s *AIService) canSendAgentSurvey(store core.Reader, thread core.Record) bool {
	var latest time.Time
	for _, msg := range store.ConversationMessages(thread.TenantID, thread.ID) {
		if msg.Sender == core.SenderCustomer && msg.CreatedAt.After(latest) {
			latest = msg.CreatedAt
		}
	}
	_, optedOut := store.Record(thread.TenantID, campaignOptOutID(thread), "campaign_opt_outs")
	return !optedOut && !latest.IsZero() && s.clock.Now().Before(latest.Add(23*time.Hour))
}

func agentBoundState(ticket core.Record, input ticketAgentContext) string {
	return tokenHash(ticket.CustomerID + "\x00" + ticket.AssigneeID + "\x00" +
		ticket.Subject + "\x00" + ticket.Content + "\x00" + ticket.Status + "\x00" +
		ticket.Priority + "\x00" + input.ConversationID + "\x00" + input.MessageID)
}

func (s *AIService) TicketAgentEnabled(user core.User) bool {
	return s.TicketAgentStatus(user)["enabled"]
}

func (s *AIService) TicketAgentStatus(user core.User) map[string]bool {
	status := map[string]bool{"enabled": false, "can_resolve": false}
	if s.Enabled() {
		_ = s.repo.ViewTenant(user.TenantID, func(store core.Reader) error {
			status["enabled"] = aiOptIn(store, user.TenantID, "ai_ticket_agent")
			status["can_resolve"] = status["enabled"] && sparringPermitsResolution(store, user.TenantID, s.model.model, s.clock.Now())
			return nil
		})
	}
	return status
}

func (s *AIService) PlanTicketAction(user core.User, id string) (TicketAgentPlan, error) {
	result := TicketAgentPlan{}
	run := s.repo.IDs.NewID()
	if !s.TicketAgentEnabled(user) {
		return result, core.Fail(403, "Doanh nghiệp chưa bật agent xử lý phiếu")
	}
	var ticket core.Record
	var input ticketAgentContext
	if err := s.repo.ViewTenant(user.TenantID, func(store core.Reader) error {
		var err error
		ticket, err = findOwnedRecord(store, user, id, core.KindTickets)
		if err == nil {
			input = s.ticketAgentContext(store, ticket)
		}
		return err
	}); err != nil {
		return result, err
	}
	if ticket.CustomerID != "" {
		var lookup struct{ Tool string }
		err := s.model.complete(user.TenantID, ticketAgentCRMPrompt, input, 100, &lookup)
		if err != nil {
			return result, err
		}
		if lookup.Tool == "read_crm" {
			err := s.repo.ViewTenant(user.TenantID, func(store core.Reader) error {
				current, err := findOwnedRecord(store, user, id, core.KindTickets)
				if err != nil || current.CustomerID != ticket.CustomerID {
					return core.Fail(409, "Hồ sơ gắn với phiếu đã thay đổi")
				}
				input.CRM = map[string]int{"open_tickets": 0, "resolved_tickets": 0}
				for _, item := range store.Records(user.TenantID, core.KindTickets) {
					if item.CustomerID == current.CustomerID && item.ID != current.ID {
						if item.Status == core.StatusResolved {
							input.CRM["resolved_tickets"]++
						} else {
							input.CRM["open_tickets"]++
						}
					}
				}
				return nil
			})
			outcome := "completed"
			if err != nil {
				outcome = "failed"
			}
			s.appendAgentToolTrace(user.TenantID, run, "read_crm", outcome)
			if err != nil {
				return result, err
			}
		}
	}
	result, err := planAgentAction(s.model, user.TenantID, input)
	if err != nil {
		return TicketAgentPlan{}, err
	}
	now := s.clock.Now().UTC()
	result.ID, result.Status, result.ExpiresAt = run, "proposed", now.Add(ticketAgentPlanTTL)
	if result.Action == "handoff" {
		result.Status = "handed_off"
	}
	err = updateAsTenant(s.repo, user, func(store core.Writer) error {
		if !aiOptIn(store, user.TenantID, "ai_ticket_agent") {
			return core.Fail(403, "Agent đã tắt cho doanh nghiệp")
		}
		if !GovernanceAllows(store, user.TenantID, "ticket_agent") {
			return core.Fail(403, "Agent đã bị dừng bởi governance")
		}
		fresh, err := findOwnedRecord(store, user, id, core.KindTickets)
		if err != nil {
			return err
		}
		state := agentBoundState(ticket, input)
		freshInput := s.ticketAgentContext(store, fresh)
		if state != agentBoundState(fresh, freshInput) || !contains(freshInput.Allowed, result.Action) {
			return core.Fail(409, "Phiếu đã thay đổi trong lúc agent phân tích")
		}
		data, err := json.Marshal(result)
		if err != nil {
			return err
		}
		store.PutRecord(core.Record{
			ID: result.ID, TenantID: user.TenantID, Kind: ticketAgentPlansKind, Name: result.Action, CustomerID: ticket.CustomerID,
			ConversationID: ticket.ID, AuthorID: user.ID, Subject: state, Content: string(data), Status: result.Status,
			Slug: agentPlannerPolicyID(), AutomationID: s.model.model,
			Enabled: input.CRM != nil, CreatedAt: now, UpdatedAt: now,
		})
		return writeAgentToolTrace(store, user.TenantID, run, result.Action, result.Status, s.model.model, now)
	})
	return result, err
}

func (s *AIService) ApplyTicketAction(user core.User, ticketID, planID string) (TicketAgentPlan, error) {
	result := TicketAgentPlan{}
	failedTool := ""
	err := updateAsTenant(s.repo, user, func(store core.Writer) error {
		if !aiOptIn(store, user.TenantID, "ai_ticket_agent") {
			return core.Fail(403, "Agent đã tắt cho doanh nghiệp")
		}
		if !GovernanceAllows(store, user.TenantID, "ticket_agent") {
			return core.Fail(403, "Agent đã bị dừng bởi governance")
		}
		plan, found := store.Record(user.TenantID, planID, ticketAgentPlansKind)
		if !found || plan.ConversationID != ticketID || plan.AuthorID != user.ID {
			return core.Fail(404, "Không tìm thấy đề xuất của agent")
		}
		if err := json.Unmarshal([]byte(plan.Content), &result); err != nil {
			return err
		}
		if plan.Status == "applied" {
			result.Status = plan.Status
			return nil
		}
		now := s.clock.Now().UTC()
		if plan.Status != "proposed" || plan.Slug != agentPlannerPolicyID() ||
			s.model == nil || plan.AutomationID != s.model.model || !now.Before(result.ExpiresAt) {
			return core.Fail(409, "Đề xuất đã hết hiệu lực")
		}
		ticket, err := findOwnedRecord(store, user, ticketID, core.KindTickets)
		if err != nil {
			return err
		}
		input := s.ticketAgentContext(store, ticket)
		if plan.Name == "resolve_ticket" && (s.model == nil || !sparringPermitsResolution(store, user.TenantID, s.model.model, now)) {
			return core.Fail(403, "AI agent cần chứng chỉ Sparring còn hiệu lực để giải quyết phiếu")
		}
		if plan.Subject != agentBoundState(ticket, input) || !contains(input.Allowed, plan.Name) {
			return core.Fail(409, "Trạng thái phiếu đã đổi. Hãy tạo đề xuất mới")
		}
		switch plan.Name {
		case "resolve_ticket":
			ticket.Status, ticket.UpdatedAt, ticket.ResolvedAt, ticket.ResolvedBy = core.StatusResolved, now, &now, user.ID
			store.PutRecord(ticket)
		case "send_csat":
			thread, _ := store.Record(user.TenantID, ticket.ConversationID, core.KindConversations)
			if err := s.channels.queueChannelSurvey(store, thread); err != nil {
				failedTool = plan.Name
				return err
			}
		default:
			return core.Fail(400, "Hành động không được hỗ trợ")
		}
		result.Status, plan.Status, plan.UpdatedAt = "applied", "applied", now
		store.PutRecord(plan)
		model := ""
		if s.model != nil {
			model = s.model.model
		}
		return writeAgentToolTrace(store, user.TenantID, planID, plan.Name, "applied", model, now)
	})
	if err != nil && failedTool != "" {
		s.appendAgentToolTrace(user.TenantID, planID, failedTool, "failed")
	}
	return result, err
}

func resolveOnCustomerConfirmation(store core.Writer, conversation, previous, message core.Record) bool {
	tenant := conversation.TenantID
	if !aiOptIn(store, tenant, "ai_ticket_agent") || !aiOptIn(store, tenant, "ai_ticket_agent_auto_resolve") ||
		!GovernanceAllows(store, tenant, "ticket_agent") ||
		previous.Sender != core.SenderAgent || previous.Status == "pending" || previous.Status == "failed" ||
		message.Sender != core.SenderCustomer ||
		normalizeDiacritics(strings.Trim(strings.TrimSpace(message.Content), ".!")) != "da giai quyet" {
		return false
	}
	var ticket core.Record
	for _, candidate := range store.Records(tenant, core.KindTickets) {
		if candidate.ConversationID != conversation.ID || candidate.Status == core.StatusResolved {
			continue
		}
		if ticket.ID != "" || candidate.Priority == core.PriorityUrgent || candidate.CustomerID != conversation.CustomerID {
			return false
		}
		ticket = candidate
	}
	if ticket.ID == "" || ticket.CustomerID == "" {
		return false
	}
	now := message.CreatedAt.UTC()
	model := ""
	if writer, ok := store.(*workspaceWriter); ok {
		model = writer.workspace.sparringModel
	}
	if !sparringPermitsResolution(store, tenant, model, now) {
		return false
	}
	ticket.Status, ticket.UpdatedAt, ticket.ResolvedAt = core.StatusResolved, now, &now
	store.PutRecord(ticket)
	run := tokenHash("agent-confirmed-resolution:" + message.ID)
	store.PutRecord(core.Record{
		ID: run, TenantID: tenant, Kind: ticketAgentPlansKind,
		Name: "resolve_ticket", Status: "auto_applied", ConversationID: ticket.ID, CustomerID: ticket.CustomerID,
		MessageID: message.ID, CreatedAt: now, UpdatedAt: now,
	})
	// Resolution is already safe to apply when an older trace buffer is unreadable.
	_ = writeAgentToolTrace(store, tenant, run, "resolve_ticket", "auto_applied", model, now)
	meterAIResolution(store, tenant, ticket.ID+":"+message.ID+":confirm", "agent_confirm", now)
	return true
}
