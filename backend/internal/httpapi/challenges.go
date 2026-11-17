package httpapi

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sync"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const challengeTTL = 5 * time.Minute

type challengeKind string

const (
	challengeLogin    challengeKind = "login"
	challengeRegister challengeKind = "register"
)

type challengeEntry struct {
	Kind        challengeKind
	UserID      bson.ObjectID
	ClinicID    bson.ObjectID
	RoleID      bson.ObjectID
	Email       string
	DisplayName string
	Data        webauthn.SessionData
	ExpiresAt   time.Time
}

// challengeStore holds short-lived, single-use WebAuthn challenges in memory.
type challengeStore struct {
	mu      sync.Mutex
	entries map[string]challengeEntry
	now     func() time.Time
}

// newChallengeStore returns an empty challengeStore.
func newChallengeStore() *challengeStore {
	return &challengeStore{
		entries: make(map[string]challengeEntry),
		now:     time.Now,
	}
}

// put stores entry with a 5-minute TTL and returns a single-use challenge ID.
func (s *challengeStore) put(entry challengeEntry) (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate challenge id: %w", err)
	}
	id := base64.RawURLEncoding.EncodeToString(raw)

	now := s.now()
	entry.ExpiresAt = now.Add(challengeTTL)

	s.mu.Lock()
	defer s.mu.Unlock()
	for k, existing := range s.entries {
		if !existing.ExpiresAt.After(now) {
			delete(s.entries, k)
		}
	}
	s.entries[id] = entry
	return id, nil
}

// take atomically retrieves and removes a non-expired challenge entry.
func (s *challengeStore) take(id string, expected challengeKind) (challengeEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.entries[id]
	if !ok {
		return challengeEntry{}, false
	}
	delete(s.entries, id)

	if entry.Kind != expected || !entry.ExpiresAt.After(s.now()) {
		return challengeEntry{}, false
	}
	return entry, true
}
