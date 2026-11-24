package core

import "time"

const (
	KindCustomers     = "customers"
	KindConversations = "conversations"
	KindTickets       = "tickets"
	KindKnowledge     = "knowledge"
	KindAutomations   = "automations"
	KindMessages      = "messages"

	RoleOwner = "owner"
	RoleAgent = "agent"

	StatusOpen       = "open"
	StatusClosed     = "closed"
	StatusInProgress = "in_progress"
	StatusResolved   = "resolved"

	SenderCustomer  = "customer"
	SenderAgent     = "agent"
	SenderAssistant = "assistant"

	PriorityLow    = "low"
	PriorityNormal = "normal"
	PriorityHigh   = "high"
	PriorityUrgent = "urgent"

	ChannelWeb       = "web"
	ChannelEmail     = "email"
	ChannelPhone     = "phone"
	ChannelZalo      = "zalo"
	ChannelMessenger = "messenger"
	ChannelWhatsApp  = "whatsapp"
	ChannelSMS       = "sms"
	ChannelInstagram = "instagram"
	SessionLifetime  = 24 * time.Hour
	PortalCodeTTL    = 10 * time.Minute
	CSATLinkTTL      = 30 * 24 * time.Hour

	// Record kinds that are operational (not a user resource) but persisted as records.
	KindWebhooks          = "webhooks"
	KindAPIKeys           = "api_keys"
	KindChannelConfig     = "channel_config"
	KindPortalCodes       = "portal_codes"
	KindCSATLinks         = "csat_links"
	KindWebhookDeliveries = "webhook_deliveries"
	KindCustomerNotes     = "customer_notes"
	KindLTV               = "customer_ltv"
	KindKnowledgePublic   = "kb_public"
	// Operational (non-user-resource) kinds used by the auth hardening layer.
	KindAuthEvent = "auth_event"
	KindStepUp    = "step_up"
)

// PublicChannels are the multi-channel integrations a tenant can configure.
var PublicChannels = []string{
	ChannelZalo, ChannelMessenger, ChannelWhatsApp, ChannelSMS, ChannelInstagram,
}

// webhook event names emitted to outgoing integrations.
const (
	EventTicketCreated   = "ticket.created"
	EventMessageNew      = "message.new"
	EventSLABreached     = "sla.breached"
	EventCSATSubmitted   = "csat.submitted"
	EventBillingQuotaLow = "credit_balance.low"
)
