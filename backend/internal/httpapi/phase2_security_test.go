package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
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

type secEnv struct {
	h         http.Handler
	users     *users.Repository
	sessions  *auth.SessionRepository
	clinic    platform.Clinic
	owner     rbac.Role
	admin     rbac.Role
	reception rbac.Role
}

type actor struct {
	userID string
	cookie *http.Cookie
	csrf   string
}

func newSecEnv(t *testing.T) *secEnv {
	t.Helper()
	ctx := context.Background()
	cl, err := mongodb.Connect(ctx, mongotest.URI(t))
	if err != nil {
		t.Fatalf("mongodb.Connect: %v", err)
	}
	t.Cleanup(func() { _ = cl.Close(context.Background()) })
	db := cl.Mongo().Database("platform_sec")

	clinics, err := platform.NewClinicRepository(ctx, db)
	must(t, err)
	roles, err := rbac.NewRoleRepository(ctx, db)
	must(t, err)
	userRepo, err := users.NewRepository(ctx, db)
	must(t, err)
	sessions, err := auth.NewSessionRepository(ctx, db)
	must(t, err)
	creds, err := auth.NewCredentialRepository(ctx, db)
	must(t, err)
	otps, err := auth.NewOTPRepository(ctx, db)
	must(t, err)
	wa, err := auth.NewWebAuthnService("Agenda Test", "localhost", []string{"http://localhost:3000"}, creds)
	must(t, err)

	srv := NewServer(cl, WithAuth(AuthDeps{
		Clinics: clinics, Roles: roles, Users: userRepo, Sessions: sessions,
		Creds: creds, OTPs: otps, WebAuthn: wa, Limiter: auth.NewRateLimiter(),
		OTPSender: newRecordingOTPSender(), LoginRateLimit: 100, OTPRateLimit: 100,
		AllowedOrigins: []string{"http://localhost:3000"},
	}))

	in, err := platform.NewClinic("clinica-sec-"+bson.NewObjectID().Hex()[14:], "Clínica Sec Test", platform.TierStandard, "pa")
	must(t, err)
	clinic, err := clinics.Create(ctx, in)
	must(t, err)
	must(t, roles.EnsureTemplates(ctx, clinic.ID))
	env := &secEnv{h: NewHandler(srv), users: userRepo, sessions: sessions, clinic: clinic}
	env.owner, err = roles.ByName(ctx, clinic.ID, rbac.RoleOwner)
	must(t, err)
	env.admin, err = roles.ByName(ctx, clinic.ID, rbac.RoleAdmin)
	must(t, err)
	env.reception, err = roles.ByName(ctx, clinic.ID, rbac.RoleReception)
	must(t, err)
	return env
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// join creates a brand-new member through the real invitation flow.
func (e *secEnv) join(t *testing.T, email string, role rbac.Role) actor {
	t.Helper()
	_, token, err := e.users.CreateInvitation(context.Background(), e.clinic.ID, email, "Miembro "+email, role.ID, bson.NewObjectID(), time.Hour)
	must(t, err)
	rec := doJSON(t, e.h, http.MethodPost, "/auth/invitations/accept", map[string]string{"token": token}, nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("join %s: %d %s", email, rec.Code, rec.Body.String())
	}
	s := decodeJSON[Session](t, rec)
	return actor{userID: *s.UserId, cookie: firstCookie(rec, auth.DefaultCookieName), csrf: s.CsrfToken}
}

func (e *secEnv) do(t *testing.T, a actor, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	return doJSON(t, e.h, method, path, body, a.cookie, a.csrf)
}

func TestInvitationAcceptDoesNotGrantSessionOnExistingAccount(t *testing.T) {
	e := newSecEnv(t)
	owner := e.join(t, "owner@sec.example", e.owner)

	victim, err := e.users.CreateUser(context.Background(), "victim@sec.example", "Víctima")
	must(t, err)

	// A clinic admin invites the victim's email and receives the raw token.
	rec := e.do(t, owner, http.MethodPost, "/users/invitations", CreateInvitationRequest{
		Email: "victim@sec.example", Name: "Víctima", RoleId: e.reception.ID.Hex(),
	})
	if rec.Code != http.StatusCreated {
		t.Fatalf("create invitation: %d %s", rec.Code, rec.Body.String())
	}
	token := decodeJSON[CreatedInvitation](t, rec).Token

	// Using the token without being the victim must not open a session.
	rec = doJSON(t, e.h, http.MethodPost, "/auth/invitations/accept", map[string]string{"token": token}, nil, "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("anonymous accept for existing account: got %d, want 409 (%s)", rec.Code, rec.Body.String())
	}
	if c := firstCookie(rec, auth.DefaultCookieName); c != nil {
		t.Fatal("a session cookie was issued for an existing account")
	}
	// A different logged-in user is not the victim either.
	rec = e.do(t, owner, http.MethodPost, "/auth/invitations/accept", map[string]string{"token": token})
	if rec.Code != http.StatusConflict {
		t.Fatalf("accept as another user: got %d, want 409", rec.Code)
	}
	// The invitation was not burned by the failed attempts.
	if rec = doJSON(t, e.h, http.MethodPost, "/auth/invitations/preview", map[string]string{"token": token}, nil, ""); rec.Code != http.StatusOK {
		t.Fatalf("invitation consumed by a rejected accept: %d", rec.Code)
	}

	// The real owner of the account, logged in, can accept it.
	_, raw, err := e.sessions.CreateStaffSession(context.Background(), victim.ID, e.clinic.ID, e.reception.ID, e.reception.Grants, time.Hour)
	must(t, err)
	sess, err := e.sessions.Validate(context.Background(), raw)
	must(t, err)
	victimActor := actor{userID: victim.ID.Hex(), cookie: &http.Cookie{Name: auth.DefaultCookieName, Value: raw}, //nolint:gosec // test-only request cookie
		csrf: sess.CSRFToken}
	rec = e.do(t, victimActor, http.MethodPost, "/auth/invitations/accept", map[string]string{"token": token})
	if rec.Code != http.StatusOK {
		t.Fatalf("accept as the account owner: %d %s", rec.Code, rec.Body.String())
	}
}

func TestAdminCannotTakeOverOwnerRole(t *testing.T) {
	e := newSecEnv(t)
	owner := e.join(t, "owner2@sec.example", e.owner)
	admin := e.join(t, "admin@sec.example", e.admin)

	forbidden := func(name string, rec *httptest.ResponseRecorder) {
		t.Helper()
		if rec.Code != http.StatusForbidden {
			t.Fatalf("%s: got %d, want 403 (%s)", name, rec.Code, rec.Body.String())
		}
	}

	forbidden("admin promotes self to owner",
		e.do(t, admin, http.MethodPut, "/users/members/"+admin.userID+"/role", UpdateMemberRoleRequest{RoleId: e.owner.ID.Hex()}))
	forbidden("admin invites an owner",
		e.do(t, admin, http.MethodPost, "/users/invitations", CreateInvitationRequest{Email: "x@sec.example", Name: "X", RoleId: e.owner.ID.Hex()}))
	forbidden("admin demotes the owner",
		e.do(t, admin, http.MethodPut, "/users/members/"+owner.userID+"/role", UpdateMemberRoleRequest{RoleId: e.reception.ID.Hex()}))
	forbidden("admin suspends the owner",
		e.do(t, admin, http.MethodDelete, "/users/members/"+owner.userID, nil))

	// An owner can still promote an admin.
	rec := e.do(t, owner, http.MethodPut, "/users/members/"+admin.userID+"/role", UpdateMemberRoleRequest{RoleId: e.owner.ID.Hex()})
	if rec.Code != http.StatusOK {
		t.Fatalf("owner promotes admin: %d %s", rec.Code, rec.Body.String())
	}
}

func TestTOTPConfirmCannotReplaceExistingCredentials(t *testing.T) {
	e := newSecEnv(t)
	owner := e.join(t, "owner3@sec.example", e.owner)

	enroll := func(a actor, password string) *httptest.ResponseRecorder {
		setup, err := auth.GenerateTOTP("Agenda Test", "owner3@sec.example")
		must(t, err)
		code, err := totp.GenerateCode(setup.Secret, time.Now().UTC())
		must(t, err)
		return e.do(t, a, http.MethodPost, "/auth/totp/confirm", TotpConfirmRequest{Secret: setup.Secret, Code: code, Password: password})
	}

	// A second session of the same user, to prove enrollment revokes it.
	_, otherRaw, err := e.sessions.CreateStaffSession(context.Background(), mustOID(t, owner.userID), e.clinic.ID, e.owner.ID, e.owner.Grants, time.Hour)
	must(t, err)

	if rec := enroll(owner, "ClaveSegura2026!"); rec.Code != http.StatusOK {
		t.Fatalf("first enrollment: %d %s", rec.Code, rec.Body.String())
	}
	if _, err := e.sessions.Validate(context.Background(), otherRaw); err == nil {
		t.Fatal("enrolling credentials left another session of the user alive")
	}
	if _, err := e.sessions.Validate(context.Background(), owner.cookie.Value); err != nil {
		t.Fatalf("enrolling credentials revoked the current session: %v", err)
	}

	// A session alone must not overwrite the established password and TOTP.
	rec := enroll(owner, "ClaveDelAtacante99!")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("replacing credentials: got %d, want 400 (%s)", rec.Code, rec.Body.String())
	}
	if got := decodeJSON[Problem](t, rec).Code; got != "credentials_exist" {
		t.Fatalf("problem code = %q, want credentials_exist", got)
	}
}

func mustOID(t *testing.T, hex string) bson.ObjectID {
	t.Helper()
	id, err := bson.ObjectIDFromHex(hex)
	must(t, err)
	return id
}
