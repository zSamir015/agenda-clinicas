package platform_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb/mongotest"
	"github.com/zSamir015/agenda-clinicas/backend/internal/platform"
)

func newRepo(t *testing.T) *platform.ClinicRepository {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c, err := mongodb.Connect(ctx, mongotest.URI(t))
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	t.Cleanup(func() { _ = c.Close(context.Background()) })
	// One database per test keeps tests independent on the shared container.
	db := c.Mongo().Database("platform_" + t.Name())
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	repo, err := platform.NewClinicRepository(ctx, db)
	if err != nil {
		t.Fatalf("NewClinicRepository: %v", err)
	}
	return repo
}

func TestNewClinicValidatesSlug(t *testing.T) {
	for _, slug := range []string{"", "ab", "Clinica", "con espacio", "-inicio", "fin-", "x_y", "a-very-long-slug-that-goes-beyond-forty-chars"} {
		if _, err := platform.NewClinic(slug, "Nombre", platform.TierStandard, "pa"); !errors.Is(err, platform.ErrInvalidSlug) {
			t.Errorf("slug %q: want ErrInvalidSlug, got %v", slug, err)
		}
	}
	if _, err := platform.NewClinic("clinica-1", "Clínica Uno", platform.TierStandard, "pa"); err != nil {
		t.Fatalf("valid slug rejected: %v", err)
	}
}

func TestCreateAndBySlug(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	c, _ := platform.NewClinic("san-jose", "San José", platform.TierStandard, "pa")
	created, err := repo.Create(ctx, c)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID.IsZero() || created.Status != platform.StatusActive {
		t.Fatalf("unexpected created clinic: %+v", created)
	}
	got, err := repo.BySlug(ctx, "san-jose")
	if err != nil || got.ID != created.ID {
		t.Fatalf("BySlug: %v %+v", err, got)
	}
	if _, err := repo.BySlug(ctx, "nope"); !errors.Is(err, platform.ErrTenantNotFound) {
		t.Fatalf("want ErrTenantNotFound, got %v", err)
	}
}

func TestCreateDuplicateSlug(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	c, _ := platform.NewClinic("repetida", "Una", platform.TierStandard, "pa")
	if _, err := repo.Create(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Create(ctx, c); !errors.Is(err, platform.ErrSlugTaken) {
		t.Fatalf("want ErrSlugTaken, got %v", err)
	}
}

func TestList(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	for _, s := range []string{"bbb", "aaa"} {
		c, _ := platform.NewClinic(s, s, platform.TierStandard, "pa")
		if _, err := repo.Create(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	list, err := repo.List(ctx)
	if err != nil || len(list) != 2 || list[0].Slug != "aaa" {
		t.Fatalf("List: %v %+v", err, list)
	}
}
