package users

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	usersCollection       = "users"
	membershipsCollection = "memberships"
	invitationsCollection = "invitations"
)

// Repository manages users, clinic memberships, and invitations in platform.
type Repository struct {
	users       *mongo.Collection
	memberships *mongo.Collection
	invitations *mongo.Collection
	now         func() time.Time
}

// NewRepository initializes the collections and ensures their unique indexes exist.
func NewRepository(ctx context.Context, platformDB *mongo.Database) (*Repository, error) {
	u := platformDB.Collection(usersCollection)
	if _, err := u.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("email_unique"),
	}); err != nil {
		return nil, fmt.Errorf("create users email index: %w", err)
	}

	m := platformDB.Collection(membershipsCollection)
	if _, err := m.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "userId", Value: 1}, {Key: "clinicId", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("user_clinic_unique"),
	}); err != nil {
		return nil, fmt.Errorf("create memberships unique index: %w", err)
	}

	inv := platformDB.Collection(invitationsCollection)
	if _, err := inv.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "tokenHash", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("token_hash_unique"),
	}); err != nil {
		return nil, fmt.Errorf("create invitations tokenHash index: %w", err)
	}

	return &Repository{
		users:       u,
		memberships: m,
		invitations: inv,
		now:         func() time.Time { return time.Now().UTC().Truncate(time.Millisecond) },
	}, nil
}

// CreateUser registers a new active user with a normalized email.
func (r *Repository) CreateUser(ctx context.Context, email, name string) (User, error) {
	norm, err := NormalizeEmail(email)
	if err != nil {
		return User{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return User{}, fmt.Errorf("%w: name is required", ErrInvalidUser)
	}

	now := r.now()
	u := User{
		Email:     norm,
		Name:      name,
		Status:    UserActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	res, err := r.users.InsertOne(ctx, u)
	if mongo.IsDuplicateKeyError(err) {
		return User{}, fmt.Errorf("%w: %s", ErrEmailTaken, norm)
	}
	if err != nil {
		return User{}, fmt.Errorf("insert user: %w", err)
	}
	u.ID = res.InsertedID.(bson.ObjectID)
	return u, nil
}

// UserByEmail returns the user matching the normalized email.
func (r *Repository) UserByEmail(ctx context.Context, email string) (User, error) {
	norm, err := NormalizeEmail(email)
	if err != nil {
		return User{}, err
	}
	var u User
	err = r.users.FindOne(ctx, bson.D{{Key: "email", Value: norm}}).Decode(&u)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("find user by email: %w", err)
	}
	return u, nil
}

// UserByID returns the user with the given ID.
func (r *Repository) UserByID(ctx context.Context, id bson.ObjectID) (User, error) {
	var u User
	err := r.users.FindOne(ctx, bson.D{{Key: "_id", Value: id}}).Decode(&u)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return User{}, ErrUserNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("find user by id: %w", err)
	}
	return u, nil
}

// AddMembership links a user to a clinic with roleID.
func (r *Repository) AddMembership(ctx context.Context, userID, clinicID, roleID bson.ObjectID) (Membership, error) {
	if userID.IsZero() || clinicID.IsZero() || roleID.IsZero() {
		return Membership{}, fmt.Errorf("%w: userId, clinicId, and roleId are required", ErrInvalidUser)
	}
	now := r.now()
	m := Membership{
		UserID:    userID,
		ClinicID:  clinicID,
		RoleID:    roleID,
		Status:    MembershipActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	res, err := r.memberships.InsertOne(ctx, m)
	if mongo.IsDuplicateKeyError(err) {
		return Membership{}, ErrMembershipExists
	}
	if err != nil {
		return Membership{}, fmt.Errorf("insert membership: %w", err)
	}
	m.ID = res.InsertedID.(bson.ObjectID)
	return m, nil
}

// MembershipByUserAndClinic returns the membership for userID in clinicID.
func (r *Repository) MembershipByUserAndClinic(ctx context.Context, userID, clinicID bson.ObjectID) (Membership, error) {
	var m Membership
	err := r.memberships.FindOne(ctx, bson.D{
		{Key: "userId", Value: userID},
		{Key: "clinicId", Value: clinicID},
	}).Decode(&m)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Membership{}, ErrMembershipNotFound
	}
	if err != nil {
		return Membership{}, fmt.Errorf("find membership: %w", err)
	}
	return m, nil
}

