package auth

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb/mongotest"
)

func TestCredentialsRegressions(t *testing.T) {
	ctx := context.Background()
	cl, err := mongodb.Connect(ctx, mongotest.URI(t))
	if err != nil {
		t.Fatalf("mongodb.Connect: %v", err)
	}
	t.Cleanup(func() { _ = cl.Close(context.Background()) })
	platformDB := cl.Mongo().Database("platform_regression")

	creds, err := NewCredentialRepository(ctx, platformDB)
	if err != nil {
		t.Fatalf("NewCredentialRepository: %v", err)
	}

	t.Run("recovery code is redeemable by only one concurrent login", func(t *testing.T) {
		userID := bson.NewObjectID()
		setup, err := GenerateTOTP("Agenda Clinicas", "race@demo.example")
		if err != nil {
			t.Fatalf("GenerateTOTP: %v", err)
		}
		plain, hashes, err := GenerateRecoveryCodes(10)
		if err != nil {
			t.Fatalf("GenerateRecoveryCodes: %v", err)
		}
		if err := creds.SetPasswordAndTOTP(ctx, userID, "ClaveSegura2026!", setup.Secret, hashes); err != nil {
			t.Fatalf("SetPasswordAndTOTP: %v", err)
		}

		const workers = 8
		var wins atomic.Int32
		var wg sync.WaitGroup
		start := make(chan struct{})
		for range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				if err := creds.VerifyPasswordAndSecondFactor(ctx, userID, "ClaveSegura2026!", plain[0]); err == nil {
					wins.Add(1)
				}
			}()
		}
		close(start)
		wg.Wait()

		if got := wins.Load(); got != 1 {
			t.Fatalf("recovery code redeemed %d times, want exactly 1", got)
		}
		stored, err := creds.ByUserID(ctx, userID)
		if err != nil {
			t.Fatalf("ByUserID: %v", err)
		}
		if len(stored.RecoveryCodeHashes) != 9 {
			t.Fatalf("remaining recovery codes = %d, want 9", len(stored.RecoveryCodeHashes))
		}
		// An unrelated code must survive a concurrent redemption of another.
		if _, ok := ConsumeRecoveryCode(stored.RecoveryCodeHashes, plain[1]); !ok {
			t.Fatal("unrelated recovery code was lost")
		}
	})

	t.Run("passkey sign count is persisted under the stored key", func(t *testing.T) {
		userID := bson.NewObjectID()
		cred := webauthn.Credential{ID: []byte("cred-sign")}
		if err := creds.AddPasskey(ctx, userID, cred); err != nil {
			t.Fatalf("AddPasskey: %v", err)
		}

		cred.Authenticator.SignCount = 7
		if err := creds.UpdatePasskeySignCount(ctx, userID, cred); err != nil {
			t.Fatalf("UpdatePasskeySignCount: %v", err)
		}
		stored, err := creds.ByUserID(ctx, userID)
		if err != nil {
			t.Fatalf("ByUserID: %v", err)
		}
		if got := stored.WebAuthnCredentials[0].Authenticator.SignCount; got != 7 {
			t.Fatalf("stored sign count = %d, want 7", got)
		}

		var raw struct {
			Creds []struct {
				Authenticator bson.M `bson:"authenticator"`
			} `bson:"webauthnCredentials"`
		}
		if err := creds.col.FindOne(ctx, bson.D{{Key: "userId", Value: userID}}).Decode(&raw); err != nil {
			t.Fatalf("raw find: %v", err)
		}
		if _, dup := raw.Creds[0].Authenticator["signCount"]; dup {
			t.Fatal("sign count written under a second key \"signCount\"")
		}

		missing := webauthn.Credential{ID: []byte("does-not-exist")}
		if err := creds.UpdatePasskeySignCount(ctx, userID, missing); err == nil {
			t.Fatal("UpdatePasskeySignCount for unknown passkey returned nil")
		}
	})

	t.Run("database errors are not reported as missing passkeys", func(t *testing.T) {
		waSvc, err := NewWebAuthnService("Agenda Clinicas", "localhost", []string{"http://localhost:3000"}, creds)
		if err != nil {
			t.Fatalf("NewWebAuthnService: %v", err)
		}
		cancelled, cancel := context.WithCancel(ctx)
		cancel()

		userID := bson.NewObjectID()
		if _, _, err := waSvc.BeginLogin(cancelled, userID, "x@demo.example", "X"); err == nil || errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("BeginLogin on DB failure: got %v, want a non-ErrInvalidCredentials error", err)
		}
		if _, _, err := waSvc.BeginRegistration(cancelled, userID, "x@demo.example", "X"); err == nil {
			t.Fatal("BeginRegistration proceeded without existing passkeys after a DB failure")
		}
	})

	t.Run("concurrent wrong OTP guesses are all counted", func(t *testing.T) {
		otps, err := NewOTPRepository(ctx, platformDB)
		if err != nil {
			t.Fatalf("NewOTPRepository: %v", err)
		}
		now := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
		otps.now = func() time.Time { return now }

		clinicID := bson.NewObjectID()
		contact := "+50760000099"
		if _, err := otps.Issue(ctx, clinicID, contact); err != nil {
			t.Fatalf("Issue: %v", err)
		}

		var maxed atomic.Int32
		var wg sync.WaitGroup
		start := make(chan struct{})
		for range DefaultOTPMaxAttempts {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				if err := otps.Verify(ctx, clinicID, contact, "999999"); errors.Is(err, ErrOTPMaxAttempts) {
					maxed.Add(1)
				}
			}()
		}
		close(start)
		wg.Wait()

		// Exactly one request observes the post-increment count reaching the limit.
		if got := maxed.Load(); got != 1 {
			t.Fatalf("ErrOTPMaxAttempts returned %d times, want exactly 1", got)
		}
	})
}
