package rbac

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const rolesCollection = "roles"

// RoleRepository manages clinic-scoped roles in platform.roles.
type RoleRepository struct {
	col *mongo.Collection
}

// NewRoleRepository returns a RoleRepository over platformDB and ensures
// the unique (clinicId, name) index exists.
func NewRoleRepository(ctx context.Context, platformDB *mongo.Database) (*RoleRepository, error) {
	col := platformDB.Collection(rolesCollection)
	_, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "clinicId", Value: 1},
			{Key: "name", Value: 1},
		},
		Options: options.Index().SetUnique(true).SetName("clinic_role_unique"),
	})
	if err != nil {
		return nil, fmt.Errorf("create roles unique index: %w", err)
	}
	return &RoleRepository{col: col}, nil
}

// EnsureTemplates idempotently creates the four template roles for clinicID.
func (r *RoleRepository) EnsureTemplates(ctx context.Context, clinicID bson.ObjectID) error {
	if clinicID.IsZero() {
		return fmt.Errorf("%w: clinicId is required", ErrInvalidRole)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	for _, role := range TemplateRoles(clinicID, now) {
		_, err := r.col.InsertOne(ctx, role)
		if mongo.IsDuplicateKeyError(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("insert template role %s: %w", role.Name, err)
		}
	}
	return nil
}

// Create stores a custom role in clinicID after verifying anti-escalation against actorGrants.
func (r *RoleRepository) Create(ctx context.Context, actorGrants []Grant, clinicID bson.ObjectID, name string, grants []Grant) (Role, error) {
	if clinicID.IsZero() {
		return Role{}, fmt.Errorf("%w: clinicId is required", ErrInvalidRole)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Role{}, fmt.Errorf("%w: name is required", ErrInvalidRole)
	}
	if err := ValidateEscalation(actorGrants, grants); err != nil {
		return Role{}, err
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	role := Role{
		ClinicID:  clinicID,
		Name:      name,
		Template:  false,
		Locked:    false,
		Grants:    grants,
		CreatedAt: now,
		UpdatedAt: now,
	}
	res, err := r.col.InsertOne(ctx, role)
	if mongo.IsDuplicateKeyError(err) {
		return Role{}, fmt.Errorf("%w: %s", ErrRoleTaken, name)
	}
	if err != nil {
		return Role{}, fmt.Errorf("insert role: %w", err)
	}
	role.ID = res.InsertedID.(bson.ObjectID)
	return role, nil
}

// Update modifies a non-locked role's name and grants after anti-escalation checks.
func (r *RoleRepository) Update(ctx context.Context, actorGrants []Grant, clinicID, roleID bson.ObjectID, name string, grants []Grant) (Role, error) {
	current, err := r.ByID(ctx, clinicID, roleID)
	if err != nil {
		return Role{}, err
	}
	if current.Locked {
		return Role{}, ErrLockedRole
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return Role{}, fmt.Errorf("%w: name is required", ErrInvalidRole)
	}
	if err := ValidateEscalation(actorGrants, grants); err != nil {
		return Role{}, err
	}

	now := time.Now().UTC().Truncate(time.Millisecond)
	res := r.col.FindOneAndUpdate(
		ctx,
		bson.D{{Key: "_id", Value: roleID}, {Key: "clinicId", Value: clinicID}},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "name", Value: name},
			{Key: "grants", Value: grants},
			{Key: "updatedAt", Value: now},
		}}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var updated Role
	if err := res.Decode(&updated); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return Role{}, fmt.Errorf("%w: %s", ErrRoleTaken, name)
		}
		if errors.Is(err, mongo.ErrNoDocuments) {
			return Role{}, ErrRoleNotFound
		}
		return Role{}, fmt.Errorf("update role: %w", err)
	}
	return updated, nil
}

// Delete removes a non-locked role belonging to clinicID.
func (r *RoleRepository) Delete(ctx context.Context, clinicID, roleID bson.ObjectID) error {
	current, err := r.ByID(ctx, clinicID, roleID)
	if err != nil {
		return err
	}
	if current.Locked {
		return ErrLockedRole
	}
	res, err := r.col.DeleteOne(ctx, bson.D{{Key: "_id", Value: roleID}, {Key: "clinicId", Value: clinicID}})
	if err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	if res.DeletedCount == 0 {
		return ErrRoleNotFound
	}
	return nil
}

// ByID returns a role scoped to clinicID.
func (r *RoleRepository) ByID(ctx context.Context, clinicID, roleID bson.ObjectID) (Role, error) {
	var role Role
	err := r.col.FindOne(ctx, bson.D{{Key: "_id", Value: roleID}, {Key: "clinicId", Value: clinicID}}).Decode(&role)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Role{}, ErrRoleNotFound
	}
	if err != nil {
		return Role{}, fmt.Errorf("find role: %w", err)
	}
	return role, nil
}

// ListByClinic returns all roles belonging to clinicID ordered by name.
func (r *RoleRepository) ListByClinic(ctx context.Context, clinicID bson.ObjectID) ([]Role, error) {
	cur, err := r.col.Find(ctx, bson.D{{Key: "clinicId", Value: clinicID}}, options.Find().SetSort(bson.D{{Key: "name", Value: 1}}))
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()

	var out []Role
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("decode roles: %w", err)
	}
	return out, nil
}
