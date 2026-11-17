package service

import "time"

// Consumer-owned ports keep orchestration independent of infrastructure.
type Clock interface{ Now() time.Time }
type IDGenerator interface{ NewID() string }
type PasswordHasher interface {
	Hash(password, salt string) string
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

type Credentials struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}
