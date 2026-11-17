package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// WebAuthnUser adapts a staff user and their stored credentials to webauthn.User.
type WebAuthnUser struct {
	ID          bson.ObjectID
	Email       string
	DisplayName string
	Credentials []webauthn.Credential
}

// WebAuthnID implements webauthn.User.
func (u WebAuthnUser) WebAuthnID() []byte { return u.ID[:] }

// WebAuthnName implements webauthn.User.
func (u WebAuthnUser) WebAuthnName() string { return u.Email }

// WebAuthnDisplayName implements webauthn.User.
func (u WebAuthnUser) WebAuthnDisplayName() string { return u.DisplayName }

// WebAuthnCredentials implements webauthn.User.
func (u WebAuthnUser) WebAuthnCredentials() []webauthn.Credential { return u.Credentials }

// WebAuthnService wraps go-webauthn/webauthn with CredentialRepository persistence.
type WebAuthnService struct {
	wa    *webauthn.WebAuthn
	creds *CredentialRepository
}

// NewWebAuthnService initializes a WebAuthn relying party for rpID and rpOrigins.
func NewWebAuthnService(rpDisplayName, rpID string, rpOrigins []string, creds *CredentialRepository) (*WebAuthnService, error) {
	wa, err := webauthn.New(&webauthn.Config{
		RPDisplayName: rpDisplayName,
		RPID:          rpID,
		RPOrigins:     rpOrigins,
	})
	if err != nil {
		return nil, fmt.Errorf("init webauthn: %w", err)
	}
	return &WebAuthnService{wa: wa, creds: creds}, nil
}

// BeginRegistration starts passkey registration for userID and returns the
// challenge options plus session data to store for FinishRegistration.
func (s *WebAuthnService) BeginRegistration(ctx context.Context, userID bson.ObjectID, email, displayName string) (*protocol.CredentialCreation, *webauthn.SessionData, error) {
	u, err := s.loadUser(ctx, userID, email, displayName)
	if err != nil {
		return nil, nil, err
	}
	creation, sessionData, err := s.wa.BeginRegistration(u)
	if err != nil {
		return nil, nil, fmt.Errorf("begin webauthn registration: %w", err)
	}
	return creation, sessionData, nil
}

// FinishRegistration verifies the client attestation and persists the new passkey.
func (s *WebAuthnService) FinishRegistration(ctx context.Context, userID bson.ObjectID, email, displayName string, session webauthn.SessionData, r *http.Request) (*webauthn.Credential, error) {
	u, err := s.loadUser(ctx, userID, email, displayName)
	if err != nil {
		return nil, err
	}
	cred, err := s.wa.FinishRegistration(u, session, r)
	if err != nil {
		return nil, fmt.Errorf("finish webauthn registration: %w", err)
	}
	if err := s.creds.AddPasskey(ctx, userID, *cred); err != nil {
		return nil, err
	}
	return cred, nil
}

// BeginLogin starts passkey authentication for userID.
func (s *WebAuthnService) BeginLogin(ctx context.Context, userID bson.ObjectID, email, displayName string) (*protocol.CredentialAssertion, *webauthn.SessionData, error) {
	u, err := s.loadUser(ctx, userID, email, displayName)
	if err != nil {
		return nil, nil, err
	}
	if len(u.Credentials) == 0 {
		return nil, nil, ErrInvalidCredentials
	}
	assertion, sessionData, err := s.wa.BeginLogin(u)
	if err != nil {
		return nil, nil, fmt.Errorf("begin webauthn login: %w", err)
	}
	return assertion, sessionData, nil
}

// FinishLogin verifies the client assertion and updates the credential sign count.
func (s *WebAuthnService) FinishLogin(ctx context.Context, userID bson.ObjectID, email, displayName string, session webauthn.SessionData, r *http.Request) (*webauthn.Credential, error) {
	u, err := s.loadUser(ctx, userID, email, displayName)
	if err != nil {
		return nil, err
	}
	cred, err := s.wa.FinishLogin(u, session, r)
	if err != nil {
		return nil, fmt.Errorf("finish webauthn login: %w", err)
	}
	if err := s.creds.UpdatePasskeySignCount(ctx, userID, *cred); err != nil {
		return nil, err
	}
	return cred, nil
}

func (s *WebAuthnService) loadUser(ctx context.Context, userID bson.ObjectID, email, displayName string) (WebAuthnUser, error) {
	var list []webauthn.Credential
	stored, err := s.creds.ByUserID(ctx, userID)
	switch {
	case err == nil:
		list = stored.WebAuthnCredentials
	case errors.Is(err, ErrInvalidCredentials):
		// No credentials document yet: user has no passkeys.
	default:
		return WebAuthnUser{}, fmt.Errorf("load passkeys: %w", err)
	}
	return WebAuthnUser{
		ID:          userID,
		Email:       email,
		DisplayName: displayName,
		Credentials: list,
	}, nil
}
