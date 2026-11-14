package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/pquerna/otp/totp"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb/mongotest"
)

func TestPasswordAndTOTP(t *testing.T) {
	if _, err := HashPassword("short"); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("short password: got %v, want ErrWeakPassword", err)
	}

	hash, err := HashPassword("clinica-segura-2026")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	ok, err := VerifyPassword("clinica-segura-2026", hash)
	if err != nil || !ok {
		t.Fatalf("VerifyPassword valid: ok=%v, err=%v", ok, err)
	}
	ok, err = VerifyPassword("otra-clave-distinta", hash)
	if err != nil || ok {
		t.Fatalf("VerifyPassword wrong: ok=%v, err=%v", ok, err)
	}

	setup, err := GenerateTOTP("Agenda Clinicas", "dra.ana@demo.example")
	if err != nil || setup.Secret == "" || setup.URI == "" {
		t.Fatalf("GenerateTOTP: setup=%+v, err=%v", setup, err)
	}
	now := time.Now().UTC()
	code, err := totp.GenerateCode(setup.Secret, now)
	if err != nil {
		t.Fatalf("totp.GenerateCode: %v", err)
	}
	if !VerifyTOTP(setup.Secret, code, now) {
		t.Fatal("VerifyTOTP failed for valid code")
	}
	if VerifyTOTP(setup.Secret, "000000", now) {
		t.Fatal("VerifyTOTP accepted invalid code")
	}

	plain, hashes, err := GenerateRecoveryCodes(10)
	if err != nil || len(plain) != 10 || len(hashes) != 10 {
		t.Fatalf("GenerateRecoveryCodes: len=%d/%d err=%v", len(plain), len(hashes), err)
	}
	rem, ok := ConsumeRecoveryCode(hashes, plain[0])
	if !ok || len(rem) != 9 {
		t.Fatalf("ConsumeRecoveryCode first use: ok=%v len=%d", ok, len(rem))
	}
	if _, ok := ConsumeRecoveryCode(rem, plain[0]); ok {
		t.Fatal("ConsumeRecoveryCode allowed reusing a consumed recovery code")
	}
}

