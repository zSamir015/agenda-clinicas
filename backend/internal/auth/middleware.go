package auth

import (
	"context"
	"crypto/subtle"
	"net"
	"net/http"
	"strconv"
	"time"
)

// DefaultCookieName and DefaultCSRFHeader define the standard HTTP session
// cookie and CSRF header names.
const (
	DefaultCookieName = "agenda_session"
	DefaultCSRFHeader = "X-CSRF-Token"
)

type sessionContextKey struct{}

// WithSession attaches s to ctx.
func WithSession(ctx context.Context, s Session) context.Context {
	return context.WithValue(ctx, sessionContextKey{}, s)
}

// SessionFrom extracts the authenticated Session from ctx.
func SessionFrom(ctx context.Context) (Session, bool) {
	s, ok := ctx.Value(sessionContextKey{}).(Session)
	return s, ok
}

// SetSessionCookie writes the session cookie with HttpOnly, SameSite=Lax, and
// optional Secure flag.
func SetSessionCookie(w http.ResponseWriter, name, rawToken string, ttl time.Duration, secure bool) {
	if name == "" {
		name = DefaultCookieName
	}
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure is controlled by environment (true in production, false only in local HTTP dev)
		Name:     name,
		Value:    rawToken,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookie expires the session cookie on the client.
func ClearSessionCookie(w http.ResponseWriter, name string, secure bool) {
	if name == "" {
		name = DefaultCookieName
	}
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secure is controlled by environment (true in production, false only in local HTTP dev)
		Name:     name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// RequireAuth validates the session cookie and, on mutating HTTP methods,
// verifies the CSRF token header in constant time.
func RequireAuth(sessions *SessionRepository, cookieName, csrfHeader string) func(http.Handler) http.Handler {
	if cookieName == "" {
		cookieName = DefaultCookieName
	}
	if csrfHeader == "" {
		csrfHeader = DefaultCSRFHeader
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(cookieName)
			if err != nil || cookie.Value == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			sess, err := sessions.Validate(r.Context(), cookie.Value)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			if isMutatingMethod(r.Method) {
				got := r.Header.Get(csrfHeader)
				if !tokensEqual(got, sess.CSRFToken) {
					http.Error(w, "invalid csrf token", http.StatusForbidden)
					return
				}
			}

			next.ServeHTTP(w, r.WithContext(WithSession(r.Context(), sess)))
		})
	}
}

// RateLimitMiddleware enforces limit requests per window keyed by keyFn (or client IP when nil).
func RateLimitMiddleware(limiter *RateLimiter, prefix string, limit int, window time.Duration, keyFn func(*http.Request) string) func(http.Handler) http.Handler {
	if keyFn == nil {
		keyFn = clientIP
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			allowed, retryAfter := limiter.Allow(prefix+":"+keyFn(r), limit, window)
			if !allowed {
				secs := max(1, int(retryAfter.Seconds()))
				w.Header().Set("Retry-After", strconv.Itoa(secs))
				http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

func tokensEqual(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
