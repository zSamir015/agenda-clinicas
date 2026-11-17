// Package rbac defines the fixed permission catalog, clinic-scoped roles,
// and anti-escalation rules for staff authorization.
package rbac

import (
	"errors"
	"fmt"
	"slices"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// Permission identifies a single capability in the system.
type Permission string

// Fixed permission catalog.
const (
	PermAppointmentsRead  Permission = "appointments.read"
	PermAppointmentsWrite Permission = "appointments.write"
	PermScheduleManage    Permission = "schedule.manage"
	PermPatientsRead      Permission = "patients.read"
	PermPatientsWrite     Permission = "patients.write"
	PermPatientsExport    Permission = "patients.export"
	PermPatientsDelete    Permission = "patients.delete"
	PermPaymentsRead      Permission = "payments.read"
	PermPaymentsRefund    Permission = "payments.refund"
	PermUsersManage       Permission = "users.manage"
	PermRolesManage       Permission = "roles.manage"
	PermClinicSettings    Permission = "clinic.settings"
)

// Scope bounds how broadly a permission applies within a clinic.
type Scope string

const (
	// ScopeOwn limits access to records assigned to the acting user.
	ScopeOwn Scope = "own"
	// ScopeAll grants access across the whole clinic.
	ScopeAll Scope = "all"
)

// Grant pairs a permission with its scope inside a role.
type Grant struct {
	Permission Permission `bson:"permission" json:"permission"`
	Scope      Scope      `bson:"scope" json:"scope"`
}

var (
	// ErrForbidden is returned when the caller lacks the required permission or scope.
	ErrForbidden = errors.New("rbac: forbidden")
	// ErrInvalidPermission is returned for unknown permission identifiers.
	ErrInvalidPermission = errors.New("rbac: invalid permission")
	// ErrInvalidScope is returned for unknown scope values.
	ErrInvalidScope = errors.New("rbac: invalid scope")
	// ErrPrivilegeEscalation is returned when a user tries to grant a permission
	// or scope they do not hold themselves.
	ErrPrivilegeEscalation = errors.New("rbac: cannot grant permission or scope not held")
)

var allPermissions = []Permission{
	PermAppointmentsRead,
	PermAppointmentsWrite,
	PermScheduleManage,
	PermPatientsRead,
	PermPatientsWrite,
	PermPatientsExport,
	PermPatientsDelete,
	PermPaymentsRead,
	PermPaymentsRefund,
	PermUsersManage,
	PermRolesManage,
	PermClinicSettings,
}

// AllPermissions returns a copy of the fixed permission catalog.
func AllPermissions() []Permission {
	return slices.Clone(allPermissions)
}

// ValidPermission reports whether p belongs to the fixed catalog.
func ValidPermission(p Permission) bool {
	return slices.Contains(allPermissions, p)
}

// ValidScope reports whether s is a supported scope.
func ValidScope(s Scope) bool {
	return s == ScopeOwn || s == ScopeAll
}

// ScopeCovers reports whether held satisfies required.
func ScopeCovers(held, required Scope) bool {
	if held == ScopeAll {
		return required == ScopeAll || required == ScopeOwn
	}
	return held == ScopeOwn && required == ScopeOwn
}

// Authorize checks whether grants satisfy the required permission and scope.
func Authorize(grants []Grant, perm Permission, required Scope) error {
	for _, g := range grants {
		if g.Permission == perm && ScopeCovers(g.Scope, required) {
			return nil
		}
	}
	return fmt.Errorf("%w: %s (%s)", ErrForbidden, perm, required)
}

// ValidateGrants verifies that every grant uses a known permission and scope
// without duplicate permissions.
func ValidateGrants(grants []Grant) error {
	seen := make(map[Permission]struct{}, len(grants))
	for _, g := range grants {
		if !ValidPermission(g.Permission) {
			return fmt.Errorf("%w: %q", ErrInvalidPermission, g.Permission)
		}
		if !ValidScope(g.Scope) {
			return fmt.Errorf("%w: %q", ErrInvalidScope, g.Scope)
		}
		if _, dup := seen[g.Permission]; dup {
			return fmt.Errorf("%w: duplicate permission %q", ErrInvalidPermission, g.Permission)
		}
		seen[g.Permission] = struct{}{}
	}
	return nil
}

// ValidateEscalation rejects targetGrants if any permission or scope exceeds actorGrants.
func ValidateEscalation(actorGrants, targetGrants []Grant) error {
	if err := ValidateGrants(targetGrants); err != nil {
		return err
	}
	held := make(map[Permission]Scope, len(actorGrants))
	for _, g := range actorGrants {
		if cur, ok := held[g.Permission]; !ok || (cur == ScopeOwn && g.Scope == ScopeAll) {
			held[g.Permission] = g.Scope
		}
	}
	for _, target := range targetGrants {
		actorScope, ok := held[target.Permission]
		if !ok || !ScopeCovers(actorScope, target.Scope) {
			return fmt.Errorf("%w: %s (%s)", ErrPrivilegeEscalation, target.Permission, target.Scope)
		}
	}
	return nil
}

// CheckLockedRoleAccess rejects assigning, inviting into, or removing holders
// of a locked role (owner) unless the actor holds that same role. Identical
// grant sets are not enough: the admin template grants everything the owner
// has, so ValidateEscalation alone lets an admin become owner.
func CheckLockedRoleAccess(actorRoleID bson.ObjectID, role Role) error {
	if role.Locked && role.ID != actorRoleID {
		return fmt.Errorf("%w: role %q is reserved to its holders", ErrPrivilegeEscalation, role.Name)
	}
	return nil
}
