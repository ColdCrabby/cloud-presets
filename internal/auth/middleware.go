package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

// TokenExtractor pulls the raw JWT out of a request. The default reads the
// Authorization: Bearer header. It is a field so a deployment can also accept a
// cookie if the frontends ever need one, without touching the middleware.
type TokenExtractor func(*http.Request) string

// BearerToken extracts a token from an "Authorization: Bearer <jwt>" header.
func BearerToken(r *http.Request) string {
	return BearerFromHeader(r.Header.Get("Authorization"))
}

// BearerFromHeader extracts the token from a raw Authorization header value.
// It is exported so callers that only have the header string — such as the
// Huma operation middleware, which reads headers off the huma.Context rather
// than an *http.Request — can reuse the exact same parsing.
func BearerFromHeader(h string) string {
	if h == "" {
		return ""
	}
	const prefix = "Bearer "
	if len(h) > len(prefix) && strings.EqualFold(h[:len(prefix)], prefix) {
		return strings.TrimSpace(h[len(prefix):])
	}
	return ""
}

// Middleware authenticates requests using a Verifier.
type Middleware struct {
	verifier   *Verifier
	extract    TokenExtractor
	writeError func(http.ResponseWriter, *http.Request, error)
}

// NewMiddleware builds authentication middleware around a Verifier.
func NewMiddleware(v *Verifier) *Middleware {
	return &Middleware{
		verifier:   v,
		extract:    BearerToken,
		writeError: writeUnauthorized,
	}
}

// WithExtractor overrides how the token is read from the request.
func (m *Middleware) WithExtractor(e TokenExtractor) *Middleware {
	m.extract = e
	return m
}

// WithErrorWriter overrides how a 401 is rendered.
func (m *Middleware) WithErrorWriter(w func(http.ResponseWriter, *http.Request, error)) *Middleware {
	m.writeError = w
	return m
}

// Verify validates a raw session token and returns its claims. It is the seam
// the Huma operation middleware uses: that path reads the token off the
// huma.Context and needs to run the same verification as RequireAuth without an
// *http.Request. The reason helpers below translate any failure into a stable
// message.
func (m *Middleware) Verify(ctx context.Context, token string) (Claims, error) {
	return m.verifier.Verify(ctx, token)
}

// Reason renders a stable, client-safe description of an authentication error,
// matching what RequireAuth puts in its 401. Exported so the Huma middleware
// produces identical wording.
func Reason(err error) string { return reason(err) }

// RequireAuth wraps next so it only runs for requests carrying a valid session
// JWT. On any authentication failure it writes a 401 and does not call next —
// deliberately distinct from the 403 an ownership check returns later, because
// the admin app silently refreshes on 401 but must explain a 403. Handlers
// behind this can read claims with ClaimsFromContext.
func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, err := m.verifier.Verify(r.Context(), m.extract(r))
		if err != nil {
			m.writeError(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(WithClaims(r.Context(), claims)))
	})
}

// Authenticate is RequireAuth in http.HandlerFunc-wrapping form, convenient for
// wrapping a single handler function.
func (m *Middleware) Authenticate(next http.HandlerFunc) http.HandlerFunc {
	return m.RequireAuth(next).ServeHTTP
}

func writeUnauthorized(w http.ResponseWriter, _ *http.Request, err error) {
	// A WWW-Authenticate header lets a client tell "no/expired token" apart at
	// the protocol level; error_description carries the specific reason.
	w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token", error_description="`+reason(err)+`"`)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"status": http.StatusUnauthorized,
		"title":  "Unauthorized",
		"detail": reason(err),
	})
}

func reason(err error) string {
	switch {
	case errors.Is(err, ErrMissingToken):
		return "missing session token"
	case errors.Is(err, ErrExpired):
		return "session token expired"
	case errors.Is(err, ErrMalformedToken):
		return "malformed session token"
	case errors.Is(err, ErrInvalidSignature):
		return "invalid token signature"
	case errors.Is(err, ErrInvalidClaims):
		return "invalid token claims"
	default:
		return "invalid session token"
	}
}
