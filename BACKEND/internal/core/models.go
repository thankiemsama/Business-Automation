package core

import "time"

type User struct {
	RecoveryHashes string `json:"recovery_hashes,omitempty"`
	TOTPSecret     string `json:"totp_secret,omitempty"`
	TOTPStep       int64  `json:"totp_step,omitempty"`
	Disabled       bool   `json:"disabled,omitempty"`
	ID             string `json:"id"`
	TenantID       string `json:"tenant_id"`
	Name           string `json:"name"`
	Email          string `json:"email"`
	Role           string `json:"role"`
	Salt           string `json:"salt"`
	PasswordHash   string `json:"password_hash"`
}

type Session struct {
	CreatedAt time.Time
	UserID    string
	ExpiresAt time.Time
}

type Record struct {
	CustomValues    string     `json:"custom_values,omitempty"`
	Summary         string     `json:"ai_summary,omitempty"`
	SummarySource   string     `json:"ai_summary_source,omitempty"`
	Sentiment       string     `json:"sentiment,omitempty"`
	RiskReason      string     `json:"sla_risk_reason,omitempty"`
	Public          bool       `json:"public,omitempty"`
	RatingComment   string     `json:"rating_comment,omitempty"`
	Tags            []string   `json:"tags,omitempty"`
	ReadAt          *time.Time `json:"read_at,omitempty"`
	AttachmentName  string     `json:"attachment_name,omitempty"`
	AttachmentData  string     `json:"attachment_data,omitempty"`
	ResponseDueAt   *time.Time `json:"response_due_at,omitempty"`
	ResolutionDueAt *time.Time `json:"resolution_due_at,omitempty"`
	EscalatedAt     *time.Time `json:"escalated_at,omitempty"`
	Rating          int        `json:"rating,omitempty"`
	RatedAt         *time.Time `json:"rated_at,omitempty"`
	ResolvedBy      string     `json:"resolved_by,omitempty"`
	RespondedBy     string     `json:"responded_by,omitempty"`
	AssigneeID      string     `json:"assignee_id,omitempty"`
	AuthorID        string     `json:"author_id,omitempty"`
	FirstResponseAt *time.Time `json:"first_response_at,omitempty"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty"`
	Unread          bool       `json:"unread,omitempty"`
	DueSoon         bool       `json:"due_soon,omitempty"`
	Overdue         bool       `json:"overdue,omitempty"`
	ID              string     `json:"id"`
	TenantID        string     `json:"tenant_id"`
	Kind            string     `json:"kind"`
	Name            string     `json:"name,omitempty"`
	Email           string     `json:"email,omitempty"`
	Phone           string     `json:"phone,omitempty"`
	Subject         string     `json:"subject,omitempty"`
	Content         string     `json:"content,omitempty"`
	CustomerID      string     `json:"customer_id,omitempty"`
	ConversationID  string     `json:"conversation_id,omitempty"`
	Status          string     `json:"status,omitempty"`
	AutomationID    string     `json:"automation_id,omitempty"`
	Priority        string     `json:"priority,omitempty"`
	Channel         string     `json:"channel,omitempty"`
	Sequence        uint64     `json:"sequence,omitempty"`
	Sender          string     `json:"sender,omitempty"`
	Keywords        []string   `json:"keywords,omitempty"`
	Slug            string     `json:"slug,omitempty"`
	Enabled         bool       `json:"enabled"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// Reader returns values scoped by tenant for all business records.
type Reader interface {
	UserByEmail(string) (User, bool)
	UserByID(string) (User, bool)
	Users(string) []User
	Tenants() []string
	Session(string) (Session, bool)
	Sessions(string) map[string]Session
	Records(tenantID, kind string) []Record
	Record(tenantID, id, kind string) (Record, bool)
}

type Writer interface {
	Reader
	PutUser(User)
	PutSession(string, Session)
	DeleteSession(string)
	RevokeSessions(string)
	ExpireSessions(time.Time)
	PutRecord(Record)
}

// Repository is a Unit of Work: all writes inside a callback commit or roll back together.
// Callbacks must not escape their transaction or recursively enter the repository.
type Repository interface {
	View(func(Reader) error) error
	Update(func(Writer) error) error
}

type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string              { return e.Message }
func Fail(status int, message string) error { return &Error{status, message} }
