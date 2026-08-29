package auth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwt"
)

// Verifier validates Stytch B2B session JWTs offline against a cached JWKS.
// Construct one at startup with New and share it; it is safe for concurrent use.
type Verifier struct {
	cfg  Config
	keys *keyCache

	// now is overridable in tests; defaults to time.Now.
	now func() time.Time
}

// New builds a Verifier, applying defaults to cfg and performing the initial
// JWKS fetch. The passed ctx governs the lifetime of the background JWKS
// refresh goroutine — cancel it on shutdown.
func New(ctx context.Context, cfg Config) (*Verifier, error) {
	full, err := cfg.withDefaults()
	if err != nil {
		return nil, err
	}
	keys, err := newKeyCache(ctx, full)
	if err != nil {
		return nil, err
	}
	return &Verifier{cfg: full, keys: keys, now: time.Now}, nil
}

// Verify validates a raw JWT string and returns the claims a handler acts on.
// It performs no network call on the request path: the signature is checked
// against the in-memory JWKS cache. Every returned error is a 401-class error
// (see errors.go); authorization against the vendor manifest is a separate step.
//
// Validation order is signature first, then registered claims, so a token with
// a bad signature never has its (untrusted) claims inspected.
func (v *Verifier) Verify(ctx context.Context, raw string) (Claims, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Claims{}, ErrMissingToken
	}

	// Stage 1: is it even a JWT? Parse structure only, no verify/validate, to
	// separate "malformed" from "bad signature".
	if _, err := jwt.Parse([]byte(raw), jwt.WithVerify(false), jwt.WithValidate(false)); err != nil {
		return Claims{}, fmt.Errorf("%w: %v", ErrMalformedToken, err)
	}

	// Stage 2: verify the signature against the cached keys. If it fails, a key
	// may have rotated in ahead of the scheduled refresh, so refresh once
	// (throttled) and retry before giving up.
	token, err := jwt.Parse([]byte(raw), jwt.WithKeySet(v.keys.keySet()), jwt.WithValidate(false))
	if err != nil {
		if v.keys.refreshIfStale(ctx) {
			token, err = jwt.Parse([]byte(raw), jwt.WithKeySet(v.keys.keySet()), jwt.WithValidate(false))
		}
		if err != nil {
			return Claims{}, fmt.Errorf("%w: %v", ErrInvalidSignature, err)
		}
	}

	// Stage 3: validate registered claims ourselves, so each failure maps to a
	// precise typed error (expired vs. wrong issuer vs. too old).
	if err := v.validateRegistered(token); err != nil {
		return Claims{}, err
	}

	return extractClaims(token), nil
}

// validateRegistered checks iss, aud, iat, exp and the maximum token age, with
// a small skew allowance, against a signature-verified token.
func (v *Verifier) validateRegistered(token jwt.Token) error {
	if token.Issuer() != v.cfg.Issuer {
		return fmt.Errorf("%w: unexpected issuer %q", ErrInvalidClaims, token.Issuer())
	}

	if !containsString(token.Audience(), v.cfg.Audience) {
		return fmt.Errorf("%w: audience does not include %q", ErrInvalidClaims, v.cfg.Audience)
	}

	iat := token.IssuedAt()
	if iat.IsZero() {
		return fmt.Errorf("%w: missing iat", ErrInvalidClaims)
	}
	exp := token.Expiration()
	if exp.IsZero() {
		return fmt.Errorf("%w: missing exp", ErrInvalidClaims)
	}

	now := v.now()
	skew := v.cfg.ClockSkew

	if now.After(exp.Add(skew)) {
		return fmt.Errorf("%w: token expired at %s", ErrExpired, exp.UTC().Format(time.RFC3339))
	}
	if iat.After(now.Add(skew)) {
		return fmt.Errorf("%w: token issued in the future", ErrInvalidClaims)
	}
	if now.Sub(iat) > v.cfg.MaxTokenAge+skew {
		return fmt.Errorf("%w: token older than max age %s", ErrExpired, v.cfg.MaxTokenAge)
	}

	return nil
}

// extractClaims pulls the identity claims handlers care about off a validated
// token. Authority is not decided here.
func extractClaims(token jwt.Token) Claims {
	return Claims{
		Subject:          token.Subject(),
		OrganizationID:   claimString(token, "organization_id"),
		OrganizationSlug: claimString(token, "organization_slug"),
		Roles:            claimStringSlice(token, "roles"),
		IssuedAt:         token.IssuedAt().Unix(),
		ExpiresAt:        token.Expiration().Unix(),
	}
}

func claimString(token jwt.Token, name string) string {
	v, ok := token.Get(name)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

func claimStringSlice(token jwt.Token, name string) []string {
	v, ok := token.Get(name)
	if !ok {
		return nil
	}
	switch vv := v.(type) {
	case []string:
		return vv
	case []interface{}:
		out := make([]string, 0, len(vv))
		for _, e := range vv {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func containsString(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
