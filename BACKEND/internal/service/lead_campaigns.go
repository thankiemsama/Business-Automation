package service

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"businessautomation/internal/core"
	"github.com/skip2/go-qrcode"
)

const leadCampaignKind = "lead_campaigns"
const campaignLeadKind = "campaign_leads"
const leadDeliveryPrefix = "lead-campaign:"

var campaignPhone = regexp.MustCompile(`^[1-9][0-9]{7,14}$`)
var campaignCode = regexp.MustCompile(`^[a-zA-Z0-9_-]{12,80}$`)

type LeadCampaign struct {
	FollowUpTickets bool     `json:"follow_up_tickets,omitempty"`
	Accepted        int      `json:"accepted,omitempty"`
	Replied         int      `json:"replied,omitempty"`
	ID              string   `json:"id,omitempty"`
	Name            string   `json:"name"`
	Channel         string   `json:"channel"`
	Phone           string   `json:"phone"`
	Welcome         string   `json:"welcome"`
	FollowUps       []string `json:"follow_ups"`
	Enabled         bool     `json:"enabled"`
	URL             string   `json:"url,omitempty"`
	Visits          uint64   `json:"visits,omitempty"`
	Leads           int      `json:"leads,omitempty"`
	Engaged         int      `json:"engaged,omitempty"`
}

type LeadLanding struct {
	Name        string `json:"name"`
	Channel     string `json:"channel"`
	Destination string `json:"destination"`
	Message     string `json:"message"`
	FollowUps   int    `json:"follow_ups"`
}

type CampaignLead struct {
	SalesAdvice    string `json:"sales_advice,omitempty"`
	Intent         string `json:"intent,omitempty"`
	ConversationID string `json:"conversation_id"`
	Name           string `json:"name"`
	Score          uint64 `json:"score"`
	Unsubscribed   bool   `json:"unsubscribed"`
}

func (m *ChannelManager) CampaignLeads(user core.User, id string) ([]CampaignLead, error) {
	items := []CampaignLead{}
	if user.Role != core.RoleOwner {
		return items, core.Fail(403, "Chỉ owner xem khách tiềm năng")
	}
	err := viewTenant(m.repo, user.TenantID, func(store core.Reader) error {
		if _, err := findOwnedRecord(store, user, id, leadCampaignKind); err != nil {
			return err
		}
		leads := store.Records(user.TenantID, campaignLeadKind)
		sort.Slice(leads, func(i, j int) bool { return laterMessage(leads[i], leads[j]) })
		seen := map[string]bool{}
		for _, lead := range leads {
			if lead.AutomationID != id || seen[lead.CustomerID] {
				continue
			}
			seen[lead.CustomerID] = true
			customer, _ := store.Record(user.TenantID, lead.CustomerID, core.KindCustomers)
			_, optedOut := store.Record(user.TenantID, campaignOptOutID(lead), "campaign_opt_outs")
			var review ConversationReview
			if !optedOut && aiOptIn(store, user.TenantID, "ai_sales_persona") {
				if job, ok := store.Record(user.TenantID, tokenHash(core.WorkflowSalesReview+":"+user.TenantID+":"+lead.ID), workflowJobsKind); ok && job.Status == "completed" {
					_ = json.Unmarshal([]byte(job.Content), &review)
				}
			}
			items = append(items, CampaignLead{
				Intent: lead.Name, SalesAdvice: review.Advice,
				ConversationID: lead.ConversationID, Name: customer.Name, Score: lead.Sequence, Unsubscribed: optedOut,
			})
			if len(items) == 50 {
				break
			}
		}
		return nil
	})
	return items, err
}

func (m *ChannelManager) ConfigureCampaignOrigin(origin string) {
	parsed, err := url.Parse(origin)
	if err == nil && parsed.Hostname() != "" && parsed.User == nil && parsed.RawQuery == "" && parsed.Fragment == "" &&
		(parsed.Path == "" || parsed.Path == "/") &&
		(parsed.Scheme == "https" || parsed.Scheme == "http" && (parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "localhost")) {
		m.campaignOrigin = strings.TrimRight(origin, "/")
	}
}

func (m *ChannelManager) campaignURL(tenant, id string) string {
	link, _ := url.JoinPath(m.campaignOrigin, "lead", tenant, id)
	return link
}

