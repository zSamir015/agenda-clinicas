package platform_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zSamir015/agenda-clinicas/backend/internal/platform"
)

// fakeSource counts lookups so tests can see whether the cache was used.
type fakeSource struct {
	clinics map[string]platform.Clinic
	calls   int
}

func (f *fakeSource) BySlug(_ context.Context, slug string) (platform.Clinic, error) {
	f.calls++
	c, ok := f.clinics[slug]
	if !ok {
		return platform.Clinic{}, platform.ErrTenantNotFound
	}
	return c, nil
}

type clock struct{ now time.Time }

func (c *clock) Now() time.Time { return c.now }

func newResolver(src *fakeSource) (*platform.TenantResolver, *clock) {
	ck := &clock{now: time.Unix(1_000_000, 0)}
	return platform.NewTenantResolver(src, platform.WithClock(ck.Now)), ck
}

func TestResolverCachesFor60s(t *testing.T) {
	src := &fakeSource{clinics: map[string]platform.Clinic{"uno": {Slug: "uno", Status: platform.StatusActive}}}
	r, ck := newResolver(src)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := r.Resolve(ctx, "uno"); err != nil {
			t.Fatal(err)
		}
	}
	if src.calls != 1 {
		t.Fatalf("want 1 lookup, got %d", src.calls)
	}
	ck.now = ck.now.Add(61 * time.Second)
	if _, err := r.Resolve(ctx, "uno"); err != nil {
		t.Fatal(err)
	}
	if src.calls != 2 {
		t.Fatalf("want 2 lookups after expiry, got %d", src.calls)
	}
}

func TestResolverNegativeCache(t *testing.T) {
	src := &fakeSource{clinics: map[string]platform.Clinic{}}
	r, ck := newResolver(src)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if _, err := r.Resolve(ctx, "fantasma"); !errors.Is(err, platform.ErrTenantNotFound) {
			t.Fatalf("want ErrTenantNotFound, got %v", err)
		}
	}
	if src.calls != 1 {
		t.Fatalf("want 1 lookup, got %d", src.calls)
	}
	ck.now = ck.now.Add(6 * time.Second)
	_, _ = r.Resolve(ctx, "fantasma")
	if src.calls != 2 {
		t.Fatalf("negative entry should expire after 5s, calls=%d", src.calls)
	}
}

func TestResolverRejectsInactive(t *testing.T) {
	for _, st := range []platform.Status{platform.StatusSuspended, platform.StatusDeleted} {
		src := &fakeSource{clinics: map[string]platform.Clinic{"x": {Slug: "x", Status: st}}}
		r, _ := newResolver(src)
		if _, err := r.Resolve(context.Background(), "x"); !errors.Is(err, platform.ErrTenantUnavailable) {
			t.Errorf("status %s: want ErrTenantUnavailable, got %v", st, err)
		}
	}
}

func TestTenantContext(t *testing.T) {
	ctx := context.Background()
	if _, ok := platform.TenantFrom(ctx); ok {
		t.Fatal("empty context must have no tenant")
	}
	c := platform.Clinic{Slug: "uno"}
	got, ok := platform.TenantFrom(platform.WithTenant(ctx, c))
	if !ok || got.Slug != "uno" {
		t.Fatalf("TenantFrom: %v %+v", ok, got)
	}
}
