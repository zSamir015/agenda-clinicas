// Package worker runs the periodic background jobs.
package worker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/zSamir015/agenda-clinicas/backend/internal/platform"
)

const defaultTimeout = 30 * time.Second

// ClinicLister lists the registered clinics.
type ClinicLister interface {
	List(ctx context.Context) ([]platform.Clinic, error)
}

// HoldReleaser releases the expired holds of the clinic in the context.
type HoldReleaser interface {
	ReleaseExpiredHolds(ctx context.Context) (int, error)
}

// HoldSweeper releases expired appointment holds across all clinics.
type HoldSweeper struct {
	Clinics ClinicLister
	Holds   HoldReleaser
	Log     *slog.Logger
	// Timeout bounds the clinic listing and the work on each clinic; zero means
	// 30 seconds.
	Timeout time.Duration
}

// Sweep releases the expired holds of every active clinic and returns how many
// it released. A failing clinic does not stop the others; their errors are
// joined in the result.
func (s HoldSweeper) Sweep(ctx context.Context) (int, error) {
	clinics, err := s.listClinics(ctx)
	if err != nil {
		return 0, fmt.Errorf("list clinics: %w", err)
	}
	total := 0
	var errs []error
	for _, c := range clinics {
		if ctx.Err() != nil {
			break
		}
		if c.Status != platform.StatusActive {
			continue
		}
		released, err := s.sweepClinic(ctx, c)
		total += released
		if released > 0 {
			s.Log.Info("released expired holds", "clinic", c.Slug, "count", released)
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("clinic %s: %w", c.Slug, err))
		}
	}
	return total, errors.Join(errs...)
}

func (s HoldSweeper) listClinics(ctx context.Context) ([]platform.Clinic, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout())
	defer cancel()
	return s.Clinics.List(ctx)
}

func (s HoldSweeper) timeout() time.Duration {
	if s.Timeout <= 0 {
		return defaultTimeout
	}
	return s.Timeout
}

func (s HoldSweeper) sweepClinic(ctx context.Context, c platform.Clinic) (int, error) {
	ctx, cancel := context.WithTimeout(platform.WithTenant(ctx, c), s.timeout())
	defer cancel()
	return s.Holds.ReleaseExpiredHolds(ctx)
}

// Run sweeps at once and then on every interval until ctx is cancelled. Errors
// are logged and the next sweep retries.
func (s HoldSweeper) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for ctx.Err() == nil {
		if _, err := s.Sweep(ctx); err != nil && ctx.Err() == nil {
			s.Log.Error("sweep expired holds", "error", err)
		}
		select {
		case <-ctx.Done():
		case <-ticker.C:
		}
	}
}