// ListMembershipsByUser returns all clinic memberships for userID.
func (r *Repository) ListMembershipsByUser(ctx context.Context, userID bson.ObjectID) ([]Membership, error) {
	cur, err := r.memberships.Find(ctx, bson.D{{Key: "userId", Value: userID}})
	if err != nil {
		return nil, fmt.Errorf("list memberships by user: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()

	var out []Membership
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("decode memberships: %w", err)
	}
	return out, nil
}

// ListMembershipsByClinic returns all memberships belonging to clinicID.
func (r *Repository) ListMembershipsByClinic(ctx context.Context, clinicID bson.ObjectID) ([]Membership, error) {
	cur, err := r.memberships.Find(ctx, bson.D{{Key: "clinicId", Value: clinicID}})
	if err != nil {
		return nil, fmt.Errorf("list memberships by clinic: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()

	var out []Membership
	if err := cur.All(ctx, &out); err != nil {
		return nil, fmt.Errorf("decode memberships: %w", err)
	}
	return out, nil
}

// UpdateMembershipRole changes a member's role, rejecting the change if it
// would demote the last active owner of the clinic.
func (r *Repository) UpdateMembershipRole(ctx context.Context, clinicID, userID, newRoleID, ownerRoleID bson.ObjectID) (Membership, error) {
	current, err := r.MembershipByUserAndClinic(ctx, userID, clinicID)
	if err != nil {
		return Membership{}, err
	}
	if current.RoleID == ownerRoleID && newRoleID != ownerRoleID && current.Status == MembershipActive {
		if err := r.ensureAnotherActiveOwner(ctx, clinicID, userID, ownerRoleID); err != nil {
			return Membership{}, err
		}
	}

	res := r.memberships.FindOneAndUpdate(
		ctx,
		bson.D{{Key: "userId", Value: userID}, {Key: "clinicId", Value: clinicID}},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "roleId", Value: newRoleID},
			{Key: "updatedAt", Value: r.now()},
		}}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var updated Membership
	if err := res.Decode(&updated); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return Membership{}, ErrMembershipNotFound
		}
		return Membership{}, fmt.Errorf("update membership role: %w", err)
	}
	return updated, nil
}

// RevokeMembership suspends a member's access to clinicID, rejecting the
// operation if it would remove the last active owner.
func (r *Repository) RevokeMembership(ctx context.Context, clinicID, userID, ownerRoleID bson.ObjectID) error {
	current, err := r.MembershipByUserAndClinic(ctx, userID, clinicID)
	if err != nil {
		return err
	}
	if current.RoleID == ownerRoleID && current.Status == MembershipActive {
		if err := r.ensureAnotherActiveOwner(ctx, clinicID, userID, ownerRoleID); err != nil {
			return err
		}
	}
	res, err := r.memberships.UpdateOne(
		ctx,
		bson.D{{Key: "userId", Value: userID}, {Key: "clinicId", Value: clinicID}},
		bson.D{{Key: "$set", Value: bson.D{
			{Key: "status", Value: MembershipSuspended},
			{Key: "updatedAt", Value: r.now()},
		}}},
	)
	if err != nil {
		return fmt.Errorf("revoke membership: %w", err)
	}
	if res.MatchedCount == 0 {
		return ErrMembershipNotFound
	}
	return nil
}

func (r *Repository) ensureAnotherActiveOwner(ctx context.Context, clinicID, excludingUserID, ownerRoleID bson.ObjectID) error {
	count, err := r.memberships.CountDocuments(ctx, bson.D{
		{Key: "clinicId", Value: clinicID},
		{Key: "roleId", Value: ownerRoleID},
		{Key: "status", Value: MembershipActive},
		{Key: "userId", Value: bson.D{{Key: "$ne", Value: excludingUserID}}},
	})
	if err != nil {
		return fmt.Errorf("count active owners: %w", err)
	}
	if count == 0 {
		return ErrLastOwner
	}
	return nil
}

