package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwa"
	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
)

const (
	testProjectID = "project-test-abc123"
	testIssuer    = "stytch.com/project-test-abc123"
)

// signingKey is an RSA keypair plus the public JWK (with kid) that a JWKS server
// would publish for it.
type signingKey struct {
	kid     string
	private jwk.Key
	public  jwk.Key
}

func newSigningKey(t *testing.T, kid string) signingKey {
	t.Helper()
	raw, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	priv, err := jwk.FromRaw(raw)
	if err != nil {
		t.Fatalf("jwk from private: %v", err)
	}
	_ = priv.Set(jwk.KeyIDKey, kid)
	_ = priv.Set(jwk.AlgorithmKey, jwa.RS256)

	pub, err := jwk.FromRaw(raw.Public())
	if err != nil {
		t.Fatalf("jwk from public: %v", err)
	}
	_ = pub.Set(jwk.KeyIDKey, kid)
	_ = pub.Set(jwk.AlgorithmKey, jwa.RS256)

	return signingKey{kid: kid, private: priv, public: pub}
}

// jwksServer serves a JWKS composed of the given keys and counts fetches so a
// test can assert offline behaviour and rotation refreshes.
type jwksServer struct {
	*httptest.Server
	fetches atomic.Int64
	keys    atomic.Value // jwk.Set
}

func newJWKSServer(t *testing.T, keys ...signingKey) *jwksServer {
	t.Helper()
	js := &jwksServer{}
	js.setKeys(keys...)
	js.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		js.fetches.Add(1)
		set := js.keys.Load().(jwk.Set)
		buf, err := json.Marshal(set)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(buf)
	}))
	t.Cleanup(js.Close)
	return js
}

func (js *jwksServer) setKeys(keys ...signingKey) {
	set := jwk.NewSet()
	for _, k := range keys {
		_ = set.AddKey(k.public)
	}
	js.keys.Store(set)
}

// mintOptions describes the token to build; zero values fall back to a valid
// token signed by key.
type mintOptions struct {
	issuer   string
	audience string
	subject  string
	orgID    string
	orgSlug  string
	roles    []string
	iat      time.Time
	exp      time.Time
	// omitIat/omitExp drop the respective registered claim.
	omitIat bool
	omitExp bool
}

func mintToken(t *testing.T, key signingKey, o mintOptions) string {
	t.Helper()
	now := time.Now()
	if o.issuer == "" {
		o.issuer = testIssuer
	}
	if o.audience == "" {
		o.audience = testProjectID
	}
	if o.subject == "" {
		o.subject = "member-123"
	}
	if o.iat.IsZero() {
		o.iat = now
	}
	if o.exp.IsZero() {
		o.exp = now.Add(5 * time.Minute)
	}

	b := jwt.NewBuilder().
		Issuer(o.issuer).
		Audience([]string{o.audience}).
		Subject(o.subject)
	if !o.omitIat {
		b = b.IssuedAt(o.iat)
	}
	if !o.omitExp {
		b = b.Expiration(o.exp)
	}
	if o.orgID != "" {
		b = b.Claim("organization_id", o.orgID)
	}
	if o.orgSlug != "" {
		b = b.Claim("organization_slug", o.orgSlug)
	}
	if o.roles != nil {
		b = b.Claim("roles", o.roles)
	}

	tok, err := b.Build()
	if err != nil {
		t.Fatalf("build token: %v", err)
	}
	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256, key.private))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return string(signed)
}

func newTestVerifier(t *testing.T, jwksURL string, keys ...signingKey) *Verifier {
	t.Helper()
	cfg := Config{
		ProjectID:          testProjectID,
		JWKSURL:            jwksURL,
		RefreshInterval:    time.Hour,
		RefreshMinInterval: time.Millisecond, // allow rotation refresh in tests
	}
	v, err := New(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new verifier: %v", err)
	}
	return v
}

