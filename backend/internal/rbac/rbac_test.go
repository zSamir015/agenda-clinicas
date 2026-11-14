package rbac_test

import (
	"context"
	"errors"
	"testing"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb/mongotest"
	"github.com/zSamir015/agenda-clinicas/backend/internal/rbac"
)

func TestAuthorizeAndScope(t *testing.T) {
	grants := []rbac.Grant{
		{Permission: rbac.PermPatientsRead, Scope: rbac.ScopeAll},
		{Permission: rbac.PermAppointmentsWrite, Scope: rbac.ScopeOwn},
	}

	if err := rbac.Authorize(grants, rbac.PermPatientsRead, rbac.ScopeAll); err != nil {
		t.Fatalf("expected all scope to cover all: %v", err)
	}
	if err := rbac.Authorize(grants, rbac.PermPatientsRead, rbac.ScopeOwn); err != nil {
		t.Fatalf("expected all scope to cover own: %v", err)
	}
	if err := rbac.Authorize(grants, rbac.PermAppointmentsWrite, rbac.ScopeOwn); err != nil {
		t.Fatalf("expected own scope to cover own: %v", err)
	}
	if err := rbac.Authorize(grants, rbac.PermAppointmentsWrite, rbac.ScopeAll); !errors.Is(err, rbac.ErrForbidden) {
		t.Fatalf("own scope covering all: got %v, want ErrForbidden", err)
	}
	if err := rbac.Authorize(grants, rbac.PermPaymentsRefund, rbac.ScopeOwn); !errors.Is(err, rbac.ErrForbidden) {
		t.Fatalf("missing permission: got %v, want ErrForbidden", err)
	}
}

