package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/zSamir015/agenda-clinicas/backend/internal/config"
)

func TestProductionLogsJSON(t *testing.T) {
	var buf bytes.Buffer
	New(&buf, config.Config{Env: "production", LogLevel: slog.LevelInfo}).Info("hello", "k", "v")
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("not JSON: %q", buf.String())
	}
	if m["msg"] != "hello" || m["k"] != "v" {
		t.Fatalf("got %v", m)
	}
}

func TestDevelopmentLogsTextAndRespectsLevel(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, config.Config{Env: "development", LogLevel: slog.LevelWarn})
	l.Info("hidden")
	l.Warn("shown")
	out := buf.String()
	if strings.Contains(out, "hidden") || !strings.Contains(out, "msg=shown") {
		t.Fatalf("got %q", out)
	}
}
