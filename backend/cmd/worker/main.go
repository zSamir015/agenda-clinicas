// Command worker runs the agenda-clinicas background jobs.
//
// main only wires pieces together and handles shutdown. The behavior lives in
// internal/worker.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/zSamir015/agenda-clinicas/backend/internal/appointments"
	"github.com/zSamir015/agenda-clinicas/backend/internal/config"
	"github.com/zSamir015/agenda-clinicas/backend/internal/logging"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb"
	"github.com/zSamir015/agenda-clinicas/backend/internal/platform"
	"github.com/zSamir015/agenda-clinicas/backend/internal/worker"
)

const holdSweepInterval = time.Minute

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

	// ctx is cancelled on Ctrl+C or SIGTERM, which stops the jobs.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	connectCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	db, err := mongodb.Connect(connectCtx, cfg.MongoURI)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close(context.Background()) }()

	clinics, err := platform.NewClinicRepository(connectCtx, db.Mongo().Database("platform"))
	if err != nil {
		return fmt.Errorf("init clinic repo: %w", err)
	}

	sweeper := worker.HoldSweeper{
		Clinics: clinics,
		Holds:   appointments.NewRepository(db.Mongo(), "operations"),
		Log:     log,
	}
	log.Info("worker running", "env", cfg.Env, "holdSweepInterval", holdSweepInterval.String())
	sweeper.Run(ctx, holdSweepInterval)
	log.Info("shutting down")
	return nil
}
