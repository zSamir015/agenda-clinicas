package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const staffCredentialsCollection = "staff_credentials" //nolint:gosec // collection name, not a credential

// StaffCredentials stores password, TOTP, recovery codes, and passkeys for a user.
type StaffCredentials struct {
	ID                  bson.ObjectID         `bson:"_id,omitempty"`
	UserID              bson.ObjectID         `bson:"userId"`
	PasswordHash        string                `bson:"passwordHash,omitempty"`
	TOTPSecret          string                `bson:"totpSecret,omitempty"`
	TOTPEnabled         bool                  `bson:"totpEnabled"`
	RecoveryCodeHashes  []string              `bson:"recoveryCodeHashes,omitempty"`
	WebAuthnCredentials []webauthn.Credential `bson:"webauthnCredentials,omitempty"`
	UpdatedAt           time.Time             `bson:"updatedAt"`
}

// CredentialRepository manages staff authentication credentials in platform.staff_credentials.
type CredentialRepository struct {
	col *mongo.Collection
	now func() time.Time
}

// NewCredentialRepository initializes platform.staff_credentials indexes.
func NewCredentialRepository(ctx context.Context, platformDB *mongo.Database) (*CredentialRepository, error) {
	col := platformDB.Collection(staffCredentialsCollection)
	if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "userId", Value: 1}},
		Options: options.Index().SetUnique(true).SetName("user_id_unique"),
	}); err != nil {
		return nil, fmt.Errorf("create staff_credentials index: %w", err)
	}
	return &CredentialRepository{
		col: col,
		now: func() time.Time { return time.Now().UTC().Truncate(time.Millisecond) },
	}, nil
}

// ByUserID loads credentials for userID or returns ErrInvalidCredentials if absent.
func (r *CredentialRepository) ByUserID(ctx context.Context, userID bson.ObjectID) (StaffCredentials, error) {
	var creds StaffCredentials
	err := r.col.FindOne(ctx, bson.D{{Key: "userId", Value: userID}}).Decode(&creds)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return StaffCredentials{}, ErrInvalidCredentials
	}
	if err != nil {
		return StaffCredentials{}, fmt.Errorf("find staff credentials: %w", err)
	}
	return creds, nil
}

// SetPasswordAndTOTP configures the Argon2id password, TOTP secret, and
// 10 single-use recovery codes for userID.
func (r *CredentialRepository) SetPasswordAndTOTP(ctx context.Context, userID bson.ObjectID, password, totpSecret string, recoveryHashes []string) error {
	if userID.IsZero() || totpSecret == "" || len(recoveryHashes) == 0 {
		return ErrInvalidCredentials
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	now := r.now()
	_, err = r.col.UpdateOne(
		ctx,
		bson.D{{Key: "userId", Value: userID}},
		bson.D{
			{Key: "$set", Value: bson.D{
				{Key: "passwordHash", Value: hash},
				{Key: "totpSecret", Value: totpSecret},
				{Key: "totpEnabled", Value: true},
				{Key: "recoveryCodeHashes", Value: recoveryHashes},
				{Key: "updatedAt", Value: now},
			}},
			{Key: "$setOnInsert", Value: bson.D{{Key: "userId", Value: userID}}},
		},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("save password and totp: %w", err)
	}
	return nil
}

// VerifyPasswordAndSecondFactor checks the Argon2id password and either a valid
// TOTP code or a single-use recovery code (consuming the recovery code if used).
func (r *CredentialRepository) VerifyPasswordAndSecondFactor(ctx context.Context, userID bson.ObjectID, password, code string) error {
	creds, err := r.ByUserID(ctx, userID)
	if err != nil {
		return err
	}
	if creds.PasswordHash == "" || !creds.TOTPEnabled {
		return ErrInvalidCredentials
	}
	ok, err := VerifyPassword(password, creds.PasswordHash)
	if err != nil || !ok {
		return ErrInvalidCredentials
	}

	if VerifyTOTP(creds.TOTPSecret, code, r.now()) {
		return nil
	}

	if _, usedRecovery := ConsumeRecoveryCode(creds.RecoveryCodeHashes, code); !usedRecovery {
		return ErrInvalidCredentials
	}

	// Atomic single-use consumption: the filter requires the hash to still be
	// present, so concurrent requests cannot both redeem the same code.
	res, err := r.col.UpdateOne(
		ctx,
		bson.D{
			{Key: "userId", Value: userID},
			{Key: "recoveryCodeHashes", Value: hashRecoveryCode(code)},
		},
		bson.D{
			{Key: "$pull", Value: bson.D{{Key: "recoveryCodeHashes", Value: hashRecoveryCode(code)}}},
			{Key: "$set", Value: bson.D{{Key: "updatedAt", Value: r.now()}}},
		},
	)
	if err != nil {
		return fmt.Errorf("consume recovery code: %w", err)
	}
	if res.MatchedCount == 0 {
		return ErrInvalidCredentials
	}
	return nil
}

// AddPasskey appends a WebAuthn credential to userID.
func (r *CredentialRepository) AddPasskey(ctx context.Context, userID bson.ObjectID, cred webauthn.Credential) error {
	if userID.IsZero() {
		return ErrInvalidCredentials
	}
	_, err := r.col.UpdateOne(
		ctx,
		bson.D{{Key: "userId", Value: userID}},
		bson.D{
			{Key: "$push", Value: bson.D{{Key: "webauthnCredentials", Value: cred}}},
			{Key: "$set", Value: bson.D{{Key: "updatedAt", Value: r.now()}}},
			{Key: "$setOnInsert", Value: bson.D{{Key: "userId", Value: userID}}},
		},
		options.UpdateOne().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("add passkey: %w", err)
	}
	return nil
}

// UpdatePasskeySignCount updates the stored credential after a successful assertion.
func (r *CredentialRepository) UpdatePasskeySignCount(ctx context.Context, userID bson.ObjectID, cred webauthn.Credential) error {
	res, err := r.col.UpdateOne(
		ctx,
		bson.D{
			{Key: "userId", Value: userID},
			{Key: "webauthnCredentials.id", Value: cred.ID},
		},
		bson.D{{Key: "$set", Value: bson.D{
			// webauthn.Credential has no bson tags, so the driver stores the
			// field names lowercased ("signcount", not "signCount").
			{Key: "webauthnCredentials.$.authenticator.signcount", Value: cred.Authenticator.SignCount},
			{Key: "updatedAt", Value: r.now()},
		}}},
	)
	if err != nil {
		return fmt.Errorf("update passkey sign count: %w", err)
	}
	if res.MatchedCount == 0 {
		return ErrInvalidCredentials
	}
	return nil
}
