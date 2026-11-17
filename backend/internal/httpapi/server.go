// Package httpapi implements the HTTP handlers for openapi/openapi.yaml.
//
// openapi.gen.go is generated from the contract (make gen-go) and must not be
// edited by hand. This package wires domain repositories, authentication,
// CSRF protection, and rate limiting into net/http.
package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/zSamir015/agenda-clinicas/backend/internal/auth"
	"github.com/zSamir015/agenda-clinicas/backend/internal/platform"
	"github.com/zSamir015/agenda-clinicas/backend/internal/rbac"
	"github.com/zSamir015/agenda-clinicas/backend/internal/users"
)

// readyTimeout bounds the database check. Without it a stopped database makes
// /readyz hang for the driver's 30 s server-selection timeout, and the
// orchestrator cannot tell "slow" from "down".
const readyTimeout = 2 * time.Second

// Pinger reports whether a dependency is reachable. *mongodb.Client satisfies
// it, and tests use a fake, so handlers never need a real database.
type Pinger interface {
	Ping(ctx context.Context) error
}

// OTPSender delivers a 6-digit verification code to a patient contact.
type OTPSender interface {
	SendPatientOTP(ctx context.Context, clinic platform.Clinic, contact, code string) error
}

type noopOTPSender struct{}

func (noopOTPSender) SendPatientOTP(context.Context, platform.Clinic, string, string) error {
	return nil
}

// AuthDeps bundles the dependencies needed for Auth, RBAC, Users, and OTP endpoints.
type AuthDeps struct {
	Clinics           *platform.ClinicRepository
	Roles             *rbac.RoleRepository
	Users             *users.Repository
	Sessions          *auth.SessionRepository
	Creds             *auth.CredentialRepository
	OTPs              *auth.OTPRepository
	WebAuthn          *auth.WebAuthnService
	Limiter           *auth.RateLimiter
	OTPSender         OTPSender
	CookieName        string
	CookieSecure      bool
	AllowedOrigins    []string
	LoginRateLimit    int
	OTPRateLimit      int
	RateLimitWindow   time.Duration
	SessionTTL        time.Duration
	PatientSessionTTL time.Duration
	InvitationTTL     time.Duration
	TOTPIssuer        string
}

// Option configures a Server.
type Option func(*Server)

// WithAuth wires authentication, RBAC, and user management dependencies.
func WithAuth(deps AuthDeps) Option {
	return func(s *Server) {
		if deps.OTPSender == nil {
			deps.OTPSender = noopOTPSender{}
		}
		if deps.CookieName == "" {
			deps.CookieName = auth.DefaultCookieName
		}
		if deps.LoginRateLimit <= 0 {
			deps.LoginRateLimit = 30
		}
		if deps.OTPRateLimit <= 0 {
			deps.OTPRateLimit = 15
		}
		if deps.RateLimitWindow <= 0 {
			deps.RateLimitWindow = time.Minute
		}
		if deps.SessionTTL <= 0 {
			deps.SessionTTL = 12 * time.Hour
		}
		if deps.PatientSessionTTL <= 0 {
			deps.PatientSessionTTL = 30 * time.Minute
		}
		if deps.InvitationTTL <= 0 {
			deps.InvitationTTL = 7 * 24 * time.Hour
		}
		if deps.TOTPIssuer == "" {
			deps.TOTPIssuer = "Agenda Clínicas"
		}
		s.auth = deps
	}
}

// Server implements StrictServerInterface.
type Server struct {
	db         Pinger
	auth       AuthDeps
	challenges *challengeStore
}

var _ StrictServerInterface = (*Server)(nil)