func (m *ChannelManager) SaveLeadCampaign(user core.User, input LeadCampaign) (LeadCampaign, error) {
	if user.Role != core.RoleOwner {
		return LeadCampaign{}, core.Fail(403, "Chỉ owner quản lý mã chiến dịch")
	}
	if m.campaignOrigin == "" || m.conversations == nil {
		return LeadCampaign{}, core.Fail(503, "Hãy cấu hình APP_ORIGIN để tạo mã chiến dịch")
	}
	input.Name, input.Phone, input.Welcome = strings.TrimSpace(input.Name), strings.TrimSpace(input.Phone), strings.TrimSpace(input.Welcome)
	if (input.Channel != core.ChannelZalo && input.Channel != core.ChannelWhatsApp) || input.Name == "" || len([]rune(input.Name)) > 80 ||
		input.Channel == core.ChannelWhatsApp && !campaignPhone.MatchString(input.Phone) ||
		len([]rune(input.Welcome)) > 500 || input.Welcome == "" || len(input.FollowUps) > 2 {
		return LeadCampaign{}, core.Fail(400, "Tên, kênh, số điện thoại hoặc lời chào không hợp lệ")
	}
	for i, text := range input.FollowUps {
		input.FollowUps[i] = strings.TrimSpace(text)
		if len([]rune(input.FollowUps[i])) > 500 {
			return LeadCampaign{}, core.Fail(400, "Mỗi lời nhắc tối đa 500 ký tự")
		}
	}
	if input.Channel == core.ChannelZalo {
		input.Phone = ""
	}
	input.URL, input.Visits, input.Leads, input.Engaged, input.Accepted, input.Replied = "", 0, 0, 0, 0, 0
	err := updateAsTenant(m.repo, user, func(store core.Writer) error {
		_, account, _, err := m.providerAccount(store, user.TenantID, input.Channel)
		if err != nil && input.Enabled {
			return err
		}
		now := m.conversations.clock.Now().UTC()
		record := core.Record{TenantID: user.TenantID, Kind: leadCampaignKind, CreatedAt: now}
		if input.ID == "" {
			if len(store.Records(user.TenantID, leadCampaignKind)) >= 50 {
				return core.Fail(409, "Tối đa 50 mã chiến dịch")
			}
			input.ID = m.conversations.ids.NewID()
		} else {
			var found bool
			record, found = store.Record(user.TenantID, input.ID, leadCampaignKind)
			if !found {
				return core.Fail(404, "Không tìm thấy mã chiến dịch")
			}
			if record.Channel != input.Channel {
				return core.Fail(409, "Mỗi mã chỉ liên kết với một kênh")
			}
		}
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		record.ID, record.Name, record.Content = input.ID, input.Name, string(data)
		record.Channel, record.Enabled, record.UpdatedAt = input.Channel, input.Enabled, now
		if account.AccountID != "" {
			record.Subject = account.AccountID
		}
		store.PutRecord(record)
		return nil
	})
	input.URL = m.campaignURL(user.TenantID, input.ID)
	return input, err
}

func (m *ChannelManager) LeadCampaigns(user core.User) ([]LeadCampaign, error) {
	items := []LeadCampaign{}
	if user.Role != core.RoleOwner {
		return items, core.Fail(403, "Chỉ owner xem mã chiến dịch")
	}
	err := viewTenant(m.repo, user.TenantID, func(store core.Reader) error {
		scores := map[string]map[string]uint64{}
		accepted, replied := map[string]map[string]bool{}, map[string]map[string]bool{}
		for _, lead := range store.Records(user.TenantID, campaignLeadKind) {
			if scores[lead.AutomationID] == nil {
				scores[lead.AutomationID] = map[string]uint64{}
				accepted[lead.AutomationID] = map[string]bool{}
				replied[lead.AutomationID] = map[string]bool{}
			}
			byCustomer := scores[lead.AutomationID]
			byCustomer[lead.CustomerID] = max(byCustomer[lead.CustomerID], lead.Sequence)
			if lead.FirstResponseAt != nil {
				accepted[lead.AutomationID][lead.CustomerID] = true
			}
			if lead.RatedAt != nil {
				replied[lead.AutomationID][lead.CustomerID] = true
			}
		}
		records := store.Records(user.TenantID, leadCampaignKind)
		sort.Slice(records, func(i, j int) bool { return records[i].CreatedAt.After(records[j].CreatedAt) })
		for _, record := range records {
			var item LeadCampaign
			if err := json.Unmarshal([]byte(record.Content), &item); err != nil {
				return err
			}
			item.Visits = record.Sequence
			item.Leads = len(scores[record.ID])
			item.Accepted, item.Replied = len(accepted[record.ID]), len(replied[record.ID])
			for _, score := range scores[record.ID] {
				if score >= 70 {
					item.Engaged++
				}
			}
			if m.campaignOrigin != "" {
				item.URL = m.campaignURL(user.TenantID, item.ID)
			}
			items = append(items, item)
		}
		return nil
	})
	return items, err
}

