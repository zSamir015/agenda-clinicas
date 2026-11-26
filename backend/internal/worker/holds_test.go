package worker_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/zSamir015/agenda-clinicas/backend/internal/appointments"
	"github.com/zSamir015/agenda-clinicas/backend/internal/migrate"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb/mongotest"
	"github.com/zSamir015/agenda-clinicas/backend/internal/platform"
	"github.com/zSamir015/agenda-clinicas/backend/internal/worker"
)

type clinicList []platform.Clinic

func (l clinicList) List(context.Context) ([]platform.Clinic, error) { return l, nil }

type releaseFunc func(ctx context.Context) (int, error)

func (f releaseFunc) ReleaseExpiredHolds(ctx context.Context) (int, error) { return f(ctx) }

func clinic(slug string, tier platform.Tier, status platform.Status) platform.Clinic {
	return platform.Clinic{ID: bson.NewObjectID(), Slug: slug, Name: slug, Tier: tier, Status: status}
}

func slugOf(ctx context.Context) string {
	c, _ := platform.TenantFrom(ctx)
	return c.Slug
}

func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestSweepReleasesExpiredHoldsInBothTiers(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	cl, err := mongodb.Connect(ctx, mongotest.URI(t))
	if err != nil {
		t.Fatalf("mongodb.Connect: %v", err)
	}
	t.Cleanup(func() { _ = cl.Close(context.Background()) })
	client := cl.Mongo()

	clinics := clinicList{
		clinic("compartida", platform.TierStandard, platform.StatusActive),
		clinic("dedicada", platform.TierDedicated, platform.StatusActive),
	}
	runner, err := migrate.NewRunner(client.Database("platform"), migrate.OperationsMigrations())
	if err != nil {
		t.Fatalf("migrate.NewRunner: %v", err)
	}
	for _, db := range []string{"operations", "clinic_" + clinics[1].ID.Hex()} {
		if err := runner.Run(ctx, client.Database(db)); err != nil {
			t.Fatalf("migrate %s: %v", db, err)
		}
	}

	repo := appointments.NewRepository(client, "operations")
	start := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, 30).Add(14 * time.Hour)
	var held []appointments.Appointment
	for _, c := range clinics {
		appt, err := repo.Book(platform.WithTenant(ctx, c), appointments.BookInput{
			PractitionerID: bson.NewObjectID(),
			RoomID:         bson.NewObjectID(),
			ServiceID:      bson.NewObjectID(),
			PatientRef:     "patient",
			StartAt:        start,
			EndAt:          start.Add(30 * time.Minute),
			HoldTTL:        time.Millisecond,
		})
		if err != nil {
			t.Fatalf("Book %s: %v", c.Slug, err)
		}
		held = append(held, appt)
	}
	time.Sleep(10 * time.Millisecond)

	sweeper := worker.HoldSweeper{Clinics: clinics, Holds: repo, Log: discard()}
	released, err := sweeper.Sweep(ctx)
	if err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if released != 2 {
		t.Fatalf("released = %d, want 2", released)
	}
	for i, c := range clinics {
		got, err := repo.ByID(platform.WithTenant(ctx, c), held[i].ID)
		if err != nil {
			t.Fatalf("ByID %s: %v", c.Slug, err)
		}
		if got.Status != appointments.StatusCancelled {
			t.Fatalf("%s status = %s, want cancelled", c.Slug, got.Status)
		}
	}
}

func TestSweepSkipsInactiveClinics(t *testing.T) {
	var visited []string
	sweeper := worker.HoldSweeper{
		Clinics: clinicList{
			clinic("suspendida", platform.TierStandard, platform.StatusSuspended),
			clinic("borrada", platform.TierDedicated, platform.StatusDeleted),
			clinic("activa", platform.TierStandard, platform.StatusActive),
		},
		Holds: releaseFunc(func(ctx context.Context) (int, error) {
			visited = append(visited, slugOf(ctx))
			return 0, nil
		}),
		Log: discard(),
	}

	if _, err := sweeper.Sweep(context.Background()); err != nil {
		t.Fatalf("Sweep: %v", err)
	}
	if len(visited) != 1 || visited[0] != "activa" {
		t.Fatalf("visited = %v, want [activa]", visited)
	}
}

func TestSweepContinuesAfterAClinicFailsOrHangs(t *testing.T) {
	boom := errors.New("boom")
	sweeper := worker.HoldSweeper{
		Clinics: clinicList{
			clinic("falla", platform.TierStandard, platform.StatusActive),
			clinic("colgada", platform.TierStandard, platform.StatusActive),
			clinic("sana", platform.TierStandard, platform.StatusActive),
		},
		Holds: releaseFunc(func(ctx context.Context) (int, error) {
			switch slugOf(ctx) {
			case "falla":
				return 1, boom
			case "colgada":
				<-ctx.Done()
				return 0, ctx.Err()
			}
			return 2, nil
		}),
		Log:     discard(),
		Timeout: 20 * time.Millisecond,
	}

	released, err := sweeper.Sweep(context.Background())
	if !errors.Is(err, boom) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want boom and deadline exceeded", err)
	}
	// The failing clinic released one hold before its error.
	if released != 3 {
		t.Fatalf("released = %d, want 3", released)
	}
}

func TestRunSweepsAtStartAndOnEveryTickUntilCancelled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var mu sync.Mutex
	calls := 0
	sweeper := worker.HoldSweeper{
		Clinics: clinicList{clinic("activa", platform.TierStandard, platform.StatusActive)},
		Holds: releaseFunc(func(context.Context) (int, error) {
			mu.Lock()
			defer mu.Unlock()
			if calls++; calls == 3 {
				cancel()
			}
			// A failed sweep must not stop the loop.
			return 0, errors.New("boom")
		}),
		Log: discard(),
	}

	sweeper.Run(ctx, 5*time.Millisecond)

	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatalf("Run did not reach three sweeps: %d calls", calls)
	}
}
