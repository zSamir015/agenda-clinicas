package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/zSamir015/agenda-clinicas/backend/internal/crypto"
	"github.com/zSamir015/agenda-clinicas/backend/internal/migrate"
	"github.com/zSamir015/agenda-clinicas/backend/internal/platform"
)

type createOptions struct {
	slug          string
	name          string
	tier          string
	region        string
	masterKeyPath string
	insecure      bool
}

func runCreate(ctx context.Context, args []string, mongoURI string, getenv func(string) string, stdout, stderr io.Writer) error {
	opts, err := parseCreateFlags(args, getenv, stderr)
	if err != nil {
		return err
	}

	clinicInput, err := platform.NewClinic(opts.slug, opts.name, platform.Tier(opts.tier), opts.region)
	if err != nil {
		return err
	}

	cl, err := connectMongo(ctx, mongoURI)
	if err != nil {
		return err
	}
	defer func() { _ = cl.Close(context.Background()) }()

	encFactory := func(ctx context.Context, client *mongo.Client) (crypto.Encryptor, error) {
		return newEncryptor(ctx, client, opts.masterKeyPath, opts.insecure)
	}

	clinic, err := provisionClinic(ctx, cl.Mongo(), clinicInput, encFactory, nil)
	if err != nil {
		return err
	}
	if err := applyOperationsMigrations(ctx, cl.Mongo(), clinic); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(stdout, "clinic ready: slug=%s id=%s tier=%s status=%s\n",
		clinic.Slug, clinic.ID.Hex(), clinic.Tier, clinic.Status)
	return nil
}

func parseCreateFlags(args []string, getenv func(string) string, stderr io.Writer) (createOptions, error) {
	defaultKey := getenv("MASTER_KEY_PATH")
	if defaultKey == "" {
		defaultKey = ".local-master-key"
	}

	var opts createOptions
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.StringVar(&opts.slug, "slug", "", "clinic subdomain slug (required)")
	fs.StringVar(&opts.name, "name", "", "clinic display name (required)")
	fs.StringVar(&opts.tier, "tier", string(platform.TierStandard), "isolation tier: standard or dedicated")
	fs.StringVar(&opts.region, "region", "pa", "clinic region")
	fs.StringVar(&opts.masterKeyPath, "master-key", defaultKey, "path to local master key file")
	fs.BoolVar(&opts.insecure, "insecure-no-encryption", false, "use in-memory fake encryptor when built without cse tag")

	if err := fs.Parse(args); err != nil {
		return createOptions{}, err
	}
	return opts, nil
}

func provisionClinic(
	ctx context.Context,
	client *mongo.Client,
	input platform.Clinic,
	encFactory func(context.Context, *mongo.Client) (crypto.Encryptor, error),
	tenantMigrations []migrate.Migration,
) (platform.Clinic, error) {
	platformDB := client.Database("platform")

	platformRunner, err := migrate.NewRunner(platformDB, migrate.PlatformMigrations())
	if err != nil {
		return platform.Clinic{}, fmt.Errorf("init platform migrations: %w", err)
	}
	if err := platformRunner.Run(ctx, platformDB); err != nil {
		return platform.Clinic{}, fmt.Errorf("run platform migrations: %w", err)
	}

	repo, err := platform.NewClinicRepository(ctx, platformDB)
	if err != nil {
		return platform.Clinic{}, fmt.Errorf("init clinic repository: %w", err)
	}

	clinic, err := createOrGetClinic(ctx, repo, input)
	if err != nil {
		return platform.Clinic{}, err
	}

	enc, err := encFactory(ctx, client)
	if err != nil {
		return platform.Clinic{}, err
	}
	if err := enc.EnsureKey(ctx, clinic.ID.Hex()); err != nil {
		return platform.Clinic{}, fmt.Errorf("ensure clinic data key: %w", err)
	}

	tenantRunner, err := migrate.NewRunner(platformDB, tenantMigrations)
	if err != nil {
		return platform.Clinic{}, fmt.Errorf("init tenant migrations: %w", err)
	}
	for _, dbName := range clinicDatabases(clinic) {
		if err := tenantRunner.Run(ctx, client.Database(dbName)); err != nil {
			return platform.Clinic{}, fmt.Errorf("migrate database %s: %w", dbName, err)
		}
	}

	return clinic, nil
}

func createOrGetClinic(ctx context.Context, repo *platform.ClinicRepository, input platform.Clinic) (platform.Clinic, error) {
	created, err := repo.Create(ctx, input)
	if err == nil {
		return created, nil
	}
	if !errors.Is(err, platform.ErrSlugTaken) {
		return platform.Clinic{}, fmt.Errorf("create clinic: %w", err)
	}
	existing, getErr := repo.BySlug(ctx, input.Slug)
	if getErr != nil {
		return platform.Clinic{}, fmt.Errorf("fetch existing clinic %q: %w", input.Slug, getErr)
	}
	return existing, nil
}

func clinicDatabases(c platform.Clinic) []string {
	if c.Tier == platform.TierDedicated {
		return []string{"clinic_" + c.ID.Hex()}
	}
	return []string{"operations", "patients_" + c.ID.Hex()}
}

func operationsDatabase(c platform.Clinic) string {
	if c.Tier == platform.TierDedicated {
		return "clinic_" + c.ID.Hex()
	}
	return "operations"
}

func applyOperationsMigrations(ctx context.Context, client *mongo.Client, clinic platform.Clinic) error {
	platformDB := client.Database("platform")
	runner, err := migrate.NewRunner(platformDB, migrate.OperationsMigrations())
	if err != nil {
		return fmt.Errorf("init operations migrations: %w", err)
	}
	opsDB := operationsDatabase(clinic)
	if err := runner.Run(ctx, client.Database(opsDB)); err != nil {
		return fmt.Errorf("migrate operations database %s: %w", opsDB, err)
	}
	return nil
}
