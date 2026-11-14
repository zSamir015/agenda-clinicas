package mongodb_test

import (
	"context"
	"testing"
	"time"

	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb/mongotest"
)

func TestConnectAndPing(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, err := mongodb.Connect(ctx, mongotest.URI(t))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = c.Close(ctx) }()
	if err := c.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}
}

func TestConnectFailsFast(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := mongodb.Connect(ctx, "mongodb://127.0.0.1:1/?replicaSet=rs0&serverSelectionTimeoutMS=500"); err == nil {
		t.Fatal("expected error for unreachable server")
	}
}
