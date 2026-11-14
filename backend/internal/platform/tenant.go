package platform

import "context"

// tenantKey is private so only WithTenant can write the value and nothing
// else in the program can forge or overwrite a tenant.
type tenantKey struct{}

// WithTenant returns a context that carries the resolved clinic.
func WithTenant(ctx context.Context, c Clinic) context.Context {
	return context.WithValue(ctx, tenantKey{}, c)
}

// TenantFrom returns the clinic of the request. There is deliberately no
// "must" variant: callers decide what a missing tenant means.
func TenantFrom(ctx context.Context) (Clinic, bool) {
	c, ok := ctx.Value(tenantKey{}).(Clinic)
	return c, ok
}