func (m *ChannelManager) LeadQRCode(user core.User, id string) (string, error) {
	if user.Role != core.RoleOwner {
		return "", core.Fail(403, "Chỉ owner tải mã QR")
	}
	if m.campaignOrigin == "" {
		return "", core.Fail(503, "Hãy cấu hình APP_ORIGIN để tạo mã QR")
	}
	if err := viewTenant(m.repo, user.TenantID, func(store core.Reader) error {
		_, err := findOwnedRecord(store, user, id, leadCampaignKind)
		return err
	}); err != nil {
		return "", err
	}
	image, err := qrcode.Encode(m.campaignURL(user.TenantID, id), qrcode.Medium, 384)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(image), nil
}

func (m *ChannelManager) OpenLeadCampaign(tenant, id string) (LeadLanding, error) {
	var landing LeadLanding
	unavailable := core.Fail(404, "Mã chiến dịch không khả dụng")
	if !campaignCode.MatchString(id) {
		return landing, unavailable
	}
	err := updateTenant(m.repo, tenant, func(store core.Writer) error {
		record, exists := store.Record(tenant, id, leadCampaignKind)
		var campaign LeadCampaign
		if !exists || !record.Enabled || json.Unmarshal([]byte(record.Content), &campaign) != nil {
			return unavailable
		}
		_, account, _, err := m.providerAccount(store, tenant, record.Channel)
		if err != nil || account.AccountID != record.Subject {
			return unavailable
		}
		landing = LeadLanding{Name: campaign.Name, Channel: record.Channel, Message: "NEXA " + id}
		for _, text := range campaign.FollowUps {
			if text != "" {
				landing.FollowUps++
			}
		}
		switch record.Channel {
		case core.ChannelZalo:
			landing.Destination = "https://zalo.me/" + url.PathEscape(record.Subject)
		case core.ChannelWhatsApp:
			landing.Destination = "https://wa.me/" + url.PathEscape(campaign.Phone)
		default:
			return unavailable
		}
		record.Sequence++
		store.PutRecord(record)
		return nil
	})
	return landing, err
}

