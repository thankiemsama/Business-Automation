package api

import (
	"net/http"
	"time"

	"businessautomation/internal/core"
)

// ServerFields accepts legacy metadata but never forwards it into domain commands.
type ServerFields struct {
	ID        string    `json:"id"`
	TenantID  string    `json:"tenant_id"`
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CustomerInput struct {
	Tags []string `json:"tags"`
	ServerFields
	Name  string `json:"name"`
	Email string `json:"email"`
	Phone string `json:"phone"`
}

func (v CustomerInput) Record() core.Record {
	return core.Record{Name: v.Name, Email: v.Email, Phone: v.Phone, Tags: v.Tags}
}

type ConversationInput struct {
	Tags []string `json:"tags"`
	ServerFields
	CustomerID string `json:"customer_id"`
	Channel    string `json:"channel"`
	Status     string `json:"status"`
	Subject    string `json:"subject"`
}

func (v ConversationInput) Record() core.Record {
	return core.Record{Tags: v.Tags, CustomerID: v.CustomerID, Channel: v.Channel, Status: v.Status, Subject: v.Subject}
}

type TicketInput struct {
	Tags []string `json:"tags"`
	ServerFields
	Name           string `json:"name"`
	Subject        string `json:"subject"`
	Content        string `json:"content"`
	CustomerID     string `json:"customer_id"`
	ConversationID string `json:"conversation_id"`
	Status         string `json:"status"`
	Priority       string `json:"priority"`
}

func (v TicketInput) Record() core.Record {
	return core.Record{
		Tags:           v.Tags,
		Name:           v.Name,
		Subject:        v.Subject,
		Content:        v.Content,
		CustomerID:     v.CustomerID,
		ConversationID: v.ConversationID,
		Status:         v.Status,
		Priority:       v.Priority,
	}
}

type KnowledgeInput struct {
	Public bool `json:"public"`
	ServerFields
	Subject string `json:"subject"`
	Content string `json:"content"`
	Enabled bool   `json:"enabled"`
}

func (v KnowledgeInput) Record() core.Record {
	return core.Record{Subject: v.Subject, Content: v.Content, Enabled: v.Enabled, Public: v.Public}
}

type AutomationInput struct {
	ServerFields
	Name     string   `json:"name"`
	Keywords []string `json:"keywords"`
	Enabled  bool     `json:"enabled"`
}

func (v AutomationInput) Record() core.Record {
	return core.Record{Name: v.Name, Keywords: v.Keywords, Enabled: v.Enabled}
}

type recordInput interface{ Record() core.Record }
type resourceDecoder func(http.ResponseWriter, *http.Request) (core.Record, error)

func decodeInput[T recordInput](w http.ResponseWriter, r *http.Request) (core.Record, error) {
	var input T
	if err := decode(w, r, &input); err != nil {
		return core.Record{}, err
	}
	return input.Record(), nil
}

func resourceDecoders() map[string]resourceDecoder {
	return map[string]resourceDecoder{
		core.KindCustomers:     decodeInput[CustomerInput],
		core.KindConversations: decodeInput[ConversationInput],
		core.KindTickets:       decodeInput[TicketInput],
		core.KindKnowledge:     decodeInput[KnowledgeInput],
		"templates":            decodeInput[KnowledgeInput],
		core.KindAutomations:   decodeInput[AutomationInput],
	}
}

// UserResponse deliberately omits all authentication secrets.
type UserResponse struct {
	Disabled bool   `json:"disabled"`
	ID       string `json:"id"`
	TenantID string `json:"tenant_id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

func publicUser(u core.User) UserResponse {
	return UserResponse{Disabled: u.Disabled, ID: u.ID, TenantID: u.TenantID, Name: u.Name, Email: u.Email, Role: u.Role}
}
