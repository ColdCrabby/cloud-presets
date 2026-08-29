package auth

import "context"

// Claims are the parts of a validated Stytch B2B session JWT that handlers act
// on. Identity lives here; authority does not. Comparing organization_id against
// the vendor manifest at the current `main` is a separate step (a 403 on
// mismatch), described in docs/vendor-workflow.md.
type Claims struct {
	// Subject is the Stytch member ID (the `sub` claim): the person signing in.
	Subject string

	// OrganizationID is the Stytch organization ID — the vendor company the
	// session is scoped to. This, not any client-supplied parameter, is the
	// caller's namespace. Sourced from the "organization_id" claim.
	OrganizationID string

	// OrganizationSlug is the human-readable organization slug, surfaced for
	// logging and UI. Sourced from the "organization_slug" claim.
	OrganizationSlug string

	// Roles are the member's roles within the organization, distinguishing
	// proposing a change from administering the organization. Sourced from the
	// "roles" claim.
	Roles []string

	// IssuedAt and ExpiresAt are unix seconds from the validated `iat`/`exp`.
	IssuedAt  int64
	ExpiresAt int64
}

// HasRole reports whether the member holds the named role.
func (c Claims) HasRole(role string) bool {
	for _, r := range c.Roles {
		if r == role {
			return true
		}
	}
	return false
}

type contextKey struct{}

var claimsKey contextKey

// WithClaims returns a copy of ctx carrying the validated claims.
func WithClaims(ctx context.Context, claims Claims) context.Context {
	return context.WithValue(ctx, claimsKey, claims)
}

// ClaimsFromContext returns the validated claims stored on the context by the
// middleware, and whether they were present. Handlers behind RequireAuth can
// rely on ok being true.
func ClaimsFromContext(ctx context.Context) (Claims, bool) {
	claims, ok := ctx.Value(claimsKey).(Claims)
	return claims, ok
}
