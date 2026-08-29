package auth

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds everything the verifier needs to validate a Stytch B2B session
// JWT offline. Every value is either a public identifier or a URL — there are
// no secrets here. The Stytch project's signing keys are fetched as public JWKS;
// the API never holds Stytch private material. Secrets that other parts of the
// system need (GitHub App key, webhook HMAC, Stytch management credentials) come
// from managed storage and are out of scope for token validation.
type Config struct {
	// ProjectID is the Stytch B2B project ID. It is the expected `aud` claim and
	// is used to derive the default issuer and JWKS URL.
	ProjectID string

	// Issuer is the expected `iss` claim. Stytch stamps session JWTs with an
	// issuer of "stytch.com/<project_id>". Overridable for custom domains.
	Issuer string

	// Audience is the expected `aud` claim. Defaults to ProjectID.
	Audience string

	// JWKSURL is the endpoint serving the project's public signing keys. Stytch
	// B2B publishes them at
	// https://api.stytch.com/v1/b2b/sessions/jwks/<project_id>. Overridable for
	// the test environment (test.stytch.com) or a custom domain.
	JWKSURL string

	// MaxTokenAge bounds how old a token may be, measured from its `iat`, on top
	// of the `exp` check. Stytch session JWTs have a fixed five-minute lifetime,
	// so a small clamp here rejects a token whose `exp` was somehow minted far in
	// the future. A little slack absorbs clock skew.
	MaxTokenAge time.Duration

	// ClockSkew is the leeway allowed when comparing time-based claims, to
	// tolerate small clock differences between the issuer and this service.
	ClockSkew time.Duration

	// RefreshInterval is how often the JWKS cache refetches keys in the
	// background. Signing keys rotate roughly every six months with a one-month
	// overlap, so any interval well under the overlap keeps both keys cached
	// through a rotation. It does not gate the request path.
	RefreshInterval time.Duration

	// RefreshMinInterval throttles unscheduled refreshes triggered by seeing an
	// unknown key ID, so a burst of tokens signed by an unknown key cannot cause
	// a burst of JWKS fetches.
	RefreshMinInterval time.Duration
}

// Defaults for the tunables. The five-minute lifetime plus generous skew is the
// documented Stytch behaviour; the refresh cadence is far tighter than the
// one-month key overlap so a rotation is never observed as a signature failure.
const (
	defaultMaxTokenAge        = 5*time.Minute + 30*time.Second
	defaultClockSkew          = 30 * time.Second
	defaultRefreshInterval    = 15 * time.Minute
	defaultRefreshMinInterval = 5 * time.Minute
	stytchDefaultJWKSHost     = "https://api.stytch.com"
)

// LoadConfigFromEnv builds a Config from environment variables. Secrets and
// deployment-specific identifiers live in the environment (populated from
// managed secret storage), never in the repository or the image.
//
//	STYTCH_PROJECT_ID   required — the B2B project ID (also the audience)
//	STYTCH_ISSUER       optional — defaults to "stytch.com/<project_id>"
//	STYTCH_AUDIENCE     optional — defaults to the project ID
//	STYTCH_JWKS_URL     optional — defaults to the api.stytch.com B2B JWKS URL
//	STYTCH_MAX_TOKEN_AGE optional — a Go duration, e.g. "5m30s"
func LoadConfigFromEnv() (Config, error) {
	cfg := Config{
		ProjectID: strings.TrimSpace(os.Getenv("STYTCH_PROJECT_ID")),
		Issuer:    strings.TrimSpace(os.Getenv("STYTCH_ISSUER")),
		Audience:  strings.TrimSpace(os.Getenv("STYTCH_AUDIENCE")),
		JWKSURL:   strings.TrimSpace(os.Getenv("STYTCH_JWKS_URL")),
	}

	if raw := strings.TrimSpace(os.Getenv("STYTCH_MAX_TOKEN_AGE")); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("auth: invalid STYTCH_MAX_TOKEN_AGE %q: %w", raw, err)
		}
		cfg.MaxTokenAge = d
	}

	return cfg.withDefaults()
}

// withDefaults fills unset fields and validates the required ones.
func (c Config) withDefaults() (Config, error) {
	if c.ProjectID == "" {
		return Config{}, fmt.Errorf("auth: %w: ProjectID is required", ErrInvalidClaims)
	}
	if c.Issuer == "" {
		c.Issuer = "stytch.com/" + c.ProjectID
	}
	if c.Audience == "" {
		c.Audience = c.ProjectID
	}
	if c.JWKSURL == "" {
		c.JWKSURL = fmt.Sprintf("%s/v1/b2b/sessions/jwks/%s", stytchDefaultJWKSHost, c.ProjectID)
	}
	if c.MaxTokenAge <= 0 {
		c.MaxTokenAge = defaultMaxTokenAge
	}
	if c.ClockSkew <= 0 {
		c.ClockSkew = defaultClockSkew
	}
	if c.RefreshInterval <= 0 {
		c.RefreshInterval = defaultRefreshInterval
	}
	if c.RefreshMinInterval <= 0 {
		c.RefreshMinInterval = defaultRefreshMinInterval
	}
	return c, nil
}