func TestCredentialsOTPAndWebAuthnIntegration(t *testing.T) {
	ctx := context.Background()
	cl, err := mongodb.Connect(ctx, mongotest.URI(t))
	if err != nil {
		t.Fatalf("mongodb.Connect: %v", err)
	}
	t.Cleanup(func() { _ = cl.Close(context.Background()) })

	platformDB := cl.Mongo().Database("platform")

	t.Run("staff password + TOTP + single-use recovery code", func(t *testing.T) {
		creds, err := NewCredentialRepository(ctx, platformDB)
		if err != nil {
			t.Fatalf("NewCredentialRepository: %v", err)
		}

		userID := bson.NewObjectID()
		setup, err := GenerateTOTP("Agenda Clinicas", "staff@demo.example")
		if err != nil {
			t.Fatalf("GenerateTOTP: %v", err)
		}
		plainCodes, hashes, err := GenerateRecoveryCodes(10)
		if err != nil {
			t.Fatalf("GenerateRecoveryCodes: %v", err)
		}

		if err := creds.SetPasswordAndTOTP(ctx, userID, "ClaveSegura2026!", setup.Secret, hashes); err != nil {
			t.Fatalf("SetPasswordAndTOTP: %v", err)
		}

		validTOTP, err := totp.GenerateCode(setup.Secret, time.Now().UTC())
		if err != nil {
			t.Fatalf("GenerateCode: %v", err)
		}
		if err := creds.VerifyPasswordAndSecondFactor(ctx, userID, "ClaveSegura2026!", validTOTP); err != nil {
			t.Fatalf("VerifyPasswordAndSecondFactor with TOTP: %v", err)
		}

		// Wrong password fails even with valid TOTP.
		if err := creds.VerifyPasswordAndSecondFactor(ctx, userID, "ClaveIncorrecta!", validTOTP); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("wrong password: got %v, want ErrInvalidCredentials", err)
		}

		// Recovery code works once and fails on second use.
		if err := creds.VerifyPasswordAndSecondFactor(ctx, userID, "ClaveSegura2026!", plainCodes[0]); err != nil {
			t.Fatalf("VerifyPasswordAndSecondFactor with recovery code: %v", err)
		}
		if err := creds.VerifyPasswordAndSecondFactor(ctx, userID, "ClaveSegura2026!", plainCodes[0]); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("reused recovery code: got %v, want ErrInvalidCredentials", err)
		}
	})

	t.Run("patient 6-digit OTP enforces expiry, 5 attempts, and 3 sends per hour", func(t *testing.T) {
		otps, err := NewOTPRepository(ctx, platformDB)
		if err != nil {
			t.Fatalf("NewOTPRepository: %v", err)
		}

		now := time.Date(2026, 11, 14, 10, 0, 0, 0, time.UTC)
		otps.now = func() time.Time { return now }

		clinicID := bson.NewObjectID()
		contact := "+50760000042"

		code, err := otps.Issue(ctx, clinicID, contact)
		if err != nil || len(code) != 6 {
			t.Fatalf("Issue OTP: code=%q err=%v", code, err)
		}

		// 5 failed attempts lock out the code.
		for i := range DefaultOTPMaxAttempts {
			err := otps.Verify(ctx, clinicID, contact, "999999")
			want := ErrOTPInvalid
			if i == DefaultOTPMaxAttempts-1 {
				want = ErrOTPMaxAttempts
			}
			if !errors.Is(err, want) {
				t.Fatalf("attempt %d: got %v, want %v", i+1, err, want)
			}
		}

		// Issue 2nd and 3rd code within the hour; 4th must be rate limited.
		code2, err := otps.Issue(ctx, clinicID, contact)
		if err != nil {
			t.Fatalf("Issue 2: %v", err)
		}
		if err := otps.Verify(ctx, clinicID, contact, code2); err != nil {
			t.Fatalf("Verify code2: %v", err)
		}
		if err := otps.Verify(ctx, clinicID, contact, code2); !errors.Is(err, ErrOTPInvalid) {
			t.Fatalf("reused OTP: got %v, want ErrOTPInvalid", err)
		}

		code3, err := otps.Issue(ctx, clinicID, contact)
		if err != nil {
			t.Fatalf("Issue 3: %v", err)
		}
		if _, err := otps.Issue(ctx, clinicID, contact); !errors.Is(err, ErrOTPRateLimited) {
			t.Fatalf("4th OTP in 1h: got %v, want ErrOTPRateLimited", err)
		}

		// Advance clock past 10m -> code3 expires.
		now = now.Add(11 * time.Minute)
		if err := otps.Verify(ctx, clinicID, contact, code3); !errors.Is(err, ErrOTPExpired) {
			t.Fatalf("expired OTP: got %v, want ErrOTPExpired", err)
		}
	})

	t.Run("WebAuthnService begins registration and login when passkeys exist", func(t *testing.T) {
		creds, err := NewCredentialRepository(ctx, platformDB)
		if err != nil {
			t.Fatalf("NewCredentialRepository: %v", err)
		}
		waSvc, err := NewWebAuthnService("Agenda Clinicas", "localhost", []string{"http://localhost:3000"}, creds)
		if err != nil {
			t.Fatalf("NewWebAuthnService: %v", err)
		}

		userID := bson.NewObjectID()
		creation, session, err := waSvc.BeginRegistration(ctx, userID, "ana@demo.example", "Dra. Ana")
		if err != nil || creation == nil || session == nil || session.Challenge == "" {
			t.Fatalf("BeginRegistration: creation=%v session=%v err=%v", creation, session, err)
		}

		// Without credentials, BeginLogin returns ErrInvalidCredentials.
		if _, _, err := waSvc.BeginLogin(ctx, userID, "ana@demo.example", "Dra. Ana"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("BeginLogin without passkeys: got %v, want ErrInvalidCredentials", err)
		}

		// Once a passkey is stored, BeginLogin produces an assertion challenge.
		if err := creds.AddPasskey(ctx, userID, webauthn.Credential{ID: []byte("cred-1")}); err != nil {
			t.Fatalf("AddPasskey: %v", err)
		}
		assertion, loginSess, err := waSvc.BeginLogin(ctx, userID, "ana@demo.example", "Dra. Ana")
		if err != nil || assertion == nil || loginSess == nil || len(assertion.Response.AllowedCredentials) != 1 {
			t.Fatalf("BeginLogin with passkey: assertion=%v session=%v err=%v", assertion, loginSess, err)
		}
	})
}
