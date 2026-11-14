package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func get(t *testing.T, h http.Handler, path string) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %q", rec.Body.String())
	}
	return rec, body
}

func TestHealthzDoesNotDependOnDatabase(t *testing.T) {
	rec, body := get(t, NewHandler(NewServer(fakePinger{err: errors.New("down")})), "/healthz")
	if rec.Code != http.StatusOK || body["status"] != "ok" {
		t.Fatalf("got %d %v", rec.Code, body)
	}
}

func TestReadyzOK(t *testing.T) {
	rec, body := get(t, NewHandler(NewServer(fakePinger{})), "/readyz")
	if rec.Code != http.StatusOK || body["status"] != "ok" {
		t.Fatalf("got %d %v", rec.Code, body)
	}
}

func TestReadyzDatabaseDown(t *testing.T) {
	rec, body := get(t, NewHandler(NewServer(fakePinger{err: errors.New("secret internal detail")})), "/readyz")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/problem+json") {
		t.Fatalf("content-type %q", ct)
	}
	if body["code"] != "database_unavailable" {
		t.Fatalf("body %v", body)
	}
	if strings.Contains(rec.Body.String(), "secret internal detail") {
		t.Fatal("internal error must not reach the client")
	}
}

// blockingPinger simulates a database that does not answer: it returns only
// when the context it receives is cancelled.
type blockingPinger struct{}

func (blockingPinger) Ping(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestReadyzTimesOutWhenDatabaseHangs(t *testing.T) {
	start := time.Now()
	rec, body := get(t, NewHandler(NewServer(blockingPinger{})), "/readyz")
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("readyz took %v, must give up on its own", elapsed)
	}
	if rec.Code != http.StatusServiceUnavailable || body["code"] != "database_unavailable" {
		t.Fatalf("got %d %v", rec.Code, body)
	}
}
