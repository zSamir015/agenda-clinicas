package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	patientOTPCollection = "patient_otps"

	// DefaultOTPTTL is the validity window of a patient 6-digit code.
	DefaultOTPTTL = 10 * time.Minute
	// DefaultOTPMaxAttempts is the maximum verification attempts per code.
	DefaultOTPMaxAttempts = 5
	// DefaultOTPMaxHourlySends is the maximum codes sent per hour to one contact.
	DefaultOTPMaxHourlySends = 3
)

var (
	// ErrOTPInvalid is returned when a code is missing or does not match.
	ErrOTPInvalid = errors.New("auth: invalid otp code")
	// ErrOTPExpired is returned when the code has passed its expiration time.
	ErrOTPExpired = errors.New("auth: otp code expired")
	// ErrOTPMaxAttempts is returned when too many failed verification attempts occurred.
	ErrOTPMaxAttempts = errors.New("auth: otp max attempts exceeded")
	// ErrOTPRateLimited is returned when hourly send quota for a contact is exceeded.
	ErrOTPRateLimited = errors.New("auth: otp hourly send limit exceeded")
)

type patientOTPRecord struct {
	ID        bson.ObjectID `bson:"_id,omitempty"`
	ClinicID  bson.ObjectID `bson:"clinicId"`
	Contact   string        `bson:"contact"`
	CodeHash  string        `bson:"codeHash"`
	Attempts  int           `bson:"attempts"`
	ExpiresAt time.Time     `bson:"expiresAt"`
	UsedAt    *time.Time    `bson:"usedAt,omitempty"`
	CreatedAt time.Time     `bson:"createdAt"`
}

// OTPRepository stores and verifies 6-digit one-time codes for patients.
type OTPRepository struct {
	col *mongo.Collection
	now func() time.Time
}

// NewOTPRepository initializes platform.patient_otps and its lookup index.
func NewOTPRepository(ctx context.Context, platformDB *mongo.Database) (*OTPRepository, error) {
	col := platformDB.Collection(patientOTPCollection)
	if _, err := col.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "clinicId", Value: 1}, {Key: "contact", Value: 1}, {Key: "createdAt", Value: -1}},
		Options: options.Index().SetName("clinic_contact_created_idx"),
	}); err != nil {
		return nil, fmt.Errorf("create otp index: %w", err)
	}
	return &OTPRepository{
		col: col,
		now: func() time.Time { return time.Now().UTC().Truncate(time.Millisecond) },
	}, nil
}

// Issue creates a 6-digit code for (clinicID, contact) if under the hourly send limit.
func (r *OTPRepository) Issue(ctx context.Context, clinicID bson.ObjectID, contact string) (string, error) {
	contact = normalizeContact(contact)
	if clinicID.IsZero() || contact == "" {
		return "", ErrOTPInvalid
	}

	now := r.now()
	since := now.Add(-time.Hour)
	sentCount, err := r.col.CountDocuments(ctx, bson.D{
		{Key: "clinicId", Value: clinicID},
		{Key: "contact", Value: contact},
		{Key: "createdAt", Value: bson.D{{Key: "$gte", Value: since}}},
	})
	if err != nil {
		return "", fmt.Errorf("count recent otp sends: %w", err)
	}
	if sentCount >= DefaultOTPMaxHourlySends {
		return "", ErrOTPRateLimited
	}

	code, err := randomSixDigits()
	if err != nil {
		return "", fmt.Errorf("generate otp digits: %w", err)
	}

	if _, err := r.col.UpdateMany(
		ctx,
		bson.D{
			{Key: "clinicId", Value: clinicID},
			{Key: "contact", Value: contact},
			{Key: "usedAt", Value: bson.D{{Key: "$exists", Value: false}}},
		},
		bson.D{{Key: "$set", Value: bson.D{{Key: "usedAt", Value: now}}}},
	); err != nil {
		return "", fmt.Errorf("supersede previous otp: %w", err)
	}

	rec := patientOTPRecord{
		ClinicID:  clinicID,
		Contact:   contact,
		CodeHash:  hashOTP(clinicID, contact, code),
		Attempts:  0,
		ExpiresAt: now.Add(DefaultOTPTTL),
		CreatedAt: now,
	}
	if _, err := r.col.InsertOne(ctx, rec); err != nil {
		return "", fmt.Errorf("insert otp record: %w", err)
	}
	return code, nil
}

// Verify checks the latest active 6-digit code for (clinicID, contact),
// enforcing expiration, max 5 attempts, and single-use consumption.
func (r *OTPRepository) Verify(ctx context.Context, clinicID bson.ObjectID, contact, code string) error {
	contact = normalizeContact(contact)
	code = strings.TrimSpace(code)
	if clinicID.IsZero() || contact == "" || len(code) != 6 {
		return ErrOTPInvalid
	}

	var rec patientOTPRecord
	err := r.col.FindOne(
		ctx,
		bson.D{
			{Key: "clinicId", Value: clinicID},
			{Key: "contact", Value: contact},
			{Key: "usedAt", Value: bson.D{{Key: "$exists", Value: false}}},
		},
		options.FindOne().SetSort(bson.D{{Key: "createdAt", Value: -1}, {Key: "_id", Value: -1}}),
	).Decode(&rec)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrOTPInvalid
	}
	if err != nil {
		return fmt.Errorf("find otp record: %w", err)
	}

	now := r.now()
	if !rec.ExpiresAt.After(now) {
		return ErrOTPExpired
	}
	if rec.Attempts >= DefaultOTPMaxAttempts {
		return ErrOTPMaxAttempts
	}

	if hashOTP(clinicID, contact, code) != rec.CodeHash {
		var after patientOTPRecord
		err := r.col.FindOneAndUpdate(
			ctx,
			bson.D{{Key: "_id", Value: rec.ID}},
			bson.D{{Key: "$inc", Value: bson.D{{Key: "attempts", Value: 1}}}},
			options.FindOneAndUpdate().SetReturnDocument(options.After),
		).Decode(&after)
		if err != nil {
			return fmt.Errorf("record otp attempt: %w", err)
		}
		if after.Attempts >= DefaultOTPMaxAttempts {
			return ErrOTPMaxAttempts
		}
		return ErrOTPInvalid
	}

	res, err := r.col.UpdateOne(
		ctx,
		bson.D{
			{Key: "_id", Value: rec.ID},
			{Key: "usedAt", Value: bson.D{{Key: "$exists", Value: false}}},
			{Key: "attempts", Value: bson.D{{Key: "$lt", Value: DefaultOTPMaxAttempts}}},
		},
		bson.D{{Key: "$set", Value: bson.D{{Key: "usedAt", Value: now}}}},
	)
	if err != nil {
		return fmt.Errorf("consume otp record: %w", err)
	}
	if res.MatchedCount == 0 {
		return ErrOTPInvalid
	}
	return nil
}

func normalizeContact(c string) string {
	return strings.TrimSpace(strings.ToLower(c))
}

func randomSixDigits() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func hashOTP(clinicID bson.ObjectID, contact, code string) string {
	sum := sha256.Sum256([]byte(clinicID.Hex() + ":" + contact + ":" + code))
	return hex.EncodeToString(sum[:])
}
