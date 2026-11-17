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

	"github.com/zSamir015/agenda-clinicas/backend/internal/auth"
	"github.com/zSamir015/agenda-clinicas/backend/internal/config"
	"github.com/zSamir015/agenda-clinicas/backend/internal/httpapi"
	"github.com/zSamir015/agenda-clinicas/backend/internal/logging"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb"
	"github.com/zSamir015/agenda-clinicas/backend/internal/platform"
	"github.com/zSamir015/agenda-clinicas/backend/internal/rbac"
	"github.com/zSamir015/agenda-clinicas/backend/internal/users"
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

	initCtx, cancelInit := context.WithTimeout(ctx, 10*time.Second)
	authDeps, err := initAuthDeps(initCtx, cfg, db)
	cancelInit()
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.NewHandler(httpapi.NewServer(db, httpapi.WithAuth(authDeps))),
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

func initAuthDeps(ctx context.Context, cfg config.Config, db *mongodb.Client) (httpapi.AuthDeps, error) {
	platformDB := db.Mongo().Database("platform")

	clinics, err := platform.NewClinicRepository(ctx, platformDB)
	if err != nil {
		return httpapi.AuthDeps{}, fmt.Errorf("init clinic repo: %w", err)
	}
	roles, err := rbac.NewRoleRepository(ctx, platformDB)
	if err != nil {
		return httpapi.AuthDeps{}, fmt.Errorf("init role repo: %w", err)
	}
	userRepo, err := users.NewRepository(ctx, platformDB)
	if err != nil {
		return httpapi.AuthDeps{}, fmt.Errorf("init users repo: %w", err)
	}
	sessions, err := auth.NewSessionRepository(ctx, platformDB)
	if err != nil {
		return httpapi.AuthDeps{}, fmt.Errorf("init sessions repo: %w", err)
	}
	creds, err := auth.NewCredentialRepository(ctx, platformDB)
	if err != nil {
		return httpapi.AuthDeps{}, fmt.Errorf("init credentials repo: %w", err)
	}
	otps, err := auth.NewOTPRepository(ctx, platformDB)
	if err != nil {
		return httpapi.AuthDeps{}, fmt.Errorf("init otp repo: %w", err)
	}
	wa, err := auth.NewWebAuthnService(cfg.WebAuthnRPDisplayName, cfg.WebAuthnRPID, cfg.WebAuthnRPOrigins, creds)
	if err != nil {
		return httpapi.AuthDeps{}, fmt.Errorf("init webauthn: %w", err)
	}

	return httpapi.AuthDeps{
		Clinics:        clinics,
		Roles:          roles,
		Users:          userRepo,
		Sessions:       sessions,
		Creds:          creds,
		OTPs:           otps,
		WebAuthn:       wa,
		Limiter:        auth.NewRateLimiter(),
		CookieSecure:   cfg.IsProduction(),
		AllowedOrigins: cfg.CORSAllowedOrigins,
	}, nil
}
