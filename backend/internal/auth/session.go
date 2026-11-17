// Package auth implements server-side sessions, CSRF protection, and
// in-memory rate limiting for staff and patient authentication.
package auth

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

	"github.com/zSamir015/agenda-clinicas/backend/internal/rbac"
)

const sessionsCollection = "sessions"

// SessionKind distinguishes staff panel sessions from patient booking sessions.
type SessionKind string

const (
	// SessionStaff identifies an authenticated clinic staff member.
	SessionStaff SessionKind = "staff"
	// SessionPatient identifies a patient authenticated via one-time code.
	SessionPatient SessionKind = "patient"
)

var (
	// ErrSessionInvalid is returned when a session token is missing or unknown.
	ErrSessionInvalid = errors.New("auth: invalid session")
	// ErrSessionExpired is returned when a session has passed its expiration time.
	ErrSessionExpired = errors.New("auth: session expired")
	// ErrSessionRevoked is returned when a session was explicitly revoked.
	ErrSessionRevoked = errors.New("auth: session revoked")
)

// Session represents a server-side session stored in platform.sessions.
type Session struct {
	ID             bson.ObjectID `bson:"_id,omitempty"`
	TokenHash      string        `bson:"tokenHash"`
	CSRFToken      string        `bson:"csrfToken"`
	Kind           SessionKind   `bson:"kind"`
	UserID         bson.ObjectID `bson:"userId,omitempty"`
	ClinicID       bson.ObjectID `bson:"clinicId"`
	RoleID         bson.ObjectID `bson:"roleId,omitempty"`
	Grants         []rbac.Grant  `bson:"grants,omitempty"`
	PatientContact string        `bson:"patientContact,omitempty"`
	ExpiresAt      time.Time     `bson:"expiresAt"`
	RevokedAt      *time.Time    `bson:"revokedAt,omitempty"`
	CreatedAt      time.Time     `bson:"createdAt"`
	LastSeenAt     time.Time     `bson:"lastSeenAt"`
}

// SessionRepository manages sessions in platform.sessions.
type SessionRepository struct {
	col *mongo.Collection
	now func() time.Time
}

// NewSessionRepository initializes platform.sessions indexes and returns a repository.
func NewSessionRepository(ctx context.Context, platformDB *mongo.Database) (*SessionRepository, error) {
	col := platformDB.Collection(sessionsCollection)
	models := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "tokenHash", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("token_hash_unique"),
		},
		{
			Keys:    bson.D{{Key: "userId", Value: 1}},
			Options: options.Index().SetName("user_id_idx"),
		},
		{
			Keys:    bson.D{{Key: "clinicId", Value: 1}, {Key: "roleId", Value: 1}},
			Options: options.Index().SetName("clinic_role_idx"),
		},
	}
	if _, err := col.Indexes().CreateMany(ctx, models); err != nil {
		return nil, fmt.Errorf("create sessions indexes: %w", err)
	}
	return &SessionRepository{
		col: col,
		now: func() time.Time { return time.Now().UTC().Truncate(time.Millisecond) },
	}, nil
}

// CreateStaffSession creates a new server-side session for a clinic staff member.
func (r *SessionRepository) CreateStaffSession(ctx context.Context, userID, clinicID, roleID bson.ObjectID, grants []rbac.Grant, ttl time.Duration) (Session, string, error) {
	if userID.IsZero() || clinicID.IsZero() || roleID.IsZero() || ttl <= 0 {
		return Session{}, "", ErrSessionInvalid
	}
	return r.create(ctx, Session{
		Kind:     SessionStaff,
		UserID:   userID,
		ClinicID: clinicID,
		RoleID:   roleID,
		Grants:   grants,
	}, ttl)
}

// CreatePatientSession creates a short-lived session for a patient scoped to clinicID.
func (r *SessionRepository) CreatePatientSession(ctx context.Context, clinicID bson.ObjectID, contact string, ttl time.Duration) (Session, string, error) {
	contact = strings.TrimSpace(contact)
	if clinicID.IsZero() || contact == "" || ttl <= 0 {
		return Session{}, "", ErrSessionInvalid
	}
	return r.create(ctx, Session{
		Kind:           SessionPatient,
		ClinicID:       clinicID,
		PatientContact: contact,
	}, ttl)
}

