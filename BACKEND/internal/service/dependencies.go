package service

import "time"

type Clock interface{ Now() time.Time }
type IDGenerator interface{ NewID() string }
type PasswordHasher interface {
	Hash(password, salt string) string
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

type Credentials struct {
	Industry string `json:"industry,omitempty"`
	Code     string `json:"code,omitempty"`
	Token    string `json:"token,omitempty"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}
