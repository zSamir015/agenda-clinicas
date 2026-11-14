package rbac

import (
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Template role names created for every new clinic.
const (
	RoleOwner        = "owner"
	RoleAdmin        = "admin"
	RoleReception    = "reception"
	RolePractitioner = "practitioner"
)

var (
	// ErrRoleNotFound is returned when a role does not exist in the clinic.
	ErrRoleNotFound = errors.New("rbac: role not found")
	// ErrRoleTaken is returned when another role in the clinic already uses the name.
	ErrRoleTaken = errors.New("rbac: role name already exists")
	// ErrLockedRole is returned when attempting to modify or delete the locked owner role.
	ErrLockedRole = errors.New("rbac: locked role cannot be modified or deleted")
	// ErrInvalidRole is returned when role metadata (such as name) is invalid.
	ErrInvalidRole = errors.New("rbac: invalid role")
)

// Role defines a clinic-scoped set of permission grants stored in platform.roles.
type Role struct {
	ID        bson.ObjectID `bson:"_id,omitempty"`
	ClinicID  bson.ObjectID `bson:"clinicId"`
	Name      string        `bson:"name"`
	Template  bool          `bson:"template"`
	Locked    bool          `bson:"locked"`
	Grants    []Grant       `bson:"grants"`
	CreatedAt time.Time     `bson:"createdAt"`
	UpdatedAt time.Time     `bson:"updatedAt"`
}

// TemplateRoles returns the four default roles for a clinic.
func TemplateRoles(clinicID bson.ObjectID, now time.Time) []Role {
	all := make([]Grant, 0, len(allPermissions))
	for _, p := range allPermissions {
		all = append(all, Grant{Permission: p, Scope: ScopeAll})
	}

	adminGrants := make([]Grant, len(all))
	copy(adminGrants, all)

	return []Role{
		{
			ClinicID:  clinicID,
			Name:      RoleOwner,
			Template:  true,
			Locked:    true,
			Grants:    all,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ClinicID:  clinicID,
			Name:      RoleAdmin,
			Template:  true,
			Locked:    false,
			Grants:    adminGrants,
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ClinicID: clinicID,
			Name:     RoleReception,
			Template: true,
			Locked:   false,
			Grants: []Grant{
				{Permission: PermAppointmentsRead, Scope: ScopeAll},
				{Permission: PermAppointmentsWrite, Scope: ScopeAll},
				{Permission: PermScheduleManage, Scope: ScopeAll},
				{Permission: PermPatientsRead, Scope: ScopeAll},
				{Permission: PermPatientsWrite, Scope: ScopeAll},
				{Permission: PermPaymentsRead, Scope: ScopeAll},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
		{
			ClinicID: clinicID,
			Name:     RolePractitioner,
			Template: true,
			Locked:   false,
			Grants: []Grant{
				{Permission: PermAppointmentsRead, Scope: ScopeOwn},
				{Permission: PermAppointmentsWrite, Scope: ScopeOwn},
				{Permission: PermScheduleManage, Scope: ScopeOwn},
				{Permission: PermPatientsRead, Scope: ScopeOwn},
				{Permission: PermPatientsWrite, Scope: ScopeOwn},
			},
			CreatedAt: now,
			UpdatedAt: now,
		},
	}
}
