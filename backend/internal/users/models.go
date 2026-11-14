// Package users manages staff accounts, clinic memberships, and one-time
// invitations in the platform database.
package users

import (
	"errors"
	"net/mail"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

// UserStatus represents the lifecycle state of a staff account.
type UserStatus string

const (
	// UserActive allows the user to sign in.
	UserActive UserStatus = "active"
	// UserDisabled blocks sign-in across all clinics.
	UserDisabled UserStatus = "disabled"
)

// MembershipStatus represents the state of a user inside one clinic.
type MembershipStatus string

const (
	// MembershipActive allows access to the clinic.
	MembershipActive MembershipStatus = "active"
	// MembershipSuspended revokes access to the clinic without deleting the user.
	MembershipSuspended MembershipStatus = "suspended"
)

var (
	// ErrInvalidEmail is returned when an email address is empty or malformed.
	ErrInvalidEmail = errors.New("users: invalid email")
	// ErrInvalidUser is returned when user fields are missing or invalid.
	ErrInvalidUser = errors.New("users: invalid user")
	// ErrUserNotFound is returned when a user does not exist.
	ErrUserNotFound = errors.New("users: user not found")
	// ErrEmailTaken is returned when an email is already registered.
	ErrEmailTaken = errors.New("users: email already registered")
	// ErrMembershipNotFound is returned when a clinic membership does not exist.
	ErrMembershipNotFound = errors.New("users: membership not found")
	// ErrMembershipExists is returned when the user already belongs to the clinic.
	ErrMembershipExists = errors.New("users: membership already exists")
	// ErrLastOwner is returned when an operation would leave a clinic without an active owner.
	ErrLastOwner = errors.New("users: cannot remove or demote the last clinic owner")
	// ErrInvitationInvalid is returned when an invitation token is unknown or malformed.
	ErrInvitationInvalid = errors.New("users: invalid invitation")
	// ErrInvitationExpired is returned when an invitation is past its expiration time.
	ErrInvitationExpired = errors.New("users: invitation expired")
	// ErrInvitationUsed is returned when an invitation has already been accepted.
	ErrInvitationUsed = errors.New("users: invitation already used")
)

// User represents a staff member in platform.users.
type User struct {
	ID        bson.ObjectID `bson:"_id,omitempty"`
	Email     string        `bson:"email"`
	Name      string        `bson:"name"`
	Status    UserStatus    `bson:"status"`
	CreatedAt time.Time     `bson:"createdAt"`
	UpdatedAt time.Time     `bson:"updatedAt"`
}

// Membership links a User to a clinic with a specific role in platform.memberships.
type Membership struct {
	ID        bson.ObjectID    `bson:"_id,omitempty"`
	UserID    bson.ObjectID    `bson:"userId"`
	ClinicID  bson.ObjectID    `bson:"clinicId"`
	RoleID    bson.ObjectID    `bson:"roleId"`
	Status    MembershipStatus `bson:"status"`
	CreatedAt time.Time        `bson:"createdAt"`
	UpdatedAt time.Time        `bson:"updatedAt"`
}

// Invitation stores a hashed one-time activation token in platform.invitations.
type Invitation struct {
	ID        bson.ObjectID `bson:"_id,omitempty"`
	ClinicID  bson.ObjectID `bson:"clinicId"`
	Email     string        `bson:"email"`
	Name      string        `bson:"name"`
	RoleID    bson.ObjectID `bson:"roleId"`
	TokenHash string        `bson:"tokenHash"`
	InvitedBy bson.ObjectID `bson:"invitedBy"`
	ExpiresAt time.Time     `bson:"expiresAt"`
	UsedAt    *time.Time    `bson:"usedAt,omitempty"`
	CreatedAt time.Time     `bson:"createdAt"`
}

// NormalizeEmail trims and lowercases an email address and validates its syntax.
func NormalizeEmail(raw string) (string, error) {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	if trimmed == "" {
		return "", ErrInvalidEmail
	}
	addr, err := mail.ParseAddress(trimmed)
	if err != nil || addr.Address != trimmed {
		return "", ErrInvalidEmail
	}
	return trimmed, nil
}
