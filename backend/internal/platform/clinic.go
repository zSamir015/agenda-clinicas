// Package platform is the registry of clinics: who exists, in which isolation
// tier and in which state. Every request starts here, because the clinic found
// in this registry decides which data the rest of the code may touch.
package platform

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Tier is the isolation level of a clinic (ADR 0002).
type Tier string

const (
	// TierStandard shares the operations database, filtered by clinicId.
	TierStandard Tier = "standard"
	// TierDedicated gives the clinic its own database.
	TierDedicated Tier = "dedicated"
)

// Status is the lifecycle state of a clinic.
type Status string

const (
	// StatusActive is the only state that can serve requests.
	StatusActive Status = "active"
	// StatusSuspended blocks access but keeps data (for example, unpaid plan).
	StatusSuspended Status = "suspended"
	// StatusDeleted marks a clinic whose data key was destroyed.
	StatusDeleted Status = "deleted"
)

// Errors callers can test with errors.Is.
var (
	ErrInvalidSlug       = errors.New("platform: invalid slug")
	ErrInvalidClinic     = errors.New("platform: invalid clinic")
	ErrSlugTaken         = errors.New("platform: slug already taken")
	ErrTenantNotFound    = errors.New("platform: clinic not found")
	ErrTenantUnavailable = errors.New("platform: clinic not active")
)

// The slug becomes a subdomain, so it follows DNS label rules: lowercase
// letters, digits and inner hyphens, 3 to 40 characters.
var slugRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{1,38})[a-z0-9]$`)

// Clinic is one tenant of the platform.
type Clinic struct {
	ID        bson.ObjectID `bson:"_id,omitempty"`
	Slug      string        `bson:"slug"`
	Name      string        `bson:"name"`
	Tier      Tier          `bson:"tier"`
	Region    string        `bson:"region"`
	Status    Status        `bson:"status"`
	CreatedAt time.Time     `bson:"createdAt"`
}

// NewClinic validates the input and returns an active clinic ready to store.
func NewClinic(slug, name string, tier Tier, region string) (Clinic, error) {
	if !slugRe.MatchString(slug) {
		return Clinic{}, fmt.Errorf("%w: %q", ErrInvalidSlug, slug)
	}
	if name == "" || region == "" {
		return Clinic{}, fmt.Errorf("%w: name and region are required", ErrInvalidClinic)
	}
	if tier != TierStandard && tier != TierDedicated {
		return Clinic{}, fmt.Errorf("%w: unknown tier %q", ErrInvalidClinic, tier)
	}
	return Clinic{Slug: slug, Name: name, Tier: tier, Region: region, Status: StatusActive}, nil
}
