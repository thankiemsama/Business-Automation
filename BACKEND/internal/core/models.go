package core

import "time"

type User struct {
	ID           string `json:"id"`
	TenantID     string `json:"tenant_id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	Salt         string `json:"salt"`
	PasswordHash string `json:"password_hash"`
}

type Session struct {
	UserID    string
	ExpiresAt time.Time
}

type Record struct {
	ID             string    `json:"id"`
	TenantID       string    `json:"tenant_id"`
	Kind           string    `json:"kind"`
	Name           string    `json:"name,omitempty"`
	Email          string    `json:"email,omitempty"`
	Phone          string    `json:"phone,omitempty"`
	Subject        string    `json:"subject,omitempty"`
	Content        string    `json:"content,omitempty"`
	CustomerID     string    `json:"customer_id,omitempty"`
	ConversationID string    `json:"conversation_id,omitempty"`
	Status         string    `json:"status,omitempty"`
	AutomationID   string    `json:"automation_id,omitempty"`
	Priority       string    `json:"priority,omitempty"`
	Channel        string    `json:"channel,omitempty"`
	Sequence       uint64    `json:"sequence,omitempty"`
	Sender         string    `json:"sender,omitempty"`
	Keywords       []string  `json:"keywords,omitempty"`
	Enabled        bool      `json:"enabled"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// Reader returns values scoped by tenant for all business records.
type Reader interface {
	UserByEmail(string) (User, bool)
	UserByID(string) (User, bool)
	Session(string) (Session, bool)
	Records(tenantID, kind string) []Record
	Record(tenantID, id, kind string) (Record, bool)
}

type Writer interface {
	Reader
	PutUser(User)
	PutSession(string, Session)
	DeleteSession(string)
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
