package auth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwk"
)

// keyCache wraps a jwk.Cache configured for a single JWKS endpoint. It fetches
// the project's public signing keys once at startup and refreshes them in the
// background, so validating a token never makes a network call. Because the
// cached set holds every key the endpoint publishes, a key rotation — during
// which Stytch serves both the outgoing and incoming key for a one-month
// overlap — is transparent: both keys are present, and whichever `kid` a token
// carries is found.
type keyCache struct {
	cache *jwk.Cache
	set   jwk.Set
	url   string

	// minRefresh throttles the extra, unscheduled refresh we trigger when a
	// token arrives signed by a key we do not yet have cached (a rotation that
	// outran the background interval), so a flood of such tokens cannot become a
	// flood of JWKS fetches.
	minRefresh time.Duration

	mu          sync.Mutex
	lastRefresh time.Time
}

// newKeyCache registers the JWKS URL and performs one blocking fetch so that a
// bad URL fails at startup rather than on the first request.
func newKeyCache(ctx context.Context, cfg Config) (*keyCache, error) {
	cache := jwk.NewCache(ctx)

	if err := cache.Register(
		cfg.JWKSURL,
		jwk.WithRefreshInterval(cfg.RefreshInterval),
		jwk.WithMinRefreshInterval(cfg.RefreshMinInterval),
	); err != nil {
		return nil, fmt.Errorf("auth: register JWKS %q: %w", cfg.JWKSURL, err)
	}

	if _, err := cache.Refresh(ctx, cfg.JWKSURL); err != nil {
		return nil, fmt.Errorf("auth: initial JWKS fetch %q: %w", cfg.JWKSURL, err)
	}

	return &keyCache{
		cache:      cache,
		set:        jwk.NewCachedSet(cache, cfg.JWKSURL),
		url:        cfg.JWKSURL,
		minRefresh: cfg.RefreshMinInterval,
	}, nil
}

// keySet returns the always-current cached key set, suitable for passing to
// jwt.WithKeySet. Reads come from the in-memory cache; no network call.
func (k *keyCache) keySet() jwk.Set {
	return k.set
}

// refreshIfStale forces a JWKS refetch, throttled to at most once per
// minRefresh window. It is called when signature verification fails so a key
// that rotated in ahead of the scheduled refresh is picked up promptly, while a
// storm of bad-signature tokens cannot be turned into a fetch amplifier.
// Returns true if a refresh actually ran.
func (k *keyCache) refreshIfStale(ctx context.Context) bool {
	k.mu.Lock()
	if time.Since(k.lastRefresh) < k.minRefresh {
		k.mu.Unlock()
		return false
	}
	k.lastRefresh = time.Now()
	k.mu.Unlock()

	// Ignore the error: on failure the previously cached keys remain in use, and
	// the caller will simply report an invalid signature.
	_, _ = k.cache.Refresh(ctx, k.url)
	return true
}