func TestValidateEscalation(t *testing.T) {
	actor := []rbac.Grant{
		{Permission: rbac.PermAppointmentsRead, Scope: rbac.ScopeOwn},
		{Permission: rbac.PermRolesManage, Scope: rbac.ScopeAll},
	}

	t.Run("allows subset of permissions and scopes", func(t *testing.T) {
		err := rbac.ValidateEscalation(actor, []rbac.Grant{
			{Permission: rbac.PermAppointmentsRead, Scope: rbac.ScopeOwn},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("rejects unheld permission", func(t *testing.T) {
		err := rbac.ValidateEscalation(actor, []rbac.Grant{
			{Permission: rbac.PermPaymentsRefund, Scope: rbac.ScopeAll},
		})
		if !errors.Is(err, rbac.ErrPrivilegeEscalation) {
			t.Fatalf("got %v, want ErrPrivilegeEscalation", err)
		}
	})

	t.Run("rejects escalating scope from own to all", func(t *testing.T) {
		err := rbac.ValidateEscalation(actor, []rbac.Grant{
			{Permission: rbac.PermAppointmentsRead, Scope: rbac.ScopeAll},
		})
		if !errors.Is(err, rbac.ErrPrivilegeEscalation) {
			t.Fatalf("got %v, want ErrPrivilegeEscalation", err)
		}
	})

	t.Run("rejects unknown permission or scope", func(t *testing.T) {
		err := rbac.ValidateEscalation(actor, []rbac.Grant{
			{Permission: rbac.Permission("nope.do"), Scope: rbac.ScopeOwn},
		})
		if !errors.Is(err, rbac.ErrInvalidPermission) {
			t.Fatalf("got %v, want ErrInvalidPermission", err)
		}

		err = rbac.ValidateEscalation(actor, []rbac.Grant{
			{Permission: rbac.PermAppointmentsRead, Scope: rbac.Scope("global")},
		})
		if !errors.Is(err, rbac.ErrInvalidScope) {
			t.Fatalf("got %v, want ErrInvalidScope", err)
		}
	})
}

func TestRoleRepositoryIntegration(t *testing.T) {
	ctx := context.Background()
	cl, err := mongodb.Connect(ctx, mongotest.URI(t))
	if err != nil {
		t.Fatalf("mongodb.Connect: %v", err)
	}
	t.Cleanup(func() { _ = cl.Close(context.Background()) })

	repo, err := rbac.NewRoleRepository(ctx, cl.Mongo().Database("platform"))
	if err != nil {
		t.Fatalf("NewRoleRepository: %v", err)
	}

	clinicA := bson.NewObjectID()
	clinicB := bson.NewObjectID()

	if err := repo.EnsureTemplates(ctx, clinicA); err != nil {
		t.Fatalf("EnsureTemplates: %v", err)
	}
	if err := repo.EnsureTemplates(ctx, clinicA); err != nil {
		t.Fatalf("EnsureTemplates idempotent: %v", err)
	}

	rolesA, err := repo.ListByClinic(ctx, clinicA)
	if err != nil {
		t.Fatalf("ListByClinic A: %v", err)
	}
	if len(rolesA) != 4 {
		t.Fatalf("expected 4 template roles, got %d", len(rolesA))
	}

	var ownerRole, practitionerRole rbac.Role
	for _, r := range rolesA {
		if r.Name == rbac.RoleOwner {
			ownerRole = r
		}
		if r.Name == rbac.RolePractitioner {
			practitionerRole = r
		}
	}
	if !ownerRole.Locked || len(ownerRole.Grants) != len(rbac.AllPermissions()) {
		t.Fatalf("unexpected owner role: %+v", ownerRole)
	}

	t.Run("owner role cannot be updated or deleted", func(t *testing.T) {
		if _, err := repo.Update(ctx, ownerRole.Grants, clinicA, ownerRole.ID, "owner2", ownerRole.Grants); !errors.Is(err, rbac.ErrLockedRole) {
			t.Fatalf("Update owner: got %v, want ErrLockedRole", err)
		}
		if err := repo.Delete(ctx, clinicA, ownerRole.ID); !errors.Is(err, rbac.ErrLockedRole) {
			t.Fatalf("Delete owner: got %v, want ErrLockedRole", err)
		}
	})

	t.Run("create custom role enforces anti-escalation and uniqueness", func(t *testing.T) {
		if _, err := repo.Create(ctx, practitionerRole.Grants, clinicA, "assistant", []rbac.Grant{
			{Permission: rbac.PermPaymentsRefund, Scope: rbac.ScopeAll},
		}); !errors.Is(err, rbac.ErrPrivilegeEscalation) {
			t.Fatalf("escalation create: got %v, want ErrPrivilegeEscalation", err)
		}

		custom, err := repo.Create(ctx, ownerRole.Grants, clinicA, "coordinator", []rbac.Grant{
			{Permission: rbac.PermAppointmentsRead, Scope: rbac.ScopeAll},
			{Permission: rbac.PermScheduleManage, Scope: rbac.ScopeAll},
		})
		if err != nil {
			t.Fatalf("Create custom: %v", err)
		}

		if _, err := repo.Create(ctx, ownerRole.Grants, clinicA, "coordinator", custom.Grants); !errors.Is(err, rbac.ErrRoleTaken) {
			t.Fatalf("duplicate role: got %v, want ErrRoleTaken", err)
		}

		updated, err := repo.Update(ctx, ownerRole.Grants, clinicA, custom.ID, "coordinator-sr", []rbac.Grant{
			{Permission: rbac.PermAppointmentsRead, Scope: rbac.ScopeAll},
		})
		if err != nil || updated.Name != "coordinator-sr" || len(updated.Grants) != 1 {
			t.Fatalf("Update custom: updated=%+v, err=%v", updated, err)
		}

		// Clinic B cannot see, update, or delete Clinic A's role.
		if _, err := repo.ByID(ctx, clinicB, custom.ID); !errors.Is(err, rbac.ErrRoleNotFound) {
			t.Fatalf("cross-clinic ByID: got %v, want ErrRoleNotFound", err)
		}
		if err := repo.Delete(ctx, clinicB, custom.ID); !errors.Is(err, rbac.ErrRoleNotFound) {
			t.Fatalf("cross-clinic Delete: got %v, want ErrRoleNotFound", err)
		}

		if err := repo.Delete(ctx, clinicA, custom.ID); err != nil {
			t.Fatalf("Delete custom: %v", err)
		}
	})
}
