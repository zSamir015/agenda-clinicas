package platform

import (
	"context"
	"errors"
	"sync"
	"time"
)

const (
	hitTTL  = 60 * time.Second // found clinics: a status change reaches every process within a minute
	missTTL = 5 * time.Second  // unknown slugs: short, so invented slugs do not hammer the database
)

// ClinicSource is what the resolver needs from the registry.
type ClinicSource interface {
	BySlug(ctx context.Context, slug string) (Clinic, error)
}

type entry struct {
	clinic  Clinic
	err     error
	expires time.Time
}

// TenantResolver turns a slug into an active clinic, caching the answer.
type TenantResolver struct {
	src   ClinicSource
	now   func() time.Time
	mu    sync.Mutex
	cache map[string]entry
}

// Option configures a TenantResolver.
type Option func(*TenantResolver)

// WithClock replaces the clock; tests use it to expire entries without sleeping.
func WithClock(now func() time.Time) Option { return func(r *TenantResolver) { r.now = now } }

// NewTenantResolver returns a resolver backed by src.
func NewTenantResolver(src ClinicSource, opts ...Option) *TenantResolver {
	r := &TenantResolver{src: src, now: time.Now, cache: map[string]entry{}}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Resolve returns the active clinic for slug. Unknown slugs give
// ErrTenantNotFound; suspended or deleted clinics give ErrTenantUnavailable.
func (r *TenantResolver) Resolve(ctx context.Context, slug string) (Clinic, error) {
	now := r.now()
	r.mu.Lock()
	e, ok := r.cache[slug]
	r.mu.Unlock()
	if ok && now.Before(e.expires) {
		return e.clinic, e.err
	}

	c, err := r.src.BySlug(ctx, slug)
	ttl := hitTTL
	switch {
	case errors.Is(err, ErrTenantNotFound):
		ttl = missTTL
	case err != nil:
		// Transient failures (network, timeout) are never cached.
		return Clinic{}, err
	case c.Status != StatusActive:
		err = ErrTenantUnavailable
	}
	if err != nil {
		c = Clinic{}
	}
	r.mu.Lock()
	r.cache[slug] = entry{clinic: c, err: err, expires: now.Add(ttl)}
	r.mu.Unlock()
	return c, err
}
