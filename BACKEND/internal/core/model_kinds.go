package core

import "time"

// Domain projections can migrate independently of the legacy Record format.
// ---- Customer -------------------------------------------------------------

type Customer struct {
	ID        string   `json:"id"`
	TenantID  string   `json:"tenant_id"`
	Name      string   `json:"name"`
	Email     string   `json:"email,omitempty"`
	Phone     string   `json:"phone,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	Status    string   `json:"status,omitempty"`
	UpdatedAt int64    `json:"updated_at,omitempty"`
}

// ---- Conversation ----------------------------------------------------------

type Conversation struct {
	ID            string   `json:"id"`
	TenantID      string   `json:"tenant_id"`
	CustomerID    string   `json:"customer_id"`
	Channel       string   `json:"channel"`
	Status        string   `json:"status"`
	Subject       string   `json:"subject,omitempty"`
	Email         string   `json:"email,omitempty"`
	AssigneeID    string   `json:"assignee_id,omitempty"`
	Tags          []string `json:"tags,omitempty"`
	Summary       string   `json:"ai_summary,omitempty"`
	SummarySource string   `json:"ai_summary_source,omitempty"`
	Unread        bool     `json:"unread,omitempty"`
	Rating        int      `json:"rating,omitempty"`
	RatedAt       int64    `json:"rated_at,omitempty"`
	CreatedAt     int64    `json:"created_at"`
	UpdatedAt     int64    `json:"updated_at"`
}

// ---- Ticket ----------------------------------------------------------------

type Ticket struct {
	ID              string   `json:"id"`
	TenantID        string   `json:"tenant_id"`
	CustomerID      string   `json:"customer_id"`
	ConversationID  string   `json:"conversation_id"`
	Subject         string   `json:"subject"`
	Content         string   `json:"content,omitempty"`
	Status          string   `json:"status"`
	Priority        string   `json:"priority"`
	Tags            []string `json:"tags,omitempty"`
	AssigneeID      string   `json:"assignee_id,omitempty"`
	ResponseDueAt   int64    `json:"response_due_at,omitempty"`
	ResolutionDueAt int64    `json:"resolution_due_at,omitempty"`
	FirstResponseAt int64    `json:"first_response_at,omitempty"`
	ResolvedAt      int64    `json:"resolved_at,omitempty"`
	ResolvedBy      string   `json:"resolved_by,omitempty"`
	EscalatedAt     int64    `json:"escalated_at,omitempty"`
	Rating          int      `json:"rating,omitempty"`
	RatedAt         int64    `json:"rated_at,omitempty"`
	RatingComment   string   `json:"rating_comment,omitempty"`
	Summary         string   `json:"ai_summary,omitempty"`
	CreatedAt       int64    `json:"created_at"`
	UpdatedAt       int64    `json:"updated_at"`
}

// ---- Message ----------------------------------------------------------------

type Message struct {
	ID             string `json:"id"`
	TenantID       string `json:"tenant_id"`
	ConversationID string `json:"conversation_id"`
	CustomerID     string `json:"customer_id,omitempty"`
	Content        string `json:"content"`
	Sender         string `json:"sender"`
	AuthorID       string `json:"author_id,omitempty"`
	Sequence       uint64 `json:"sequence,omitempty"`
	AttachmentName string `json:"attachment_name,omitempty"`
	AttachmentData string `json:"attachment_data,omitempty"`
	Email          string `json:"email,omitempty"`
	Status         string `json:"status,omitempty"`
	Sentiment      string `json:"sentiment,omitempty"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
}

// ---- Knowledge Article -------------------------------------------------------