// NewServer returns a Server that checks db for readiness and handles API routes.
func NewServer(db Pinger, opts ...Option) *Server {
	s := &Server{
		db:         db,
		challenges: newChallengeStore(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

func (s *Server) authEnabled() bool {
	return s.auth.Sessions != nil && s.auth.Clinics != nil && s.auth.Roles != nil && s.auth.Users != nil
}

// NewHandler wires the strict server into a net/http mux with CORS, rate
// limiting on auth entry points, and session/CSRF checks on protected routes.
func NewHandler(s *Server) http.Handler {
	base := HandlerFromMux(NewStrictHandler(s, nil), http.NewServeMux())
	if !s.authEnabled() {
		return base
	}

	cookieName := s.auth.CookieName
	if cookieName == "" {
		cookieName = auth.DefaultCookieName
	}

	protected := s.requireSessionMiddleware(cookieName)(base)
	optionalSession := s.optionalSessionMiddleware(cookieName)
	loginLimited := s.rateLimitProblem("login", s.auth.LoginRateLimit, s.auth.RateLimitWindow)(base)
	otpLimited := s.rateLimitProblem("otp", s.auth.OTPRateLimit, s.auth.RateLimitWindow)(base)

	dispatch := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		switch {
		case path == "/healthz" || path == "/readyz":
			base.ServeHTTP(w, r)
		case path == "/auth/login/password" || path == "/auth/login/passkey/begin" || path == "/auth/login/passkey/finish":
			loginLimited.ServeHTTP(w, r)
		case path == "/auth/invitations/preview":
			base.ServeHTTP(w, r)
		case path == "/auth/invitations/accept":
			optionalSession(otpLimited).ServeHTTP(w, r)
		case path == "/auth/patient/otp/send" || path == "/auth/patient/otp/verify":
			otpLimited.ServeHTTP(w, r)
		case strings.HasPrefix(path, "/auth/") || strings.HasPrefix(path, "/rbac/") || strings.HasPrefix(path, "/users/"):
			protected.ServeHTTP(w, r)
		default:
			base.ServeHTTP(w, r)
		}
	})

	return s.corsMiddleware(dispatch)
}

type rawTokenKey struct{}

func withRawSessionToken(ctx context.Context, raw string) context.Context {
	return context.WithValue(ctx, rawTokenKey{}, raw)
}

func rawSessionTokenFrom(ctx context.Context) (string, bool) {
	raw, ok := ctx.Value(rawTokenKey{}).(string)
	return raw, ok && raw != ""
}

func (s *Server) requireSessionMiddleware(cookieName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(cookieName)
			if err != nil || cookie.Value == "" {
				writeProblem(w, http.StatusUnauthorized, "Sesión requerida", "unauthorized", "Inicia sesión para continuar.")
				return
			}

			sess, err := s.auth.Sessions.Validate(r.Context(), cookie.Value)
			if err != nil {
				writeProblem(w, http.StatusUnauthorized, "Sesión inválida o expirada", "unauthorized", "Vuelve a iniciar sesión.")
				return
			}

			if isMutatingMethod(r.Method) {
				csrf := r.Header.Get(auth.DefaultCSRFHeader)
				if csrf == "" || sess.CSRFToken == "" || subtle.ConstantTimeCompare([]byte(csrf), []byte(sess.CSRFToken)) != 1 {
					writeProblem(w, http.StatusForbidden, "Token CSRF inválido", "invalid_csrf", "Recarga la página e intenta nuevamente.")
					return
				}
			}

			ctx := auth.WithSession(r.Context(), sess)
			ctx = withRawSessionToken(ctx, cookie.Value)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// optionalSessionMiddleware attaches the session when the request carries a
// valid cookie (and, for mutating methods, a matching CSRF token). Anything
// else continues anonymously: the handler decides whether a session is needed.
func (s *Server) optionalSessionMiddleware(cookieName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(cookieName)
			if err != nil || cookie.Value == "" {
				next.ServeHTTP(w, r)
				return
			}
			sess, err := s.auth.Sessions.Validate(r.Context(), cookie.Value)
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			if isMutatingMethod(r.Method) {
				csrf := r.Header.Get(auth.DefaultCSRFHeader)
				if csrf == "" || sess.CSRFToken == "" || subtle.ConstantTimeCompare([]byte(csrf), []byte(sess.CSRFToken)) != 1 {
					next.ServeHTTP(w, r)
					return
				}
			}
			ctx := auth.WithSession(r.Context(), sess)
			ctx = withRawSessionToken(ctx, cookie.Value)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func (s *Server) rateLimitProblem(prefix string, limit int, window time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if s.auth.Limiter == nil {
				next.ServeHTTP(w, r)
				return
			}
			key := prefix + ":" + requestIP(r)
			allowed, retryAfter := s.auth.Limiter.Allow(key, limit, window)
			if !allowed {
				secs := max(1, int(retryAfter.Seconds()))
				w.Header().Set("Retry-After", strconv.Itoa(secs))
				writeProblem(w, http.StatusTooManyRequests, "Demasiados intentos", "rate_limited", "Espera unos segundos antes de volver a intentarlo.")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && slices.Contains(s.auth.AllowedOrigins, origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, "+auth.DefaultCSRFHeader)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

func requestIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeProblem(w http.ResponseWriter, status int, title, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	p := Problem{
		Title:  title,
		Status: status,
		Code:   code,
	}
	if detail != "" {
		p.Detail = &detail
	}
	_ = json.NewEncoder(w).Encode(p)
}

func newProblem(status int, title, code, detail string) Problem {
	p := Problem{
		Title:  title,
		Status: status,
		Code:   code,
	}
	if detail != "" {
		p.Detail = &detail
	}
	return p
}

// GetHealthz answers liveness. It never touches the database: a database
// outage must not make the orchestrator restart a healthy process.
func (s *Server) GetHealthz(context.Context, GetHealthzRequestObject) (GetHealthzResponseObject, error) {
	return GetHealthz200JSONResponse{Status: Ok}, nil
}

// GetReadyz answers readiness: 200 if MongoDB answers, 503 otherwise. The
// underlying error is not sent to the client because it can leak internals.
func (s *Server) GetReadyz(ctx context.Context, _ GetReadyzRequestObject) (GetReadyzResponseObject, error) {
	ctx, cancel := context.WithTimeout(ctx, readyTimeout)
	defer cancel()
	if err := s.db.Ping(ctx); err != nil {
		return GetReadyz503ApplicationProblemPlusJSONResponse{
			Title:  "Servicio no disponible",
			Status: http.StatusServiceUnavailable,
			Code:   "database_unavailable",
		}, nil
	}
	return GetReadyz200JSONResponse{Status: Ok}, nil
}
