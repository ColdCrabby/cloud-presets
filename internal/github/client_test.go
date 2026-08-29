package github

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gh "github.com/google/go-github/v88/github"
)

// One key for the whole suite: generating RSA keys is the slowest thing these
// tests do, and nothing here depends on keys differing. It is generated in
// process and never written outside the test's temp dir — no real App key is
// involved in any test.
var (
	testKeyOnce sync.Once
	testKeyPEM  []byte
)

func keyPEM(t *testing.T) []byte {
	t.Helper()
	testKeyOnce.Do(func() {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			panic(err)
		}
		testKeyPEM = pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(key),
		})
	})
	return testKeyPEM
}

// fakeGitHub is a minimal stand-in for the two endpoints this package touches:
// the installation token endpoint and the installation repository listing.
type fakeGitHub struct {
	server *httptest.Server

	tokenRequests atomic.Int32

	// tokenStatus, when non-zero, is returned instead of a token.
	tokenStatus int

	// tokenTTL controls how long minted tokens are valid. The transport
	// refreshes a minute before expiry, so a short TTL forces a refresh.
	tokenTTL time.Duration

	permissions map[string]string
	repos       []string

	// reposHandler, when set, replaces the default repository listing.
	reposHandler http.HandlerFunc
}

func newFakeGitHub(t *testing.T, f *fakeGitHub) *fakeGitHub {
	t.Helper()

	if f.tokenTTL == 0 {
		f.tokenTTL = time.Hour
	}
	if f.permissions == nil {
		f.permissions = map[string]string{
			"contents":      "write",
			"pull_requests": "write",
			"metadata":      "read",
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/app/installations/", func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/access_tokens") {
			http.NotFound(w, r)
			return
		}
		f.tokenRequests.Add(1)
		if f.tokenStatus != 0 {
			w.WriteHeader(f.tokenStatus)
			fmt.Fprint(w, `{"message":"Bad credentials"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"token":       fmt.Sprintf("ghs_test_%d", f.tokenRequests.Load()),
			"expires_at":  time.Now().Add(f.tokenTTL).UTC().Format(time.RFC3339),
			"permissions": f.permissions,
		})
	})

	mux.HandleFunc("/installation/repositories", func(w http.ResponseWriter, r *http.Request) {
		if f.reposHandler != nil {
			f.reposHandler(w, r)
			return
		}
		repos := make([]map[string]any, 0, len(f.repos))
		for _, full := range f.repos {
			owner, name, _ := strings.Cut(full, "/")
			repos = append(repos, map[string]any{
				"name":      name,
				"full_name": full,
				"owner":     map[string]any{"login": owner},
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"total_count":  len(repos),
			"repositories": repos,
		})
	})

	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeGitHub) client(t *testing.T, repository string) *Client {
	t.Helper()
	c, err := New(Config{
		AppID:          123,
		InstallationID: 456,
		PrivateKeyPEM:  keyPEM(t),
		BaseURL:        f.server.URL,
		Repository:     repository,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return c
}

func TestNewRejectsBadConfig(t *testing.T) {
	valid := keyPEM(t)

	cases := map[string]Config{
		"missing app id":          {InstallationID: 1, PrivateKeyPEM: valid},
		"missing installation id": {AppID: 1, PrivateKeyPEM: valid},
		"missing key":             {AppID: 1, InstallationID: 1},
		"key is not pem":          {AppID: 1, InstallationID: 1, PrivateKeyPEM: []byte("not a pem")},
		"key is wrong pem type": {AppID: 1, InstallationID: 1, PrivateKeyPEM: pem.EncodeToMemory(
			&pem.Block{Type: "CERTIFICATE", Bytes: []byte("nonsense")})},
	}

	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := New(cfg); !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("New() error = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

func TestConfigStringRedactsPrivateKey(t *testing.T) {
	cfg := Config{AppID: 1, InstallationID: 2, PrivateKeyPEM: keyPEM(t)}

	got := cfg.String()
	if strings.Contains(got, "PRIVATE KEY") || strings.Contains(got, string(keyPEM(t)[:40])) {
		t.Fatalf("Config.String() leaked key material: %s", got)
	}
	if !strings.Contains(got, "redacted") {
		t.Fatalf("Config.String() = %s, want it to mark the key redacted", got)
	}
}

func TestResolvePrivateKey(t *testing.T) {
	key := keyPEM(t)

	t.Run("inline pem", func(t *testing.T) {
		got, err := ResolvePrivateKey(string(key), "")
		if err != nil {
			t.Fatalf("ResolvePrivateKey() error = %v", err)
		}
		if string(got) != strings.TrimSpace(string(key)) {
			t.Fatalf("ResolvePrivateKey() did not return the PEM unchanged")
		}
	})

	t.Run("inline base64", func(t *testing.T) {
		got, err := ResolvePrivateKey(base64.StdEncoding.EncodeToString(key), "")
		if err != nil {
			t.Fatalf("ResolvePrivateKey() error = %v", err)
		}
		if err := checkPrivateKey(got); err != nil {
			t.Fatalf("decoded key did not validate: %v", err)
		}
	})

	t.Run("file", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "app.pem")
		if err := os.WriteFile(path, key, 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := ResolvePrivateKey("", path)
		if err != nil {
			t.Fatalf("ResolvePrivateKey() error = %v", err)
		}
		if err := checkPrivateKey(got); err != nil {
			t.Fatalf("file key did not validate: %v", err)
		}
	})

	t.Run("both", func(t *testing.T) {
		if _, err := ResolvePrivateKey(string(key), "/tmp/app.pem"); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("ResolvePrivateKey() error = %v, want ErrInvalidConfig", err)
		}
	})

	t.Run("neither", func(t *testing.T) {
		if _, err := ResolvePrivateKey("", ""); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("ResolvePrivateKey() error = %v, want ErrInvalidConfig", err)
		}
	})

	t.Run("missing file", func(t *testing.T) {
		if _, err := ResolvePrivateKey("", filepath.Join(t.TempDir(), "absent.pem")); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("ResolvePrivateKey() error = %v, want ErrInvalidConfig", err)
		}
	})
}

func TestParseID(t *testing.T) {
	if got, err := ParseID("GITHUB_APP_ID", " 42 "); err != nil || got != 42 {
		t.Fatalf("ParseID() = %d, %v, want 42, nil", got, err)
	}
	for _, raw := range []string{"", "abc"} {
		if _, err := ParseID("GITHUB_APP_ID", raw); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("ParseID(%q) error = %v, want ErrInvalidConfig", raw, err)
		}
	}
	_, err := ParseID("GITHUB_APP_ID", "abc")
	if !strings.Contains(fmt.Sprint(err), "GITHUB_APP_ID") {
		t.Fatalf("ParseID() error = %q, want it to name the variable", err)
	}
}

func TestTokenIsMintedOnceAndReused(t *testing.T) {
	fake := newFakeGitHub(t, &fakeGitHub{repos: []string{"ColdCrabby/presets"}})
	client := fake.client(t, "")

	for range 3 {
		if _, err := client.Token(t.Context()); err != nil {
			t.Fatalf("Token() error = %v", err)
		}
	}
	if _, _, err := client.rest.Apps.ListRepos(t.Context(), nil); err != nil {
		t.Fatalf("ListRepos() error = %v", err)
	}

	if got := fake.tokenRequests.Load(); got != 1 {
		t.Fatalf("token requests = %d, want 1 — a valid token must be reused", got)
	}
}

func TestTokenRefreshesBeforeExpiry(t *testing.T) {
	// The transport refreshes a minute ahead of expiry, so a token that expires
	// in well under a minute is always due for renewal.
	fake := newFakeGitHub(t, &fakeGitHub{tokenTTL: 10 * time.Second})
	client := fake.client(t, "")

	first, err := client.Token(t.Context())
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	second, err := client.Token(t.Context())
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}

	if first == second {
		t.Fatal("Token() returned the same near-expiry token twice, want a refresh")
	}
	if got := fake.tokenRequests.Load(); got != 2 {
		t.Fatalf("token requests = %d, want 2", got)
	}
}

func TestTokenRejectionIsAPermissionError(t *testing.T) {
	fake := newFakeGitHub(t, &fakeGitHub{tokenStatus: http.StatusUnauthorized})
	client := fake.client(t, "")

	_, err := client.Token(t.Context())

	var permErr *PermissionError
	if !errors.As(err, &permErr) {
		t.Fatalf("Token() error = %v (%T), want *PermissionError", err, err)
	}
	if !strings.Contains(permErr.Error(), "rotated") {
		t.Fatalf("PermissionError = %q, want it to hint at key rotation", permErr)
	}
}

func TestTokenRejectionDuringRESTCallIsAPermissionError(t *testing.T) {
	// The transport refreshes inside RoundTrip, so a revoked key fails during
	// ordinary REST traffic rather than at a call to Token. That path must
	// report the same error, not an opaque transport failure.
	fake := newFakeGitHub(t, &fakeGitHub{tokenStatus: http.StatusUnauthorized})
	client := fake.client(t, "")

	_, resp, err := client.REST().Apps.ListRepos(t.Context(), nil)

	var permErr *PermissionError
	if !errors.As(err, &permErr) {
		t.Fatalf("ListRepos() error = %v (%T), want *PermissionError", err, err)
	}
	if err := Classify("list repositories", resp, err); !errors.As(err, &permErr) {
		t.Fatalf("Classify() error = %v (%T), want the *PermissionError preserved", err, err)
	}
}

func TestVerifyInstallationAcceptsTheIntendedGrant(t *testing.T) {
	fake := newFakeGitHub(t, &fakeGitHub{repos: []string{"ColdCrabby/presets", "ColdCrabby/other"}})
	client := fake.client(t, "ColdCrabby/presets")

	if err := client.VerifyInstallation(t.Context()); err != nil {
		t.Fatalf("VerifyInstallation() error = %v, want nil", err)
	}
}

func TestVerifyInstallationRejectsMissingPermissions(t *testing.T) {
	fake := newFakeGitHub(t, &fakeGitHub{
		permissions: map[string]string{"contents": "read", "metadata": "read"},
		repos:       []string{"ColdCrabby/presets"},
	})
	client := fake.client(t, "ColdCrabby/presets")

	err := client.VerifyInstallation(t.Context())

	var permErr *PermissionError
	if !errors.As(err, &permErr) {
		t.Fatalf("VerifyInstallation() error = %v (%T), want *PermissionError", err, err)
	}
	msg := permErr.Error()
	if !strings.Contains(msg, "contents:write") || !strings.Contains(msg, "pull_requests:write") {
		t.Fatalf("VerifyInstallation() error = %q, want both shortfalls named", msg)
	}
}

func TestVerifyInstallationRejectsAdministrationGrant(t *testing.T) {
	// Administration would let the bot rewrite branch protection on main, which
	// is precisely the control that keeps merges human.
	fake := newFakeGitHub(t, &fakeGitHub{
		permissions: map[string]string{
			"contents":       "write",
			"pull_requests":  "write",
			"metadata":       "read",
			"administration": "write",
		},
		repos: []string{"ColdCrabby/presets"},
	})
	client := fake.client(t, "ColdCrabby/presets")

	err := client.VerifyInstallation(t.Context())

	var permErr *PermissionError
	if !errors.As(err, &permErr) {
		t.Fatalf("VerifyInstallation() error = %v (%T), want *PermissionError", err, err)
	}
	if !strings.Contains(permErr.Error(), "administration:write") {
		t.Fatalf("VerifyInstallation() error = %q, want the excess grant named", permErr)
	}
}

func TestVerifyInstallationRejectsAnyUnneededGrant(t *testing.T) {
	// The check is deny-by-default rather than a blocklist. workflows would let
	// the bot rewrite the CI that gates its own pull requests, and checks would
	// let it satisfy a required status — neither is on any list of obviously
	// dangerous permissions, and both must still be reported.
	fake := newFakeGitHub(t, &fakeGitHub{
		permissions: map[string]string{
			"contents":      "write",
			"pull_requests": "write",
			"metadata":      "read",
			"workflows":     "write",
			"checks":        "write",
		},
		repos: []string{"ColdCrabby/presets"},
	})
	client := fake.client(t, "ColdCrabby/presets")

	err := client.VerifyInstallation(t.Context())

	var permErr *PermissionError
	if !errors.As(err, &permErr) {
		t.Fatalf("VerifyInstallation() error = %v (%T), want *PermissionError", err, err)
	}
	msg := permErr.Error()
	if !strings.Contains(msg, "workflows:write") || !strings.Contains(msg, "checks:write") {
		t.Fatalf("VerifyInstallation() error = %q, want both excess grants named", msg)
	}
}

func TestVerifyInstallationRejectsRepositoryOutOfScope(t *testing.T) {
	fake := newFakeGitHub(t, &fakeGitHub{repos: []string{"ColdCrabby/something-else"}})
	client := fake.client(t, "ColdCrabby/presets")

	err := client.VerifyInstallation(t.Context())

	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("VerifyInstallation() error = %v (%T), want *NotFoundError", err, err)
	}
	if !strings.Contains(notFound.Error(), "ColdCrabby/something-else") {
		t.Fatalf("VerifyInstallation() error = %q, want the reachable repositories listed", notFound)
	}
}

func TestVerifyInstallationSkipsScopeCheckWhenNoRepositoryConfigured(t *testing.T) {
	fake := newFakeGitHub(t, &fakeGitHub{
		reposHandler: func(w http.ResponseWriter, r *http.Request) {
			t.Error("repository listing must not be called when no repository is configured")
		},
	})
	client := fake.client(t, "")

	if err := client.VerifyInstallation(t.Context()); err != nil {
		t.Fatalf("VerifyInstallation() error = %v, want nil", err)
	}
}

func TestPrimaryRateLimitIsClassified(t *testing.T) {
	reset := time.Now().Add(20 * time.Minute).Truncate(time.Second)
	fake := newFakeGitHub(t, &fakeGitHub{
		reposHandler: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-RateLimit-Limit", "5000")
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Used", "5000")
			w.Header().Set("X-RateLimit-Resource", "core")
			w.Header().Set("X-RateLimit-Reset", fmt.Sprint(reset.Unix()))
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"API rate limit exceeded"}`)
		},
	})
	client := fake.client(t, "ColdCrabby/presets")

	err := client.VerifyInstallation(t.Context())

	var limited *RateLimitedError
	if !errors.As(err, &limited) {
		t.Fatalf("VerifyInstallation() error = %v (%T), want *RateLimitedError", err, err)
	}
	if limited.Kind != RateLimitPrimary {
		t.Fatalf("Kind = %q, want %q", limited.Kind, RateLimitPrimary)
	}
	if limited.Limit != 5000 || limited.Remaining != 0 || limited.Resource != "core" {
		t.Fatalf("counters = %+v, want limit 5000, remaining 0, resource core", limited)
	}
	if !limited.Reset.Equal(reset) {
		t.Fatalf("Reset = %v, want %v", limited.Reset, reset)
	}
	if got := limited.RetryAfter(time.Now()); got <= 0 || got > 20*time.Minute {
		t.Fatalf("RetryAfter() = %v, want a positive wait no longer than the reset window", got)
	}
}

