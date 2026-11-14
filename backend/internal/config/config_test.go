package config

import (
	"log/slog"
	"testing"
)

// env turns a map into a getenv function, so tests never touch the real
// environment of the process.
func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(map[string]string{"MONGO_URI": "mongodb://x/?replicaSet=rs0"}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Env != "development" || cfg.HTTPAddr != ":8080" || cfg.LogLevel != slog.LevelInfo {
		t.Fatalf("defaults wrong: %+v", cfg)
	}
	if cfg.MongoURI != "mongodb://x/?replicaSet=rs0" {
		t.Fatalf("MongoURI not read: %q", cfg.MongoURI)
	}
	if cfg.IsProduction() {
		t.Fatal("development must not be production")
	}
}

func TestLoadRequiresMongoURI(t *testing.T) {
	if _, err := Load(env(nil)); err == nil {
		t.Fatal("expected error when MONGO_URI is missing")
	}
}

func TestLoadRejectsUnknownEnv(t *testing.T) {
	_, err := Load(env(map[string]string{"MONGO_URI": "mongodb://x", "APP_ENV": "staging"}))
	if err == nil {
		t.Fatal("expected error for unknown APP_ENV")
	}
}

func TestLoadRejectsBadLogLevel(t *testing.T) {
	_, err := Load(env(map[string]string{"MONGO_URI": "mongodb://x", "LOG_LEVEL": "loud"}))
	if err == nil {
		t.Fatal("expected error for unknown LOG_LEVEL")
	}
}

func TestLoadReadsEverything(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"MONGO_URI": "mongodb://x",
		"APP_ENV":   "production",
		"HTTP_ADDR": ":9000",
		"LOG_LEVEL": "debug",
	}))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.LogLevel != slog.LevelDebug || cfg.HTTPAddr != ":9000" || !cfg.IsProduction() {
		t.Fatalf("got %+v", cfg)
	}
}
