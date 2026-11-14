// Package logging builds the process logger.
//
// In production logs are JSON so a log platform can index each field. In
// development they are key=value text, which is easier to read in a terminal.
package logging

import (
	"io"
	"log/slog"

	"github.com/zSamir015/agenda-clinicas/backend/internal/config"
)

// New returns a JSON logger in production and a text logger otherwise.
func New(w io.Writer, cfg config.Config) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cfg.LogLevel}
	if cfg.IsProduction() {
		return slog.New(slog.NewJSONHandler(w, opts))
	}
	return slog.New(slog.NewTextHandler(w, opts))
}