func TestVerifyValidToken(t *testing.T) {
	key := newSigningKey(t, "key-1")
	srv := newJWKSServer(t, key)
	v := newTestVerifier(t, srv.URL)

	raw := mintToken(t, key, mintOptions{
		orgID:   "organization-abc",
		orgSlug: "acme-filaments",
		roles:   []string{"stytch_member", "editor"},
	})

	fetchesBefore := srv.fetches.Load()
	claims, err := v.Verify(context.Background(), raw)
	if err != nil {
		t.Fatalf("expected valid token, got %v", err)
	}

	if claims.OrganizationID != "organization-abc" {
		t.Errorf("organization_id = %q", claims.OrganizationID)
	}
	if claims.OrganizationSlug != "acme-filaments" {
		t.Errorf("organization_slug = %q", claims.OrganizationSlug)
	}
	if !claims.HasRole("editor") {
		t.Errorf("roles = %v, expected editor", claims.Roles)
	}
	if claims.Subject != "member-123" {
		t.Errorf("subject = %q", claims.Subject)
	}
	// No JWKS fetch on the verify path — validation is offline.
	if got := srv.fetches.Load(); got != fetchesBefore {
		t.Errorf("verify made %d JWKS fetches, expected 0", got-fetchesBefore)
	}
}

func TestVerifyRejects(t *testing.T) {
	key := newSigningKey(t, "key-1")
	otherKey := newSigningKey(t, "attacker")
	srv := newJWKSServer(t, key)
	v := newTestVerifier(t, srv.URL)

	now := time.Now()

	cases := []struct {
		name    string
		raw     string
		wantErr error
	}{
		{
			name:    "empty",
			raw:     "",
			wantErr: ErrMissingToken,
		},
		{
			name:    "garbage",
			raw:     "not-a-jwt",
			wantErr: ErrMalformedToken,
		},
		{
			name:    "signed by unknown key",
			raw:     mintToken(t, otherKey, mintOptions{}),
			wantErr: ErrInvalidSignature,
		},
		{
			name:    "wrong issuer",
			raw:     mintToken(t, key, mintOptions{issuer: "stytch.com/someone-else"}),
			wantErr: ErrInvalidClaims,
		},
		{
			name:    "wrong audience",
			raw:     mintToken(t, key, mintOptions{audience: "different-project"}),
			wantErr: ErrInvalidClaims,
		},
		{
			name:    "expired",
			raw:     mintToken(t, key, mintOptions{iat: now.Add(-10 * time.Minute), exp: now.Add(-5 * time.Minute)}),
			wantErr: ErrExpired,
		},
		{
			name:    "too old for max age",
			raw:     mintToken(t, key, mintOptions{iat: now.Add(-2 * time.Hour), exp: now.Add(1 * time.Hour)}),
			wantErr: ErrExpired,
		},
		{
			name:    "missing iat",
			raw:     mintToken(t, key, mintOptions{omitIat: true}),
			wantErr: ErrInvalidClaims,
		},
		{
			name:    "missing exp",
			raw:     mintToken(t, key, mintOptions{omitExp: true}),
			wantErr: ErrInvalidClaims,
		},
		{
			name:    "issued in the future",
			raw:     mintToken(t, key, mintOptions{iat: now.Add(10 * time.Minute), exp: now.Add(15 * time.Minute)}),
			wantErr: ErrInvalidClaims,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := v.Verify(context.Background(), tc.raw)
			if err == nil {
				t.Fatalf("expected error %v, got nil", tc.wantErr)
			}
			if !isErr(err, tc.wantErr) {
				t.Fatalf("expected %v, got %v", tc.wantErr, err)
			}
		})
	}
}

// TestVerifyHandlesRotation checks that a token signed by a key that rotated in
// after startup is accepted once the throttled refresh picks it up — the JWKS
// overlap window in miniature.
func TestVerifyHandlesRotation(t *testing.T) {
	oldKey := newSigningKey(t, "key-old")
	srv := newJWKSServer(t, oldKey)
	v := newTestVerifier(t, srv.URL)

	// A token from the still-published old key keeps working.
	if _, err := v.Verify(context.Background(), mintToken(t, oldKey, mintOptions{})); err != nil {
		t.Fatalf("old key token rejected: %v", err)
	}

	// New key rotates in; JWKS now publishes both (overlap).
	newKey := newSigningKey(t, "key-new")
	srv.setKeys(oldKey, newKey)

	// A token from the new key initially fails against the cached set, triggers a
	// refresh, and then verifies.
	claims, err := v.Verify(context.Background(), mintToken(t, newKey, mintOptions{orgID: "org-x"}))
	if err != nil {
		t.Fatalf("new key token rejected after rotation: %v", err)
	}
	if claims.OrganizationID != "org-x" {
		t.Errorf("organization_id = %q", claims.OrganizationID)
	}

	// Old key token still verifies during the overlap.
	if _, err := v.Verify(context.Background(), mintToken(t, oldKey, mintOptions{})); err != nil {
		t.Fatalf("old key token rejected during overlap: %v", err)
	}
}

