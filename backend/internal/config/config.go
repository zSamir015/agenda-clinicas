// Package config reads the service configuration from environment variables.
//
// Everything that changes between machines (addresses, credentials, log level)
// comes from the environment, never from the code. Load receives the lookup
// function as a parameter: main passes os.Getenv and tests pass a map, so
// tests never depend on the real environment.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
)

// Config holds the settings shared by the API and the worker.
type Config struct {
	Env                   string     // "development", "test" or "production"
	HTTPAddr              string     // address the API listens on, for example ":8080"
	MongoURI              string     // MongoDB connection string, must point to a replica set
	LogLevel              slog.Level // minimum level that gets logged
	WebAuthnRPID          string     // relying party ID for passkeys (e.g. "localhost")
	WebAuthnRPDisplayName string     // human-readable relying party name
	WebAuthnRPOrigins     []string   // allowed WebAuthn origins
	CORSAllowedOrigins    []string   // allowed browser origins for cookie-authenticated API calls
}

// IsProduction reports whether the process runs in production.
func (c Config) IsProduction() bool { return c.Env == "production" }

// Load builds a Config from getenv and validates it.
//
// MONGO_URI has no default: a service pointed at the wrong database is worse
// than one that refuses to start. On any error it returns the zero Config so
// callers never see a half-valid configuration.
func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		Env:                   orDefault(getenv("APP_ENV"), "development"),
		HTTPAddr:              orDefault(getenv("HTTP_ADDR"), ":8080"),
		MongoURI:              getenv("MONGO_URI"),
		WebAuthnRPID:          orDefault(getenv("WEBAUTHN_RP_ID"), "localhost"),
		WebAuthnRPDisplayName: orDefault(getenv("WEBAUTHN_RP_DISPLAY_NAME"), "Agenda Clínicas"),
		WebAuthnRPOrigins:     splitCSVOrDefault(getenv("WEBAUTHN_RP_ORIGINS"), []string{"http://localhost:3000"}),
		CORSAllowedOrigins:    splitCSVOrDefault(getenv("CORS_ALLOWED_ORIGINS"), []string{"http://localhost:3000"}),
	}
	switch cfg.Env {
	case "development", "test", "production":
	default:
		return Config{}, fmt.Errorf("APP_ENV must be development, test or production, got %q", cfg.Env)
	}
	if cfg.MongoURI == "" {
		return Config{}, errors.New("MONGO_URI is required")
	}
	// slog.Level already understands "debug", "info", "warn" and "error".
	if err := cfg.LogLevel.UnmarshalText([]byte(orDefault(getenv("LOG_LEVEL"), "info"))); err != nil {
		return Config{}, fmt.Errorf("LOG_LEVEL: %w", err)
	}
	return cfg, nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func splitCSVOrDefault(raw string, def []string) []string {
	if strings.TrimSpace(raw) == "" {
		return def
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}
