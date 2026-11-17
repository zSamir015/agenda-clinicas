package httpapi

import (
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestChallengeStoreTakeOwned(t *testing.T) {
	store := newChallengeStore()
	now := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	owner, intruder := bson.NewObjectID(), bson.NewObjectID()
	id, err := store.put(challengeEntry{Kind: challengeRegister, UserID: owner})
	if err != nil {
		t.Fatalf("put: %v", err)
	}

	if _, ok := store.takeOwned(id, challengeRegister, intruder); ok {
		t.Fatal("another user took the challenge")
	}
	if _, ok := store.takeOwned(id, challengeLogin, owner); ok {
		t.Fatal("challenge taken with the wrong kind")
	}
	entry, ok := store.takeOwned(id, challengeRegister, owner)
	if !ok || entry.UserID != owner {
		t.Fatalf("owner could not take the challenge after a failed attempt: ok=%v", ok)
	}
	if _, ok := store.takeOwned(id, challengeRegister, owner); ok {
		t.Fatal("challenge was reusable")
	}
}

func TestChallengeStoreExpiry(t *testing.T) {
	store := newChallengeStore()
	now := time.Date(2026, 11, 17, 10, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	id, err := store.put(challengeEntry{Kind: challengeLogin, UserID: bson.NewObjectID()})
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	now = now.Add(challengeTTL + time.Second)
	if _, ok := store.take(id, challengeLogin); ok {
		t.Fatal("expired challenge was returned")
	}
	if len(store.entries) != 0 {
		t.Fatal("expired challenge was not dropped")
	}
}