type KnowledgeArticle struct {
	ID        string `json:"id"`
	TenantID  string `json:"tenant_id"`
	Subject   string `json:"subject"`
	Content   string `json:"content"`
	Slug      string `json:"slug,omitempty"`
	Public    bool   `json:"public,omitempty"`
	Enabled   bool   `json:"enabled"`
	Version   uint64 `json:"version,omitempty"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// ---- Webhook Subscription ----------------------------------------------------

type WebhookSubscription struct {
	ID        string   `json:"id"`
	TenantID  string   `json:"tenant_id"`
	Name      string   `json:"name"`
	URL       string   `json:"url"`
	Events    []string `json:"events"`
	Secret    string   `json:"secret,omitempty"`
	Enabled   bool     `json:"enabled"`
	CreatedAt int64    `json:"created_at"`
	UpdatedAt int64    `json:"updated_at"`
}

// ---- Channel Configuration ---------------------------------------------------

type ChannelConfig struct {
	ID, TenantID, Channel string
	Name, Content         string
	Enabled               bool
	UpdatedAt             time.Time
}

func ChannelConfigFromRecord(record Record) ChannelConfig {
	return ChannelConfig{ID: record.ID, TenantID: record.TenantID, Channel: record.Channel,
		Name: record.Name, Content: record.Content, Enabled: record.Enabled, UpdatedAt: record.UpdatedAt}
}

func (c ChannelConfig) ApplyTo(record Record) Record {
	record.ID, record.TenantID, record.Kind = c.ID, c.TenantID, KindChannelConfig
	record.Channel, record.Name, record.Content = c.Channel, c.Name, c.Content
	record.Enabled, record.UpdatedAt = c.Enabled, c.UpdatedAt
	return record
}

const KindSurveyEmailJobs = "csat_mail"

type SurveyEmailJob struct {
	ID, TenantID, TicketID, CustomerID string
	Recipient, SurveyURL               string
	Pending                            bool
	Attempts                           uint64
	CreatedAt, NextAttemptAt           time.Time
}

func SurveyEmailJobFromRecord(record Record) SurveyEmailJob {
	return SurveyEmailJob{ID: record.ID, TenantID: record.TenantID, TicketID: record.ConversationID,
		CustomerID: record.CustomerID, Recipient: record.Email, SurveyURL: record.Content,
		Pending: record.Enabled, Attempts: record.Sequence, CreatedAt: record.CreatedAt, NextAttemptAt: record.UpdatedAt}
}

func (j SurveyEmailJob) ApplyTo(record Record) Record {
	record.ID, record.TenantID, record.Kind = j.ID, j.TenantID, KindSurveyEmailJobs
	record.ConversationID, record.CustomerID = j.TicketID, j.CustomerID
	record.Email, record.Content = j.Recipient, j.SurveyURL
	record.Enabled, record.Sequence = j.Pending, j.Attempts
	record.CreatedAt, record.UpdatedAt = j.CreatedAt, j.NextAttemptAt
	return record
}

// ---- API Key -----------------------------------------------------------------

type APIKeyRecord struct {
	ID         string   `json:"id"`
	TenantID   string   `json:"tenant_id"`
	AuthorID   string   `json:"author_id"`
	Name       string   `json:"name"`
	Prefix     string   `json:"content"` // stores the "nexa_<prefix>" display
	Scopes     []string `json:"keywords"`
	RatePerMin uint64   `json:"sequence"`
	Enabled    bool     `json:"enabled"`
	CreatedAt  int64    `json:"created_at"`
	UpdatedAt  int64    `json:"updated_at"`
}

// ---- CSAT Link ----------------------------------------------------------------

type CSATLinkRecord struct {
	ID             string `json:"id"`
	TenantID       string `json:"tenant_id"`
	ConversationID string `json:"conversation_id"`
	Subject        string `json:"subject,omitempty"`
	Email          string `json:"email,omitempty"`
	Enabled        bool   `json:"enabled"`
	ExpiresAt      int64  `json:"updated_at"`
}

// ---- Portal Code / Session ------------------------------------------------------

type PortalCodeRecord struct {
	ID         string `json:"id"`
	TenantID   string `json:"tenant_id"`
	CustomerID string `json:"customer_id"`
	Email      string `json:"email"`
	Enabled    bool   `json:"enabled"`
	CreatedAt  int64  `json:"created_at"`
	UpdatedAt  int64  `json:"updated_at"`
}
