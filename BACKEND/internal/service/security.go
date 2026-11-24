package service

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strings"
)

const (
	randomIDBytes          = 24
	passwordHashIterations = 600000
	legacyHashIterations   = 210000
	passwordHashBytes      = 32
	passwordHashVersion    = "pbkdf2v2."
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

func pbkdf2Hex(password, salt string, iterations int) string {
	b, err := pbkdf2.Key(sha256.New, password, []byte(salt), iterations, passwordHashBytes)
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// Hash emits a versioned value at OWASP-consistent 600k iterations so legacy
// 210k-iteration hashes stay distinguishable and can be upgraded in place.
func (PBKDF2Hasher) Hash(p, s string) string {
	return passwordHashVersion + pbkdf2Hex(p, s, passwordHashIterations)
}

// hashVerifier is implemented by hashers that can transparently verify and
// upgrade legacy stored values; plain test hashers skip the upgrade path.
type hashVerifier interface {
	Verify(password, salt, stored string) (match bool, upgraded string)
}

func (PBKDF2Hasher) Verify(password, salt, stored string) (bool, string) {
	if stored == "" {
		return false, stored
	}
	iterations := passwordHashIterations
	raw := stored
	if !strings.HasPrefix(raw, passwordHashVersion) {
		// Legacy bare 64-hex value computed with the original 210k iterations.
		iterations = legacyHashIterations
	} else {
		raw = strings.TrimPrefix(raw, passwordHashVersion)
	}
	if pbkdf2Hex(password, salt, iterations) == raw {
		return true, passwordHashVersion + pbkdf2Hex(password, salt, passwordHashIterations)
	}
	return false, stored
}

// verifyStoredHash checks a password against a stored hash, returning the
// upgraded value when a legacy hash just matched (so the caller can persist it).
func verifyStoredHash(hasher PasswordHasher, password, salt, stored string) (bool, string) {
	if verifier, ok := hasher.(hashVerifier); ok {
		return verifier.Verify(password, salt, stored)
	}
	return subtle.ConstantTimeCompare([]byte(hasher.Hash(password, salt)), []byte(stored)) == 1, stored
}

func tokenHash(t string) string { b := sha256.Sum256([]byte(t)); return hex.EncodeToString(b[:]) }
