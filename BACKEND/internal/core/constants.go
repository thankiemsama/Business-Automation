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

	ChannelWeb   = "web"
	ChannelEmail = "email"
	ChannelPhone = "phone"

	SessionLifetime = 24 * time.Hour
)
