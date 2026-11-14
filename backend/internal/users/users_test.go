package users_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb/mongotest"
	"github.com/zSamir015/agenda-clinicas/backend/internal/users"
)

func TestUsersMembershipsAndInvitationsIntegration(t *testing.T) {
	ctx := context.Background()
	cl, err := mongodb.Connect(ctx, mongotest.URI(t))
	if err != nil {
		t.Fatalf("mongodb.Connect: %v", err)
	}
	t.Cleanup(func() { _ = cl.Close(context.Background()) })

	repo, err := users.NewRepository(ctx, cl.Mongo().Database("platform"))
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}

	t.Run("email normalization and uniqueness", func(t *testing.T) {
		if _, err := repo.CreateUser(ctx, "not-an-email", "Bad"); !errors.Is(err, users.ErrInvalidEmail) {
			t.Fatalf("invalid email: got %v, want ErrInvalidEmail", err)
		}

		u, err := repo.CreateUser(ctx, "  Dra.Ana@Demo.Example ", "Dra. Ana Pérez")
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		if u.Email != "dra.ana@demo.example" {
			t.Fatalf("normalized email got %q", u.Email)
		}

		if _, err := repo.CreateUser(ctx, "dra.ana@demo.example", "Dup"); !errors.Is(err, users.ErrEmailTaken) {
			t.Fatalf("duplicate email: got %v, want ErrEmailTaken", err)
		}

		loaded, err := repo.UserByEmail(ctx, "DRA.ANA@demo.example")
		if err != nil || loaded.ID != u.ID {
			t.Fatalf("UserByEmail: loaded=%+v, err=%v", loaded, err)
		}
	})

	t.Run("multi-clinic memberships and last-owner protection", func(t *testing.T) {
		clinicA := bson.NewObjectID()
		clinicB := bson.NewObjectID()
		ownerRoleA := bson.NewObjectID()
		adminRoleA := bson.NewObjectID()
		receptionRoleB := bson.NewObjectID()

		u1, err := repo.CreateUser(ctx, "owner1@demo.example", "Owner Uno")
		if err != nil {
			t.Fatalf("CreateUser u1: %v", err)
		}
		u2, err := repo.CreateUser(ctx, "owner2@demo.example", "Owner Dos")
		if err != nil {
			t.Fatalf("CreateUser u2: %v", err)
		}

		if _, err := repo.AddMembership(ctx, u1.ID, clinicA, ownerRoleA); err != nil {
			t.Fatalf("AddMembership u1->A: %v", err)
		}
		if _, err := repo.AddMembership(ctx, u1.ID, clinicB, receptionRoleB); err != nil {
			t.Fatalf("AddMembership u1->B: %v", err)
		}

		memberships, err := repo.ListMembershipsByUser(ctx, u1.ID)
		if err != nil || len(memberships) != 2 {
			t.Fatalf("expected 2 clinic memberships for u1, got %d (err=%v)", len(memberships), err)
		}

		// Demoting or revoking the sole owner must fail.
		if _, err := repo.UpdateMembershipRole(ctx, clinicA, u1.ID, adminRoleA, ownerRoleA); !errors.Is(err, users.ErrLastOwner) {
			t.Fatalf("demote last owner: got %v, want ErrLastOwner", err)
		}
		if err := repo.RevokeMembership(ctx, clinicA, u1.ID, ownerRoleA); !errors.Is(err, users.ErrLastOwner) {
			t.Fatalf("revoke last owner: got %v, want ErrLastOwner", err)
		}

		// Adding a second owner allows demoting the first one.
		if _, err := repo.AddMembership(ctx, u2.ID, clinicA, ownerRoleA); err != nil {
			t.Fatalf("AddMembership u2->A: %v", err)
		}
		updated, err := repo.UpdateMembershipRole(ctx, clinicA, u1.ID, adminRoleA, ownerRoleA)
		if err != nil || updated.RoleID != adminRoleA {
			t.Fatalf("demote after second owner: updated=%+v, err=%v", updated, err)
		}
	})

	t.Run("one-time hashed invitations", func(t *testing.T) {
		clinicID := bson.NewObjectID()
		roleID := bson.NewObjectID()
		inviterID := bson.NewObjectID()

		inv, rawToken, err := repo.CreateInvitation(ctx, clinicID, "Nuevo@Demo.Example", "Dr. Nuevo", roleID, inviterID, time.Hour)
		if err != nil {
			t.Fatalf("CreateInvitation: %v", err)
		}
		if rawToken == "" || inv.TokenHash == rawToken {
			t.Fatal("expected hashed token different from rawToken")
		}

		u, m, err := repo.AcceptInvitation(ctx, rawToken)
		if err != nil {
			t.Fatalf("AcceptInvitation: %v", err)
		}
		if u.Email != "nuevo@demo.example" || m.ClinicID != clinicID || m.RoleID != roleID {
			t.Fatalf("unexpected user/membership: u=%+v m=%+v", u, m)
		}

		// Second use of the same one-time token must fail.
		if _, _, err := repo.AcceptInvitation(ctx, rawToken); !errors.Is(err, users.ErrInvitationUsed) {
			t.Fatalf("reused invitation: got %v, want ErrInvitationUsed", err)
		}

		// Expired invitation fails.
		_, expiredRaw, err := repo.CreateInvitation(ctx, clinicID, "exp@demo.example", "Exp", roleID, inviterID, time.Millisecond)
		if err != nil {
			t.Fatalf("CreateInvitation expired: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
		if _, _, err := repo.AcceptInvitation(ctx, expiredRaw); !errors.Is(err, users.ErrInvitationExpired) {
			t.Fatalf("expired invitation: got %v, want ErrInvitationExpired", err)
		}
	})
}