// CreateInvitation generates a one-time invitation token, stores only its
// SHA-256 hash in platform.invitations, and returns the record plus raw token.
func (r *Repository) CreateInvitation(ctx context.Context, clinicID bson.ObjectID, email, name string, roleID, invitedBy bson.ObjectID, ttl time.Duration) (Invitation, string, error) {
	norm, err := NormalizeEmail(email)
	if err != nil {
		return Invitation{}, "", err
	}
	name = strings.TrimSpace(name)
	if name == "" || clinicID.IsZero() || roleID.IsZero() || ttl <= 0 {
		return Invitation{}, "", ErrInvitationInvalid
	}

	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		return Invitation{}, "", fmt.Errorf("generate invitation token: %w", err)
	}
	rawToken := base64.RawURLEncoding.EncodeToString(rawBytes)

	now := r.now()
	inv := Invitation{
		ClinicID:  clinicID,
		Email:     norm,
		Name:      name,
		RoleID:    roleID,
		TokenHash: hashToken(rawToken),
		InvitedBy: invitedBy,
		ExpiresAt: now.Add(ttl),
		CreatedAt: now,
	}
	res, err := r.invitations.InsertOne(ctx, inv)
	if err != nil {
		return Invitation{}, "", fmt.Errorf("insert invitation: %w", err)
	}
	inv.ID = res.InsertedID.(bson.ObjectID)
	return inv, rawToken, nil
}

// PreviewInvitation validates rawToken and returns the invitation without consuming it.
func (r *Repository) PreviewInvitation(ctx context.Context, rawToken string) (Invitation, error) {
	inv, _, err := r.findActiveInvitation(ctx, rawToken)
	return inv, err
}

// AcceptInvitation atomically consumes a valid one-time invitation token,
// creates or loads the user by email, and creates their clinic membership.
func (r *Repository) AcceptInvitation(ctx context.Context, rawToken string) (User, Membership, error) {
	existing, now, err := r.findActiveInvitation(ctx, rawToken)
	if err != nil {
		return User{}, Membership{}, err
	}

	res := r.invitations.FindOneAndUpdate(
		ctx,
		bson.D{
			{Key: "tokenHash", Value: existing.TokenHash},
			{Key: "usedAt", Value: bson.D{{Key: "$exists", Value: false}}},
			{Key: "expiresAt", Value: bson.D{{Key: "$gt", Value: now}}},
		},
		bson.D{{Key: "$set", Value: bson.D{{Key: "usedAt", Value: now}}}},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	var consumed Invitation
	if err := res.Decode(&consumed); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return User{}, Membership{}, ErrInvitationUsed
		}
		return User{}, Membership{}, fmt.Errorf("consume invitation: %w", err)
	}

	u, err := r.UserByEmail(ctx, consumed.Email)
	if errors.Is(err, ErrUserNotFound) {
		u, err = r.CreateUser(ctx, consumed.Email, consumed.Name)
	}
	if err != nil {
		return User{}, Membership{}, err
	}

	m, err := r.AddMembership(ctx, u.ID, consumed.ClinicID, consumed.RoleID)
	if err != nil {
		return User{}, Membership{}, err
	}
	return u, m, nil
}

func (r *Repository) findActiveInvitation(ctx context.Context, rawToken string) (Invitation, time.Time, error) {
	if strings.TrimSpace(rawToken) == "" {
		return Invitation{}, time.Time{}, ErrInvitationInvalid
	}
	tokenHash := hashToken(rawToken)
	now := r.now()

	var existing Invitation
	err := r.invitations.FindOne(ctx, bson.D{{Key: "tokenHash", Value: tokenHash}}).Decode(&existing)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Invitation{}, time.Time{}, ErrInvitationInvalid
	}
	if err != nil {
		return Invitation{}, time.Time{}, fmt.Errorf("find invitation: %w", err)
	}
	if existing.UsedAt != nil {
		return Invitation{}, time.Time{}, ErrInvitationUsed
	}
	if !existing.ExpiresAt.After(now) {
		return Invitation{}, time.Time{}, ErrInvitationExpired
	}
	return existing, now, nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
