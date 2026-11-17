package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/zSamir015/agenda-clinicas/backend/internal/crypto"
	"github.com/zSamir015/agenda-clinicas/backend/internal/migrate"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb/mongotest"
	"github.com/zSamir015/agenda-clinicas/backend/internal/platform"
)

func TestCreateAndListIntegration(t *testing.T) {
	uri := mongotest.URI(t)
	ctx := context.Background()
	getenv := func(k string) string {
		if k == "MONGO_URI" {
			return uri
		}
		return ""
	}

	t.Run("invalid slug fails", func(t *testing.T) {
		var out, errBuf bytes.Buffer
		err := run(ctx, []string{"create", "--slug", "A", "--name", "Clinica Mal", "--insecure-no-encryption"}, getenv, &out, &errBuf)
		if !errors.Is(err, platform.ErrInvalidSlug) {
			t.Fatalf("got %v, want ErrInvalidSlug", err)
		}
	})

	t.Run("create standard and dedicated clinics then list", func(t *testing.T) {
		var out, errBuf bytes.Buffer
		err := run(ctx, []string{
			"create",
			"--slug", "clinica-norte",
			"--name", "Clinica Norte",
			"--tier", "standard",
			"--region", "pa",
			"--insecure-no-encryption",
		}, getenv, &out, &errBuf)
		if err != nil {
			t.Fatalf("create standard: %v", err)
		}
		if !strings.Contains(out.String(), "slug=clinica-norte") {
			t.Fatalf("unexpected create output: %q", out.String())
		}

		out.Reset()
		err = run(ctx, []string{
			"create",
			"--slug", "clinica-sur",
			"--name", "Clinica Sur",
			"--tier", "dedicated",
			"--insecure-no-encryption",
		}, getenv, &out, &errBuf)
		if err != nil {
			t.Fatalf("create dedicated: %v", err)
		}

		out.Reset()
		err = run(ctx, []string{"list"}, getenv, &out, &errBuf)
		if err != nil {
			t.Fatalf("list: %v", err)
		}

		table := out.String()
		for _, want := range []string{"SLUG", "NAME", "TIER", "STATUS", "clinica-norte", "Clinica Norte", "standard", "clinica-sur", "Clinica Sur", "dedicated", "active"} {
			if !strings.Contains(table, want) {
				t.Fatalf("list output missing %q:\n%s", want, table)
			}
		}

		// Verify operational collections were migrated on operations (standard) and clinic_<id> (dedicated).
		cl, err := mongodb.Connect(ctx, uri)
		if err != nil {
			t.Fatalf("mongodb.Connect: %v", err)
		}
		t.Cleanup(func() { _ = cl.Close(context.Background()) })

		repo, err := platform.NewClinicRepository(ctx, cl.Mongo().Database("platform"))
		if err != nil {
			t.Fatalf("NewClinicRepository: %v", err)
		}
		dedicated, err := repo.BySlug(ctx, "clinica-sur")
		if err != nil {
			t.Fatalf("BySlug clinica-sur: %v", err)
		}
		for _, dbName := range []string{"operations", "clinic_" + dedicated.ID.Hex()} {
			names, err := cl.Mongo().Database(dbName).ListCollectionNames(ctx, bson.D{})
			if err != nil {
				t.Fatalf("ListCollectionNames %s: %v", dbName, err)
			}
			for _, wantColl := range []string{"practitioners", "rooms", "services", "availability_rules", "blocks"} {
				if !strings.Contains(strings.Join(names, ","), wantColl) {
					t.Fatalf("database %s missing migrated collection %q: %v", dbName, wantColl, names)
				}
			}
		}
	})

	t.Run("second attempt resumes after EnsureKey failure", func(t *testing.T) {
		cl, err := mongodb.Connect(ctx, uri)
		if err != nil {
			t.Fatalf("mongodb.Connect: %v", err)
		}
		t.Cleanup(func() { _ = cl.Close(context.Background()) })

		input, err := platform.NewClinic("clinica-reintento", "Clinica Reintento", platform.TierStandard, "pa")
		if err != nil {
			t.Fatalf("NewClinic: %v", err)
		}

		failingEnc := func(_ context.Context, _ *mongo.Client) (crypto.Encryptor, error) {
			return nil, errors.New("simulated key vault outage")
		}
		if _, err := provisionClinic(ctx, cl.Mongo(), input, failingEnc, nil); err == nil {
			t.Fatal("expected first attempt to fail")
		}

		fakeEnc := crypto.NewFake()
		tenantMigs := []migrate.Migration{
			{
				Version: 100,
				Name:    "init_marker",
				Up: func(ctx context.Context, db *mongo.Database) error {
					_, err := db.Collection("mig_markers").InsertOne(ctx, bson.M{"ok": true})
					return err
				},
			},
		}

		clinic, err := provisionClinic(ctx, cl.Mongo(), input, func(context.Context, *mongo.Client) (crypto.Encryptor, error) {
			return fakeEnc, nil
		}, tenantMigs)
		if err != nil {
			t.Fatalf("second attempt failed: %v", err)
		}

		// Verify DEK was ensured on retry by encrypting with it.
		if _, err := fakeEnc.EncryptRandom(ctx, clinic.ID.Hex(), "1-234-567"); err != nil {
			t.Fatalf("EncryptRandom after retry: %v", err)
		}

		// Verify clinic databases received the tenant migration.
		for _, dbName := range []string{"operations", "patients_" + clinic.ID.Hex()} {
			n, err := cl.Mongo().Database(dbName).Collection("mig_markers").CountDocuments(ctx, bson.M{})
			if err != nil || n != 1 {
				t.Fatalf("database %s migration marker count=%d, err=%v", dbName, n, err)
			}
		}
	})
}
