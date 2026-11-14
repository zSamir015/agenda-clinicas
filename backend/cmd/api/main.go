// Command api serves the agenda-clinicas HTTP API.
//
// main only wires pieces together (config, logger, database, server) and
// handles startup and shutdown. The behavior lives in internal/.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zSamir015/agenda-clinicas/backend/internal/config"
	"github.com/zSamir015/agenda-clinicas/backend/internal/httpapi"
	"github.com/zSamir015/agenda-clinicas/backend/internal/logging"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	log := logging.New(os.Stdout, cfg)

	// ctx is cancelled on Ctrl+C or SIGTERM, which starts the graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	db, err := mongodb.Connect(connectCtx, cfg.MongoURI)
	cancel()
	if err != nil {
		return err
	}
	defer func() { _ = db.Close(context.Background()) }()

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewHandler(httpapi.NewServer(db)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("api listening", "addr", cfg.HTTPAddr, "env", cfg.Env)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("http server: %w", err)
		}
	case <-ctx.Done():
		log.Info("shutting down")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
	}
	return nil
}