func (m *ChannelManager) handleCampaignMessage(store core.Writer, conversation, inbound core.Record, account string) bool {
	now := inbound.CreatedAt
	bindingID := tokenHash("campaign-conversation:" + conversation.ID)
	binding, linked := store.Record(inbound.TenantID, bindingID, "campaign_conversations")
	parts := strings.Fields(inbound.Content)
	isText := inbound.AttachmentName == "" && inbound.AttachmentData == ""
	consentLimit := 0
	if len(parts) == 3 {
		switch strings.ToUpper(parts[2]) {
		case "OK1":
			consentLimit = 1
		case "OK2":
			consentLimit = 2
		}
	}
	isCode := isText && (len(parts) == 2 || consentLimit > 0) &&
		strings.EqualFold(parts[0], "NEXA") && campaignCode.MatchString(parts[1])
	stop := isText && strings.EqualFold(strings.TrimSpace(inbound.Content), "STOP")
	if linked && !isCode {
		if lead, ok := store.Record(inbound.TenantID, binding.AutomationID, campaignLeadKind); ok {
			if stop {
				lead.Status, lead.Enabled = "unsubscribed", false
				store.PutRecord(core.Record{ID: campaignOptOutID(conversation), TenantID: inbound.TenantID, Kind: "campaign_opt_outs", CreatedAt: now})
			} else {
				lead.Sequence = min(100, lead.Sequence+20)
				lead.Enabled = false
				if lead.FirstResponseAt != nil && lead.RatedAt == nil && !now.Before(*lead.FirstResponseAt) {
					lead.RatedAt = &now
				}
				if isText {
					m.recordCampaignIntent(store, &lead, conversation, inbound, now)
				}
			}
			lead.UpdatedAt = now
			store.PutRecord(lead)
			return stop
		}
	}
	if stop {
		store.PutRecord(core.Record{ID: campaignOptOutID(conversation), TenantID: inbound.TenantID, Kind: "campaign_opt_outs", CreatedAt: now})
		return true
	}
	if !isCode || linked {
		return isCode && linked
	}
	record, exists := store.Record(inbound.TenantID, parts[1], leadCampaignKind)
	var campaign LeadCampaign
	if !exists || !record.Enabled || record.Channel != conversation.Channel || record.Subject != account ||
		json.Unmarshal([]byte(record.Content), &campaign) != nil {
		return false
	}
	leadID := tokenHash("lead:" + campaign.ID + ":" + conversation.CustomerID)
	if _, exists := store.Record(inbound.TenantID, leadID, campaignLeadKind); exists {
		return true
	}
	for _, lead := range store.Records(inbound.TenantID, campaignLeadKind) {
		if lead.AutomationID == campaign.ID && lead.CustomerID == conversation.CustomerID {
			return true
		}
	}
	_, optedOut := store.Record(inbound.TenantID, campaignOptOutID(conversation), "campaign_opt_outs")
	optedIn := consentLimit > 0 && !optedOut
	lead := core.Record{
		ID: leadID, TenantID: inbound.TenantID, Kind: campaignLeadKind, AutomationID: campaign.ID,
		ConversationID: conversation.ID, CustomerID: conversation.CustomerID, Channel: conversation.Channel,
		MessageID: inbound.ID, Sequence: 30, Enabled: optedIn, CreatedAt: now, UpdatedAt: now,
	}
	store.PutRecord(lead)
	store.PutRecord(core.Record{ID: bindingID, TenantID: inbound.TenantID, Kind: "campaign_conversations", AutomationID: lead.ID, ConversationID: conversation.ID})
	if !optedOut {
		m.scheduleCampaignMessage(store, conversation, lead, campaign.Welcome, 0, now)
	}
	if optedIn {
		for i, followUp := range campaign.FollowUps {
			if consentLimit == 0 {
				break
			}
			if followUp == "" {
				continue
			}
			consentLimit--
			due := now.Add([]time.Duration{time.Hour, 8 * time.Hour}[i])
			m.scheduleCampaignMessage(store, conversation, lead, followUp+"\n\nGửi STOP để ngừng nhận lời nhắc.", i+1, due)
		}
	}
	return true
}

func (m *ChannelManager) recordCampaignIntent(store core.Writer, lead *core.Record, conversation, inbound core.Record, now time.Time) {
	text := normalizeDiacritics(strings.Trim(strings.TrimSpace(inbound.Content), ".!?"))
	var intent string
	switch text {
	case "can tu van", "toi can tu van", "muon duoc tu van":
		intent = "Tư vấn"
	case "can bao gia", "toi can bao gia", "gui bao gia":
		intent = "Báo giá"
	case "can ho tro", "toi can ho tro", "can nhan vien ho tro":
		intent = "Hỗ trợ"
	default:
		return
	}
	lead.Name = intent
	campaignRecord, found := store.Record(lead.TenantID, lead.AutomationID, leadCampaignKind)
	var campaign LeadCampaign
	if !found || !campaignRecord.Enabled || json.Unmarshal([]byte(campaignRecord.Content), &campaign) != nil || !campaign.FollowUpTickets {
		return
	}
	id := tokenHash("campaign-followup-ticket:" + lead.ID)
	if _, exists := store.Record(lead.TenantID, id, core.KindTickets); exists {
		return
	}
	ticket := core.Record{ID: id, TenantID: lead.TenantID, Kind: core.KindTickets, CustomerID: lead.CustomerID,
		ConversationID: conversation.ID, AutomationID: "campaign_followup", AssigneeID: conversation.AssigneeID,
		Subject: "Theo dõi chiến dịch · " + intent, Content: clipped(inbound.Content, 800),
		Status: core.StatusOpen, Priority: core.PriorityNormal, CreatedAt: now, UpdatedAt: now}
	response, resolution := ticketDeadlines(ticket)
	ticket.ResponseDueAt, ticket.ResolutionDueAt = &response, &resolution
	store.PutRecord(ticket)
}

