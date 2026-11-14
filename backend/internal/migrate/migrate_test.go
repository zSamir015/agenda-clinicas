package migrate_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/zSamir015/agenda-clinicas/backend/internal/migrate"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb/mongotest"
)

func connectTestDB(t *testing.T, prefix string) (*mongodb.Client, *mongo.Database, *mongo.Database) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)

	c, err := mongodb.Connect(ctx, mongotest.URI(t))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer closeCancel()
		_ = c.Close(closeCtx)
	})

	suffix := bson.NewObjectID().Hex()
	platformDB := c.Mongo().Database(prefix + "_platform_" + suffix)
	targetDB := c.Mongo().Database(prefix + "_target_" + suffix)
	return c, platformDB, targetDB
}

func TestRunnerAppliesInOrderAndIsIdempotent(t *testing.T) {
	_, platformDB, targetDB := connectTestDB(t, "order")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var executed []int
	migrations := []migrate.Migration{
		// Intentionally out of order to verify Runner sorts by Version.
		{
			Version: 2,
			Name:    "second",
			Up: func(_ context.Context, _ *mongo.Database) error {
				executed = append(executed, 2)
				return nil
			},
		},
		{
			Version: 1,
			Name:    "first",
			Up: func(_ context.Context, _ *mongo.Database) error {
				executed = append(executed, 1)
				return nil
			},
		},
		{
			Version: 3,
			Name:    "third",
			Up: func(_ context.Context, _ *mongo.Database) error {
				executed = append(executed, 3)
				return nil
			},
		},
	}

	runner, err := migrate.NewRunner(platformDB, migrations)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	if err := runner.Run(ctx, targetDB); err != nil {
		t.Fatalf("first Run: %v", err)
	}
	if !slices.Equal(executed, []int{1, 2, 3}) {
		t.Fatalf("expected execution order [1 2 3], got %v", executed)
	}

	// Second run must be a no-op.
	executed = nil
	if err := runner.Run(ctx, targetDB); err != nil {
		t.Fatalf("second Run: %v", err)
	}
	if len(executed) != 0 {
		t.Fatalf("expected no migrations on second run, got %v", executed)
	}
}

func TestRunnerResumesAfterFailure(t *testing.T) {
	_, platformDB, targetDB := connectTestDB(t, "resume")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	failThird := true
	var executed []int
	sentinel := errors.New("boom")

	migrations := []migrate.Migration{
		{
			Version: 1,
			Name:    "first",
			Up: func(_ context.Context, _ *mongo.Database) error {
				executed = append(executed, 1)
				return nil
			},
		},
		{
			Version: 2,
			Name:    "second",
			Up: func(_ context.Context, _ *mongo.Database) error {
				executed = append(executed, 2)
				return nil
			},
		},
		{
			Version: 3,
			Name:    "third",
			Up: func(_ context.Context, _ *mongo.Database) error {
				executed = append(executed, 3)
				if failThird {
					return sentinel
				}
				return nil
			},
		},
		{
			Version: 4,
			Name:    "fourth",
			Up: func(_ context.Context, _ *mongo.Database) error {
				executed = append(executed, 4)
				return nil
			},
		},
	}

	runner, err := migrate.NewRunner(platformDB, migrations)
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}

	err = runner.Run(ctx, targetDB)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if !slices.Equal(executed, []int{1, 2, 3}) {
		t.Fatalf("expected [1 2 3] before failure, got %v", executed)
	}

	// Fix migration 3 and re-run: must resume at 3 and run 3, 4 only.
	failThird = false
	executed = nil
	if err := runner.Run(ctx, targetDB); err != nil {
		t.Fatalf("resume Run: %v", err)
	}
	if !slices.Equal(executed, []int{3, 4}) {
		t.Fatalf("expected resume at [3 4], got %v", executed)
	}
}

func TestPlatformMigrationsClinicsSlugUnique(t *testing.T) {
	_, platformDB, _ := connectTestDB(t, "platform")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	runner, err := migrate.NewRunner(platformDB, migrate.PlatformMigrations())
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	if err := runner.Run(ctx, platformDB); err != nil {
		t.Fatalf("Run: %v", err)
	}

	clinics := platformDB.Collection("clinics")
	if _, err := clinics.InsertOne(ctx, bson.D{{Key: "slug", Value: "istmo"}}); err != nil {
		t.Fatalf("first InsertOne: %v", err)
	}
	if _, err := clinics.InsertOne(ctx, bson.D{{Key: "slug", Value: "istmo"}}); !mongo.IsDuplicateKeyError(err) {
		t.Fatalf("expected duplicate key error on clinics.slug, got %v", err)
	}
}

func TestNewRunnerValidation(t *testing.T) {
	_, platformDB, _ := connectTestDB(t, "validate")

	noop := func(_ context.Context, _ *mongo.Database) error { return nil }

	cases := []struct {
		name       string
		platformDB *mongo.Database
		migrations []migrate.Migration
	}{
		{name: "nil platformDB", platformDB: nil, migrations: nil},
		{name: "non-positive version", platformDB: platformDB, migrations: []migrate.Migration{{Version: 0, Name: "bad", Up: noop}}},
		{name: "empty name", platformDB: platformDB, migrations: []migrate.Migration{{Version: 1, Name: "", Up: noop}}},
		{name: "nil Up", platformDB: platformDB, migrations: []migrate.Migration{{Version: 1, Name: "bad", Up: nil}}},
		{name: "duplicate version", platformDB: platformDB, migrations: []migrate.Migration{
			{Version: 1, Name: "a", Up: noop},
			{Version: 1, Name: "b", Up: noop},
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := migrate.NewRunner(tc.platformDB, tc.migrations); err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}
