package service

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

const (
	randomIDBytes          = 24
	passwordHashIterations = 210000
	passwordHashBytes      = 32
)

type RandomIDs struct{}
type PBKDF2Hasher struct{}

func (RandomIDs) NewID() string {
	b := make([]byte, randomIDBytes)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func (PBKDF2Hasher) Hash(p, s string) string {
	b, err := pbkdf2.Key(sha256.New, p, []byte(s), passwordHashIterations, passwordHashBytes)
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func tokenHash(t string) string { b := sha256.Sum256([]byte(t)); return hex.EncodeToString(b[:]) }