func (r *SessionRepository) create(ctx context.Context, s Session, ttl time.Duration) (Session, string, error) {
	rawToken, err := randomToken(32)
	if err != nil {
		return Session{}, "", fmt.Errorf("generate session token: %w", err)
	}
	csrfToken, err := randomToken(24)
	if err != nil {
		return Session{}, "", fmt.Errorf("generate csrf token: %w", err)
	}

	now := r.now()
	s.TokenHash = hashToken(rawToken)
	s.CSRFToken = csrfToken
	s.CreatedAt = now
	s.LastSeenAt = now
	s.ExpiresAt = now.Add(ttl)

	res, err := r.col.InsertOne(ctx, s)
	if err != nil {
		return Session{}, "", fmt.Errorf("insert session: %w", err)
	}
	s.ID = res.InsertedID.(bson.ObjectID)
	return s, rawToken, nil
}

// Validate checks a raw session token and returns the active session.
func (r *SessionRepository) Validate(ctx context.Context, rawToken string) (Session, error) {
	if strings.TrimSpace(rawToken) == "" {
		return Session{}, ErrSessionInvalid
	}
	var s Session
	err := r.col.FindOne(ctx, bson.D{{Key: "tokenHash", Value: hashToken(rawToken)}}).Decode(&s)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return Session{}, ErrSessionInvalid
	}
	if err != nil {
		return Session{}, fmt.Errorf("find session: %w", err)
	}
	if s.RevokedAt != nil {
		return Session{}, ErrSessionRevoked
	}
	if !s.ExpiresAt.After(r.now()) {
		return Session{}, ErrSessionExpired
	}
	return s, nil
}

// Revoke immediately invalidates the session identified by rawToken.
func (r *SessionRepository) Revoke(ctx context.Context, rawToken string) error {
	if strings.TrimSpace(rawToken) == "" {
		return ErrSessionInvalid
	}
	now := r.now()
	res, err := r.col.UpdateOne(
		ctx,
		bson.D{{Key: "tokenHash", Value: hashToken(rawToken)}, {Key: "revokedAt", Value: bson.D{{Key: "$exists", Value: false}}}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "revokedAt", Value: now}}}},
	)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	if res.MatchedCount == 0 {
		return ErrSessionInvalid
	}
	return nil
}

// RevokeByUser revokes all active sessions belonging to userID.
func (r *SessionRepository) RevokeByUser(ctx context.Context, userID bson.ObjectID) error {
	now := r.now()
	_, err := r.col.UpdateMany(
		ctx,
		bson.D{{Key: "userId", Value: userID}, {Key: "revokedAt", Value: bson.D{{Key: "$exists", Value: false}}}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "revokedAt", Value: now}}}},
	)
	if err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}
	return nil
}

// RevokeOthers revokes every active session of userID except the one
// identified by keepRawToken.
func (r *SessionRepository) RevokeOthers(ctx context.Context, userID bson.ObjectID, keepRawToken string) error {
	now := r.now()
	_, err := r.col.UpdateMany(
		ctx,
		bson.D{
			{Key: "userId", Value: userID},
			{Key: "tokenHash", Value: bson.D{{Key: "$ne", Value: hashToken(keepRawToken)}}},
			{Key: "revokedAt", Value: bson.D{{Key: "$exists", Value: false}}},
		},
		bson.D{{Key: "$set", Value: bson.D{{Key: "revokedAt", Value: now}}}},
	)
	if err != nil {
		return fmt.Errorf("revoke other sessions: %w", err)
	}
	return nil
}

// InvalidateRoleCache updates the cached grants on all active sessions
// bound to (clinicID, roleID) when a role's permissions change.
func (r *SessionRepository) InvalidateRoleCache(ctx context.Context, clinicID, roleID bson.ObjectID, newGrants []rbac.Grant) error {
	_, err := r.col.UpdateMany(
		ctx,
		bson.D{
			{Key: "clinicId", Value: clinicID},
			{Key: "roleId", Value: roleID},
			{Key: "revokedAt", Value: bson.D{{Key: "$exists", Value: false}}},
		},
		bson.D{{Key: "$set", Value: bson.D{{Key: "grants", Value: newGrants}}}},
	)
	if err != nil {
		return fmt.Errorf("invalidate role cache: %w", err)
	}
	return nil
}

func randomToken(numBytes int) (string, error) {
	b := make([]byte, numBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