func campaignOptOutID(conversation core.Record) string {
	return tokenHash("campaign-opt-out:" + conversation.Channel + ":" + conversation.CustomerID)
}

func (m *ChannelManager) scheduleCampaignMessage(store core.Writer, conversation, lead core.Record, text string, step int, due time.Time) {
	thread, ok := store.Record(conversation.TenantID, tokenHash("channel-thread:"+conversation.ID), "channel_threads")
	if !ok {
		return
	}
	id := tokenHash("campaign-send:" + lead.ID + ":" + strconv.Itoa(step))
	if err := m.enqueue(store, conversation.TenantID, conversation.Channel, thread.Content, thread.Name, text, id, conversation.ID, ""); err != nil {
		return
	}
	job, _ := store.Record(conversation.TenantID, id, channelOutbox)
	job.AutomationID, job.EmailRetryAt, job.Sequence = leadDeliveryPrefix+lead.ID, &due, uint64(step)
	store.PutRecord(job)
}

func (m *ChannelManager) prepareCampaignDelivery(store core.Writer, job *core.Record, leadID string, now time.Time) bool {
	lead, found := store.Record(job.TenantID, leadID, campaignLeadKind)
	campaign, active := store.Record(job.TenantID, lead.AutomationID, leadCampaignKind)
	thread, exists := store.Record(job.TenantID, job.ConversationID, core.KindConversations)
	_, optedOut := store.Record(job.TenantID, campaignOptOutID(thread), "campaign_opt_outs")
	if !found || !active || !campaign.Enabled || campaign.Subject != job.Subject || !exists ||
		thread.Channel != job.Channel || thread.Status != core.StatusOpen || optedOut || job.Sequence > 0 && !lead.Enabled {
		job.Status, job.EmailLastError, job.EmailRetryAt = "cancelled", "campaign_inactive", nil
		m.saveDelivery(store, *job, now)
		return false
	}
	deadline := campaignServiceDeadline(job.Channel, lead.CreatedAt)
	if !now.Before(deadline) {
		job.Status, job.EmailLastError, job.EmailRetryAt = "cancelled", "campaign_window_expired", nil
		m.saveDelivery(store, *job, now)
		return false
	}
	if job.Sequence > 0 {
		local := now.In(ReportZone)
		open := reportDay(local).Add(8 * time.Hour)
		if local.Hour() >= 20 {
			open = open.AddDate(0, 0, 1)
		}
		if local.Before(open) {
			if !open.Before(deadline) {
				job.Status, job.EmailLastError, job.EmailRetryAt = "cancelled", "campaign_window_expired", nil
				m.saveDelivery(store, *job, now)
				return false
			}
			job.EmailRetryAt, job.UpdatedAt = &open, now
			store.PutRecord(*job)
			return false
		}
		count := 0
		for _, prior := range store.Records(job.TenantID, channelOutbox) {
			if prior.ID != job.ID && prior.Channel == job.Channel && prior.Subject == job.Subject &&
				prior.Name == job.Name && prior.Sequence > 0 && strings.HasPrefix(prior.AutomationID, leadDeliveryPrefix) &&
				(prior.Status == "sent" || prior.Status == "sending" || prior.EmailLastError == "delivery_unknown") &&
				prior.UpdatedAt.After(now.Add(-24*time.Hour)) {
				count++
			}
		}
		if count >= 2 {
			job.Status, job.EmailLastError, job.EmailRetryAt = "cancelled", "campaign_frequency_cap", nil
			m.saveDelivery(store, *job, now)
			return false
		}
	}
	if job.MessageID == "" {
		message := core.Record{
			ID: m.conversations.ids.NewID(), TenantID: job.TenantID, Kind: core.KindMessages,
			ConversationID: thread.ID, Sender: core.SenderAssistant, Content: job.Content, Status: "pending",
			Sequence: nextSequence(store, core.User{TenantID: job.TenantID}, thread.ID), CreatedAt: now, UpdatedAt: now,
		}
		store.PutRecord(message)
		job.MessageID = message.ID
	}
	return true
}

func campaignServiceDeadline(channel string, interaction time.Time) time.Time {
	switch channel {
	case core.ChannelZalo:
		return interaction.Add(48 * time.Hour)
	case core.ChannelWhatsApp:
		return interaction.Add(24 * time.Hour)
	default:
		return interaction
	}
}
