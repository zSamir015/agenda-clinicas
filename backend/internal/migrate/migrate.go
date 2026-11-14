// Package migrate provides a versioned, idempotent, and resumable schema
// migration runner backed by the platform.migrations collection.
package migrate

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const migrationsCollection = "migrations"

// Migration describes a single numbered schema or index change for a database.
type Migration struct {
	Version int
	Name    string
	Up      func(ctx context.Context, db *mongo.Database) error
}

type migrationRecord struct {
	Database  string    `bson:"database"`
	Version   int       `bson:"version"`
	Name      string    `bson:"name"`
	AppliedAt time.Time `bson:"appliedAt"`
}

// Runner applies pending migrations in version order and records each
// completed step in platform.migrations.
type Runner struct {
	platformDB *mongo.Database
	migrations []Migration
	now        func() time.Time
}

// NewRunner validates and returns a Runner that stores state in platformDB.
func NewRunner(platformDB *mongo.Database, migrations []Migration) (*Runner, error) {
	if platformDB == nil {
		return nil, errors.New("migrate: platform database is required")
	}

	sorted := slices.Clone(migrations)
	slices.SortFunc(sorted, func(a, b Migration) int {
		return a.Version - b.Version
	})

	seen := make(map[int]struct{}, len(sorted))
	for _, m := range sorted {
		if m.Version <= 0 {
			return nil, fmt.Errorf("migrate: version must be positive, got %d", m.Version)
		}
		if m.Name == "" {
			return nil, fmt.Errorf("migrate: migration %d has empty name", m.Version)
		}
		if m.Up == nil {
			return nil, fmt.Errorf("migrate: migration %d (%s) has nil Up func", m.Version, m.Name)
		}
		if _, dup := seen[m.Version]; dup {
			return nil, fmt.Errorf("migrate: duplicate migration version %d", m.Version)
		}
		seen[m.Version] = struct{}{}
	}

	return &Runner{
		platformDB: platformDB,
		migrations: sorted,
		now:        time.Now,
	}, nil
}

// Run applies all pending migrations to targetDB in ascending version order.
// Previously applied versions are skipped, and if a migration fails, earlier
// completed versions remain recorded so subsequent runs resume at the failure.
func (r *Runner) Run(ctx context.Context, targetDB *mongo.Database) error {
	if targetDB == nil {
		return errors.New("migrate: target database is required")
	}

	coll := r.platformDB.Collection(migrationsCollection)
	if err := ensureMigrationsIndex(ctx, coll); err != nil {
		return fmt.Errorf("migrate: ensure migrations index: %w", err)
	}

	applied, err := appliedVersions(ctx, coll, targetDB.Name())
	if err != nil {
		return fmt.Errorf("migrate: load applied versions for %s: %w", targetDB.Name(), err)
	}

	for _, m := range r.migrations {
		if applied[m.Version] {
			continue
		}
		if err := m.Up(ctx, targetDB); err != nil {
			return fmt.Errorf("migrate: apply v%d (%s) to %s: %w", m.Version, m.Name, targetDB.Name(), err)
		}
		rec := migrationRecord{
			Database:  targetDB.Name(),
			Version:   m.Version,
			Name:      m.Name,
			AppliedAt: r.now().UTC(),
		}
		if _, err := coll.InsertOne(ctx, rec); err != nil {
			return fmt.Errorf("migrate: record v%d (%s) for %s: %w", m.Version, m.Name, targetDB.Name(), err)
		}
	}

	return nil
}

func ensureMigrationsIndex(ctx context.Context, coll *mongo.Collection) error {
	model := mongo.IndexModel{
		Keys: bson.D{
			{Key: "database", Value: 1},
			{Key: "version", Value: 1},
		},
		Options: options.Index().SetUnique(true).SetName("database_version_unique"),
	}
	_, err := coll.Indexes().CreateOne(ctx, model)
	return err
}

func appliedVersions(ctx context.Context, coll *mongo.Collection, dbName string) (map[int]bool, error) {
	cur, err := coll.Find(ctx, bson.D{{Key: "database", Value: dbName}})
	if err != nil {
		return nil, err
	}
	defer func() { _ = cur.Close(ctx) }()

	applied := make(map[int]bool)
	for cur.Next(ctx) {
		var rec migrationRecord
		if err := cur.Decode(&rec); err != nil {
			return nil, err
		}
		applied[rec.Version] = true
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return applied, nil
}
