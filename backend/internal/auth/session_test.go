package auth

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb"
	"github.com/zSamir015/agenda-clinicas/backend/internal/mongodb/mongotest"
	"github.com/zSamir015/agenda-clinicas/backend/internal/rbac"
)

func TestRateLimiter(t *testing.T) {
	now := time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)
	lim := NewRateLimiter()
	lim.now = func() time.Time { return now }

	for i := range 3 {
		ok, _ := lim.Allow("login:1.2.3.4", 3, time.Minute)
		if !ok {
			t.Fatalf("attempt %d should be allowed", i+1)
		}
	}

	ok, wait := lim.Allow("login:1.2.3.4", 3, time.Minute)
	if ok || wait <= 0 {
		t.Fatalf("4th attempt should be blocked, got ok=%v wait=%v", ok, wait)
	}

	now = now.Add(time.Minute)
	if ok, _ := lim.Allow("login:1.2.3.4", 3, time.Minute); !ok {
		t.Fatal("attempt after window reset should be allowed")
	}
}

func TestSessionsCSRFAndRateLimitMiddleware(t *testing.T) {
	ctx := context.Background()
	cl, err := mongodb.Connect(ctx, mongotest.URI(t))
	if err != nil {
		t.Fatalf("mongodb.Connect: %v", err)
	}
	t.Cleanup(func() { _ = cl.Close(context.Background()) })

	sessions, err := NewSessionRepository(ctx, cl.Mongo().Database("platform"))
	if err != nil {
		t.Fatalf("NewSessionRepository: %v", err)
	}

	userID := bson.NewObjectID()
	clinicID := bson.NewObjectID()
	roleID := bson.NewObjectID()
	initialGrants := []rbac.Grant{{Permission: rbac.PermAppointmentsRead, Scope: rbac.ScopeOwn}}

	sess, rawToken, err := sessions.CreateStaffSession(ctx, userID, clinicID, roleID, initialGrants, time.Hour)
	if err != nil {
		t.Fatalf("CreateStaffSession: %v", err)
	}
	if rawToken == "" || sess.TokenHash == rawToken || sess.CSRFToken == "" {
		t.Fatal("expected hashed session token and non-empty CSRF token")
	}

	t.Run("role cache invalidation updates active sessions", func(t *testing.T) {
		newGrants := []rbac.Grant{{Permission: rbac.PermAppointmentsRead, Scope: rbac.ScopeAll}}
		if err := sessions.InvalidateRoleCache(ctx, clinicID, roleID, newGrants); err != nil {
			t.Fatalf("InvalidateRoleCache: %v", err)
		}
		got, err := sessions.Validate(ctx, rawToken)
		if err != nil {
			t.Fatalf("Validate after role update: %v", err)
		}
		if len(got.Grants) != 1 || got.Grants[0].Scope != rbac.ScopeAll {
			t.Fatalf("expected updated grants in session, got %+v", got.Grants)
		}
	})

	t.Run("RequireAuth enforces session cookie and CSRF on mutations", func(t *testing.T) {
		protected := RequireAuth(sessions, "", "")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s, ok := SessionFrom(r.Context())
			if !ok || s.UserID != userID {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))

		// No cookie -> 401
		req := httptest.NewRequest(http.MethodGet, "/panel", nil)
		rec := httptest.NewRecorder()
		protected.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("no cookie: got %d, want 401", rec.Code)
		}

		testCookie := &http.Cookie{
			Name:     DefaultCookieName,
			Value:    rawToken,
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		}

		// Valid cookie on GET -> 200
		req = httptest.NewRequest(http.MethodGet, "/panel", nil)
		req.AddCookie(testCookie)
		rec = httptest.NewRecorder()
		protected.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("valid GET: got %d, want 200", rec.Code)
		}

		// Valid cookie on POST without CSRF -> 403
		req = httptest.NewRequest(http.MethodPost, "/panel/citas", nil)
		req.AddCookie(testCookie)
		rec = httptest.NewRecorder()
		protected.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("POST without CSRF: got %d, want 403", rec.Code)
		}

		// Valid cookie + valid CSRF on POST -> 200
		req = httptest.NewRequest(http.MethodPost, "/panel/citas", nil)
		req.AddCookie(testCookie)
		req.Header.Set(DefaultCSRFHeader, sess.CSRFToken)
		rec = httptest.NewRecorder()
		protected.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("POST with CSRF: got %d, want 200", rec.Code)
		}
	})

	t.Run("immediate revocation blocks subsequent requests", func(t *testing.T) {
		if err := sessions.Revoke(ctx, rawToken); err != nil {
			t.Fatalf("Revoke: %v", err)
		}
		if _, err := sessions.Validate(ctx, rawToken); !errors.Is(err, ErrSessionRevoked) {
			t.Fatalf("Validate revoked: got %v, want ErrSessionRevoked", err)
		}
	})

	t.Run("patient session and rate limit middleware", func(t *testing.T) {
		pSess, pTok, err := sessions.CreatePatientSession(ctx, clinicID, "+50760000001", 30*time.Minute)
		if err != nil {
			t.Fatalf("CreatePatientSession: %v", err)
		}
		if pSess.Kind != SessionPatient {
			t.Fatalf("expected patient session, got %s", pSess.Kind)
		}
		if _, err := sessions.Validate(ctx, pTok); err != nil {
			t.Fatalf("Validate patient session: %v", err)
		}

		lim := NewRateLimiter()
		limited := RateLimitMiddleware(lim, "otp", 2, time.Minute, nil)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		for range 2 {
			req := httptest.NewRequest(http.MethodPost, "/otp", nil)
			req.RemoteAddr = "10.0.0.5:1234"
			rec := httptest.NewRecorder()
			limited.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200 under limit, got %d", rec.Code)
			}
		}

		req := httptest.NewRequest(http.MethodPost, "/otp", nil)
		req.RemoteAddr = "10.0.0.5:1234"
		rec := httptest.NewRecorder()
		limited.ServeHTTP(rec, req)
		if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
			t.Fatalf("expected 429 with Retry-After, got %d (Retry-After=%q)", rec.Code, rec.Header().Get("Retry-After"))
		}
	})
}
