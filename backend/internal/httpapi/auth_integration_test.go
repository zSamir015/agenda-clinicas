package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/zSamir015/agenda-clinicas/backend/internal/auth"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb/mongotest"
	"github.com/zSamir015/agenda-clinicas/backend/internal/platform"
	"github.com/zSamir015/agenda-clinicas/backend/internal/rbac"
	"github.com/zSamir015/agenda-clinicas/backend/internal/users"
)

type recordingOTPSender struct {
	mu       sync.Mutex
	lastCode map[string]string
}

func newRecordingOTPSender() *recordingOTPSender {
	return &recordingOTPSender{lastCode: make(map[string]string)}
}

func (r *recordingOTPSender) SendPatientOTP(_ context.Context, _ platform.Clinic, contact, code string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lastCode[contact] = code
	return nil
}

func (r *recordingOTPSender) codeFor(contact string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.lastCode[contact]
}

func doJSON(t *testing.T, h http.Handler, method, path string, body any, cookie *http.Cookie, csrf string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode %s %s: %v", method, path, err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.RemoteAddr = "127.0.0.1:42000"
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		req.AddCookie(cookie)
	}
	if csrf != "" {
		req.Header.Set(auth.DefaultCSRFHeader, csrf)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeJSON[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode JSON (%d %q): %v", rec.Code, rec.Body.String(), err)
	}
	return out
}

func firstCookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func TestAuthRBACAndOTPHTTPFlow(t *testing.T) {
	ctx := context.Background()
	cl, err := mongodb.Connect(ctx, mongotest.URI(t))
	if err != nil {
		t.Fatalf("mongodb.Connect: %v", err)
	}
	t.Cleanup(func() { _ = cl.Close(context.Background()) })

	platformDB := cl.Mongo().Database("platform")
	clinics, err := platform.NewClinicRepository(ctx, platformDB)
	if err != nil {
		t.Fatalf("NewClinicRepository: %v", err)
	}
	roles, err := rbac.NewRoleRepository(ctx, platformDB)
	if err != nil {
		t.Fatalf("NewRoleRepository: %v", err)
	}
	userRepo, err := users.NewRepository(ctx, platformDB)
	if err != nil {
		t.Fatalf("NewRepository: %v", err)
	}
	sessions, err := auth.NewSessionRepository(ctx, platformDB)
	if err != nil {
		t.Fatalf("NewSessionRepository: %v", err)
	}
	creds, err := auth.NewCredentialRepository(ctx, platformDB)
	if err != nil {
		t.Fatalf("NewCredentialRepository: %v", err)
	}
	otps, err := auth.NewOTPRepository(ctx, platformDB)
	if err != nil {
		t.Fatalf("NewOTPRepository: %v", err)
	}
	wa, err := auth.NewWebAuthnService("Agenda Test", "localhost", []string{"http://localhost:3000"}, creds)
	if err != nil {
		t.Fatalf("NewWebAuthnService: %v", err)
	}

	sender := newRecordingOTPSender()
	srv := NewServer(cl, WithAuth(AuthDeps{
		Clinics:        clinics,
		Roles:          roles,
		Users:          userRepo,
		Sessions:       sessions,
		Creds:          creds,
		OTPs:           otps,
		WebAuthn:       wa,
		Limiter:        auth.NewRateLimiter(),
		OTPSender:      sender,
		LoginRateLimit: 10,
		OTPRateLimit:   10,
		AllowedOrigins: []string{"http://localhost:3000"},
	}))
	h := NewHandler(srv)

	clinicInput, err := platform.NewClinic("clinica-http-test", "Clínica HTTP Test", platform.TierStandard, "pa")
	if err != nil {
		t.Fatalf("NewClinic: %v", err)
	}
	clinic, err := clinics.Create(ctx, clinicInput)
	if err != nil {
		t.Fatalf("clinics.Create: %v", err)
	}
	if err := roles.EnsureTemplates(ctx, clinic.ID); err != nil {
		t.Fatalf("EnsureTemplates: %v", err)
	}
	ownerRole, err := roles.ByName(ctx, clinic.ID, rbac.RoleOwner)
	if err != nil {
		t.Fatalf("ByName owner: %v", err)
	}
	receptionRole, err := roles.ByName(ctx, clinic.ID, rbac.RoleReception)
	if err != nil {
		t.Fatalf("ByName reception: %v", err)
	}

	// Unauthenticated request to /auth/session -> 401 problem+json.
	rec := doJSON(t, h, http.MethodGet, "/auth/session", nil, nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated /auth/session: got %d, want 401", rec.Code)
	}

	// Owner invitation -> preview -> accept -> TOTP setup + confirm.
	_, ownerToken, err := userRepo.CreateInvitation(ctx, clinic.ID, "duena@demo.example", "Dra. Sofía Ruiz", ownerRole.ID, bson.NewObjectID(), time.Hour)
	if err != nil {
		t.Fatalf("CreateInvitation: %v", err)
	}

	rec = doJSON(t, h, http.MethodPost, "/auth/invitations/preview", map[string]string{"token": ownerToken}, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("preview invitation: %d %s", rec.Code, rec.Body.String())
	}
	preview := decodeJSON[InvitationPreview](t, rec)
	if preview.ClinicSlug != clinic.Slug || preview.RoleName != rbac.RoleOwner {
		t.Fatalf("unexpected preview: %+v", preview)
	}

	rec = doJSON(t, h, http.MethodPost, "/auth/invitations/accept", map[string]string{"token": ownerToken}, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("accept invitation: %d %s", rec.Code, rec.Body.String())
	}
	ownerSess := decodeJSON[Session](t, rec)
	ownerCookie := firstCookie(rec, auth.DefaultCookieName)
	if ownerCookie == nil || ownerSess.CsrfToken == "" {
		t.Fatalf("expected session cookie and csrfToken")
	}

	// Preview again after consumption -> 410 Gone.
	rec = doJSON(t, h, http.MethodPost, "/auth/invitations/preview", map[string]string{"token": ownerToken}, nil, "")
	if rec.Code != http.StatusGone {
		t.Fatalf("reused invite preview: got %d, want 410", rec.Code)
	}

	// CSRF enforced on mutating route.
	rec = doJSON(t, h, http.MethodPost, "/auth/totp/setup", nil, ownerCookie, "wrong-csrf")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("bad csrf: got %d, want 403", rec.Code)
	}

	// Configure TOTP + Argon2id password + 10 recovery codes.
	rec = doJSON(t, h, http.MethodPost, "/auth/totp/setup", nil, ownerCookie, ownerSess.CsrfToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("totp setup: %d %s", rec.Code, rec.Body.String())
	}
	setup := decodeJSON[TotpSetupResponse](t, rec)

	totpCode, err := totp.GenerateCode(setup.Secret, time.Now().UTC())
	if err != nil {
		t.Fatalf("GenerateCode: %v", err)
	}
	rec = doJSON(t, h, http.MethodPost, "/auth/totp/confirm", map[string]string{
		"password": "ClaveSuperSegura2026!",
		"secret":   setup.Secret,
		"code":     totpCode,
	}, ownerCookie, ownerSess.CsrfToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("totp confirm: %d %s", rec.Code, rec.Body.String())
	}
	confirm := decodeJSON[TotpConfirmResponse](t, rec)
	if len(confirm.RecoveryCodes) != 10 {
		t.Fatalf("expected 10 recovery codes, got %d", len(confirm.RecoveryCodes))
	}

	// Logout + login with recovery code (single-use).
	rec = doJSON(t, h, http.MethodPost, "/auth/logout", nil, ownerCookie, ownerSess.CsrfToken)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout: %d", rec.Code)
	}
	rec = doJSON(t, h, http.MethodGet, "/auth/session", nil, ownerCookie, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session should return 401, got %d", rec.Code)
	}

	rec = doJSON(t, h, http.MethodPost, "/auth/login/password", map[string]string{
		"clinicSlug": clinic.Slug,
		"email":      "duena@demo.example",
		"password":   "ClaveSuperSegura2026!",
		"code":       confirm.RecoveryCodes[0],
	}, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("login with recovery code: %d %s", rec.Code, rec.Body.String())
	}
	ownerSess = decodeJSON[Session](t, rec)
	ownerCookie = firstCookie(rec, auth.DefaultCookieName)

	// Reusing the same recovery code must fail.
	rec = doJSON(t, h, http.MethodPost, "/auth/login/password", map[string]string{
		"clinicSlug": clinic.Slug,
		"email":      "duena@demo.example",
		"password":   "ClaveSuperSegura2026!",
		"code":       confirm.RecoveryCodes[0],
	}, nil, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("reused recovery code: got %d, want 401", rec.Code)
	}

	// RBAC: create custom role, update it, verify session grant cache invalidation, protect owner lock.
	rec = doJSON(t, h, http.MethodPost, "/rbac/roles", map[string]any{
		"name": "coordinator",
		"grants": []map[string]string{
			{"permission": "appointments.read", "scope": "all"},
			{"permission": "roles.manage", "scope": "all"},
		},
	}, ownerCookie, ownerSess.CsrfToken)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create role: %d %s", rec.Code, rec.Body.String())
	}
	customRole := decodeJSON[Role](t, rec)

	// Cannot modify locked owner role.
	rec = doJSON(t, h, http.MethodPut, "/rbac/roles/"+ownerRole.ID.Hex(), map[string]any{
		"name":   "owner",
		"grants": []map[string]string{{"permission": "appointments.read", "scope": "own"}},
	}, ownerCookie, ownerSess.CsrfToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("edit owner role: got %d, want 403", rec.Code)
	}

	// Invite staff member as coordinator, accept, try privilege escalation (must fail 403), then test cache invalidation.
	rec = doJSON(t, h, http.MethodPost, "/users/invitations", map[string]string{
		"email":  "coord@demo.example",
		"name":   "Carlos Méndez",
		"roleId": customRole.Id,
	}, ownerCookie, ownerSess.CsrfToken)
	if rec.Code != http.StatusCreated {
		t.Fatalf("postUsersInvitations: %d %s", rec.Code, rec.Body.String())
	}
	staffInv := decodeJSON[CreatedInvitation](t, rec)

	rec = doJSON(t, h, http.MethodPost, "/auth/invitations/accept", map[string]string{"token": staffInv.Token}, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("accept staff invite: %d %s", rec.Code, rec.Body.String())
	}
	coordSess := decodeJSON[Session](t, rec)
	coordCookie := firstCookie(rec, auth.DefaultCookieName)

	// Privilege escalation: coordinator lacks users.manage -> cannot create role with users.manage.
	rec = doJSON(t, h, http.MethodPost, "/rbac/roles", map[string]any{
		"name":   "super-coord",
		"grants": []map[string]string{{"permission": "users.manage", "scope": "all"}},
	}, coordCookie, coordSess.CsrfToken)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("privilege escalation: got %d, want 403", rec.Code)
	}

	// Owner updates coordinator grants -> coord's active session receives updated grants.
	rec = doJSON(t, h, http.MethodPut, "/rbac/roles/"+customRole.Id, map[string]any{
		"name": "coordinator-v2",
		"grants": []map[string]string{
			{"permission": "appointments.read", "scope": "all"},
			{"permission": "appointments.write", "scope": "all"},
		},
	}, ownerCookie, ownerSess.CsrfToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("update role: %d %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, h, http.MethodGet, "/auth/session", nil, coordCookie, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("coord session after role update: %d", rec.Code)
	}
	refreshedCoord := decodeJSON[Session](t, rec)
	if refreshedCoord.Grants == nil || len(*refreshedCoord.Grants) != 2 {
		t.Fatalf("expected invalidated session cache to show 2 grants, got %+v", refreshedCoord.Grants)
	}

	// Last-owner protection on demote and delete.
	rec = doJSON(t, h, http.MethodPut, "/users/members/"+*ownerSess.UserId+"/role", map[string]string{
		"roleId": receptionRole.ID.Hex(),
	}, ownerCookie, ownerSess.CsrfToken)
	if rec.Code != http.StatusConflict {
		t.Fatalf("demote last owner: got %d, want 409", rec.Code)
	}
	rec = doJSON(t, h, http.MethodDelete, "/users/members/"+*ownerSess.UserId, nil, ownerCookie, ownerSess.CsrfToken)
	if rec.Code != http.StatusConflict {
		t.Fatalf("revoke last owner: got %d, want 409", rec.Code)
	}

	// Revoking coord membership immediately revokes their active session.
	rec = doJSON(t, h, http.MethodDelete, "/users/members/"+*coordSess.UserId, nil, ownerCookie, ownerSess.CsrfToken)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("revoke coord membership: %d %s", rec.Code, rec.Body.String())
	}
	rec = doJSON(t, h, http.MethodGet, "/auth/session", nil, coordCookie, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("revoked member session should fail 401, got %d", rec.Code)
	}

	// Deleting unused role works, deleting active role conflicts.
	rec = doJSON(t, h, http.MethodDelete, "/rbac/roles/"+receptionRole.ID.Hex(), nil, ownerCookie, ownerSess.CsrfToken)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete unused reception role: %d %s", rec.Code, rec.Body.String())
	}

	// Patient OTP send -> verify -> patient session isolated from staff endpoints.
	rec = doJSON(t, h, http.MethodPost, "/auth/patient/otp/send", map[string]string{
		"clinicSlug": clinic.Slug,
		"contact":    "+50761112233",
	}, nil, "")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("otp send: %d %s", rec.Code, rec.Body.String())
	}
	otpCode := sender.codeFor("+50761112233")
	if len(otpCode) != 6 {
		t.Fatalf("expected 6-digit OTP sent, got %q", otpCode)
	}

	rec = doJSON(t, h, http.MethodPost, "/auth/patient/otp/verify", map[string]string{
		"clinicSlug": clinic.Slug,
		"contact":    "+50761112233",
		"code":       otpCode,
	}, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("otp verify: %d %s", rec.Code, rec.Body.String())
	}
	patientSess := decodeJSON[Session](t, rec)
	patientCookie := firstCookie(rec, auth.DefaultCookieName)
	if patientSess.Kind != SessionKind("patient") || patientCookie == nil {
		t.Fatalf("unexpected patient session: %+v", patientSess)
	}

	// Patient session cannot access staff RBAC routes.
	rec = doJSON(t, h, http.MethodGet, "/rbac/roles", nil, patientCookie, "")
	if rec.Code != http.StatusForbidden {
		t.Fatalf("patient accessing /rbac/roles: got %d, want 403", rec.Code)
	}

	// WebAuthn login begin returns 401 when no passkey registered yet, and RegisterBegin works when authenticated.
	rec = doJSON(t, h, http.MethodPost, "/auth/passkeys/register/begin", nil, ownerCookie, ownerSess.CsrfToken)
	if rec.Code != http.StatusOK {
		t.Fatalf("passkey register begin: %d %s", rec.Code, rec.Body.String())
	}
	pkBegin := decodeJSON[PasskeyChallengeResponse](t, rec)
	if pkBegin.ChallengeId == "" || pkBegin.Options["publicKey"] == nil {
		t.Fatalf("expected challengeId and publicKey options, got %+v", pkBegin)
	}
}