func TestSecondaryRateLimitIsClassified(t *testing.T) {
	fake := newFakeGitHub(t, &fakeGitHub{
		reposHandler: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Retry-After", "45")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprint(w, `{"message":"You have exceeded a secondary rate limit",`+
				`"documentation_url":"https://docs.github.com/rest/overview/rate-limits-for-the-rest-api#secondary-rate-limits"}`)
		},
	})
	client := fake.client(t, "ColdCrabby/presets")

	err := client.VerifyInstallation(t.Context())

	var limited *RateLimitedError
	if !errors.As(err, &limited) {
		t.Fatalf("VerifyInstallation() error = %v (%T), want *RateLimitedError", err, err)
	}
	if limited.Kind != RateLimitSecondary {
		t.Fatalf("Kind = %q, want %q", limited.Kind, RateLimitSecondary)
	}
	if got := limited.RetryAfter(time.Now()); got != 45*time.Second {
		t.Fatalf("RetryAfter() = %v, want 45s from the Retry-After header", got)
	}
}

func TestClassify(t *testing.T) {
	t.Run("nil error", func(t *testing.T) {
		if err := Classify("do a thing", nil, nil); err != nil {
			t.Fatalf("Classify() = %v, want nil", err)
		}
	})

	t.Run("not found", func(t *testing.T) {
		resp := &gh.Response{Response: &http.Response{StatusCode: http.StatusNotFound}}
		err := Classify("ColdCrabby/presets", resp, &gh.ErrorResponse{Message: "Not Found"})

		var notFound *NotFoundError
		if !errors.As(err, &notFound) {
			t.Fatalf("Classify() = %v (%T), want *NotFoundError", err, err)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		resp := &gh.Response{Response: &http.Response{StatusCode: http.StatusForbidden}}
		err := Classify("push a branch", resp, &gh.ErrorResponse{Message: "Resource not accessible by integration"})

		var permErr *PermissionError
		if !errors.As(err, &permErr) {
			t.Fatalf("Classify() = %v (%T), want *PermissionError", err, err)
		}
		if !strings.Contains(permErr.Error(), "Resource not accessible by integration") {
			t.Fatalf("Classify() = %q, want GitHub's own message preserved", permErr)
		}
	})

	t.Run("exhausted quota not typed by go-github", func(t *testing.T) {
		resp := &gh.Response{
			Response: &http.Response{StatusCode: http.StatusForbidden},
			Rate:     gh.Rate{Limit: 5000, Remaining: 0, Used: 5000, Resource: "core"},
		}
		err := Classify("open a pull request", resp, &gh.ErrorResponse{Message: "quota gone"})

		var limited *RateLimitedError
		if !errors.As(err, &limited) {
			t.Fatalf("Classify() = %v (%T), want *RateLimitedError", err, err)
		}
		if limited.Kind != RateLimitPrimary {
			t.Fatalf("Kind = %q, want %q", limited.Kind, RateLimitPrimary)
		}
	})

	t.Run("other errors keep the operation in the message", func(t *testing.T) {
		err := Classify("open a pull request", nil, errors.New("connection reset"))
		if !strings.Contains(err.Error(), "open a pull request") ||
			!strings.Contains(err.Error(), "connection reset") {
			t.Fatalf("Classify() = %q, want the operation and cause", err)
		}
	})
}

func TestRateLimitedErrorRetryAfterIsNeverNegative(t *testing.T) {
	limited := &RateLimitedError{Kind: RateLimitPrimary, Reset: time.Now().Add(-time.Hour)}
	if got := limited.RetryAfter(time.Now()); got != 0 {
		t.Fatalf("RetryAfter() = %v, want 0 for a reset already past", got)
	}
}
