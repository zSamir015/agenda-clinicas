package main

import (
	"context"
	"flag"
	"fmt"
	"io"

	"github.com/zSamir015/agenda-clinicas/backend/internal/platform"
)

// runMigrate applies pending operations migrations for every registered clinic.
func runMigrate(ctx context.Context, args []string, mongoURI string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return err
	}

	cl, err := connectMongo(ctx, mongoURI)
	if err != nil {
		return err
	}
	defer func() { _ = cl.Close(context.Background()) }()

	repo, err := platform.NewClinicRepository(ctx, cl.Mongo().Database("platform"))
	if err != nil {
		return fmt.Errorf("init clinic repository: %w", err)
	}
	clinics, err := repo.List(ctx)
	if err != nil {
		return fmt.Errorf("list clinics: %w", err)
	}

	for _, c := range clinics {
		if err := applyOperationsMigrations(ctx, cl.Mongo(), c); err != nil {
			return fmt.Errorf("clinic %s: %w", c.Slug, err)
		}
		_, _ = fmt.Fprintf(stdout, "migrated: slug=%s database=%s\n", c.Slug, operationsDatabase(c))
	}
	return nil
}