func TestClockSkewToleratesRecentExpiry(t *testing.T) {
	key := newSigningKey(t, "key-1")
	srv := newJWKSServer(t, key)
	v := newTestVerifier(t, srv.URL)

	now := time.Now()
	// Expired 10s ago — within the 30s default skew, so still accepted.
	raw := mintToken(t, key, mintOptions{iat: now.Add(-5 * time.Minute), exp: now.Add(-10 * time.Second)})
	if _, err := v.Verify(context.Background(), raw); err != nil {
		t.Fatalf("token within skew rejected: %v", err)
	}
}

func TestMiddleware(t *testing.T) {
	key := newSigningKey(t, "key-1")
	srv := newJWKSServer(t, key)
	v := newTestVerifier(t, srv.URL)
	mw := NewMiddleware(v)

	var gotClaims Claims
	var handlerCalled bool
	protected := mw.RequireAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		c, ok := ClaimsFromContext(r.Context())
		if !ok {
			t.Error("claims not in context")
		}
		gotClaims = c
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("valid token passes through", func(t *testing.T) {
		handlerCalled = false
		raw := mintToken(t, key, mintOptions{orgID: "org-1", roles: []string{"admin"}})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/vendor/presets", nil)
		req.Header.Set("Authorization", "Bearer "+raw)
		protected.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if !handlerCalled {
			t.Fatal("handler not called")
		}
		if gotClaims.OrganizationID != "org-1" {
			t.Errorf("claims.OrganizationID = %q", gotClaims.OrganizationID)
		}
	})

	t.Run("missing token yields 401", func(t *testing.T) {
		handlerCalled = false
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/vendor/presets", nil)
		protected.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", rec.Code)
		}
		if handlerCalled {
			t.Fatal("handler should not run on 401")
		}
		if !strings.Contains(rec.Header().Get("WWW-Authenticate"), "Bearer") {
			t.Errorf("missing WWW-Authenticate header: %q", rec.Header().Get("WWW-Authenticate"))
		}
	})

	t.Run("expired token yields 401 not 403", func(t *testing.T) {
		now := time.Now()
		raw := mintToken(t, key, mintOptions{iat: now.Add(-10 * time.Minute), exp: now.Add(-5 * time.Minute)})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/vendor/presets", nil)
		req.Header.Set("Authorization", "Bearer "+raw)
		protected.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401 (distinct from the 403 used for ownership)", rec.Code)
		}
	})
}

func TestConfigDefaults(t *testing.T) {
	cfg, err := Config{ProjectID: "project-live-xyz"}.withDefaults()
	if err != nil {
		t.Fatalf("withDefaults: %v", err)
	}
	if cfg.Issuer != "stytch.com/project-live-xyz" {
		t.Errorf("issuer = %q", cfg.Issuer)
	}
	if cfg.Audience != "project-live-xyz" {
		t.Errorf("audience = %q", cfg.Audience)
	}
	if !strings.Contains(cfg.JWKSURL, "project-live-xyz") {
		t.Errorf("jwks url = %q", cfg.JWKSURL)
	}
	if cfg.MaxTokenAge <= 0 || cfg.ClockSkew <= 0 {
		t.Errorf("durations not defaulted: %+v", cfg)
	}

	if _, err := (Config{}).withDefaults(); err == nil {
		t.Error("expected error for missing ProjectID")
	}
}

// isErr reports whether err wraps target.
func isErr(err, target error) bool {
	for err != nil {
		if err == target {
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
