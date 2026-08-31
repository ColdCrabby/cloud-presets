// Command server runs the Cold Crabby preset cloud HTTP API.
//
// It serves the Huma v2 API on the standard library http.ServeMux via the
// humago adapter — no router dependency. The catalog starts empty, so the
// server reports not ready until ingest (a later foundation-wave issue)
// publishes the first revision.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ColdCrabby/cloud-presets/internal/api"
	"github.com/ColdCrabby/cloud-presets/internal/auth"
	"github.com/ColdCrabby/cloud-presets/internal/catalog"
	ghapp "github.com/ColdCrabby/cloud-presets/internal/github"
	"github.com/ColdCrabby/cloud-presets/internal/preset"
	"github.com/ColdCrabby/cloud-presets/internal/submit"
	"github.com/ColdCrabby/cloud-presets/internal/upload"
)

// defaultPresetsRepository is the repository the bot proposes changes to.
const defaultPresetsRepository = "ColdCrabby/presets"

// Default locations of the built Angular apps, overridable via env.
const (
	defaultPublicDir = "apps/public/dist/public/browser"
	defaultVendorDir = "apps/vendor-admin/dist/vendor-admin/browser"
)

// installationCheckTimeout bounds the startup preflight so an unreachable or
// slow GitHub cannot hold the server off its listener.
const installationCheckTimeout = 15 * time.Second

// newUploadOption builds the api.WithUploads option: a pinned preset validator,
// an in-memory draft store, and — when the bot is configured — a GitHub-backed
// submitter that opens pull requests. When the validator cannot be built the
// upload endpoints are disabled rather than serving unvalidated uploads; when
// the bot is absent, claiming still resolves and validates but does not open a
// pull request.
func newUploadOption(ghClient *ghapp.Client) api.Option {
	validator, err := preset.New()
	if err != nil {
		log.Printf("uploads: preset validator unavailable, manual upload is disabled: %v", err)
		return api.WithUploads(nil, nil, nil)
	}

	var submitter submit.Submitter
	if ghClient != nil {
		s, err := ghapp.NewSubmitter(ghClient)
		if err != nil {
			log.Printf("uploads: submitter unavailable, PRs will not be opened: %v", err)
		} else {
			submitter = s
		}
	}

	store := upload.NewStore(upload.DefaultTTL)
	log.Printf("uploads: manual upload enabled (drafts expire after %s, PRs %s)",
		upload.DefaultTTL, prState(submitter != nil))
	return api.WithUploads(validator, store, submitter)
}

func prState(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		if port := os.Getenv("PORT"); port != "" {
			addr = ":" + port
		} else {
			addr = ":8080"
		}
	}

	ghClient, err := newGitHubClient()
	if err != nil {
		if errors.Is(err, ghapp.ErrNotConfigured) {
			log.Print("github: no App credentials in the environment, vendor submissions are disabled")
		} else {
			log.Printf("github: client unavailable, vendor submissions are disabled: %v", err)
		}
	}

	holder := catalog.NewHolder()
	// No ingest pipeline exists yet (a later foundation-wave issue), so seed a
	// small sample catalog by default so GET /v1/presets serves real, rankable
	// data. Set SEED_SAMPLE_CATALOG=0 to start empty (not ready) instead.
	if seedSampleCatalog() {
		c := catalog.Sample()
		holder.Swap(c)
		log.Printf("catalog: seeded sample revision %q (%d presets) pending ingest", c.Revision, len(c.Records))
	}
	_, apiHandler := api.New(holder, newAuthMiddleware(), newUploadOption(ghClient))

	origins := corsAllowedOrigins()
	log.Printf("cors: allowing cross-origin API access from %v", origins)
	handler := withFrontends(api.CORS(origins, apiHandler))

	log.Printf("preset cloud API listening on %s (ready=%t)", addr, holder.Ready())
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

// seedSampleCatalog reports whether to load the built-in sample catalog at
// startup. It defaults to true and is disabled by setting SEED_SAMPLE_CATALOG
// to "0" or "false".
func seedSampleCatalog() bool {
	switch strings.ToLower(os.Getenv("SEED_SAMPLE_CATALOG")) {
	case "0", "false", "no", "off":
		return false
	default:
		return true
	}
}

// withFrontends mounts the API under /v1 and serves the two Angular apps at one
// origin: the public app at / and the vendor-admin app at /vendor/, exactly as
// production does. This single-origin shape is also what dev uses — set the
// *_DEV_URL env vars and this same server reverse-proxies each surface to its
// live Vite dev server (HMR included) instead of serving a built dist, so local
// dev mirrors the deployment down to the URL layout. Missing build dirs are
// skipped so the API still serves on its own.
//
//	PUBLIC_DEV_URL   proxy /        to the public app's dev server
//	VENDOR_DEV_URL   proxy /vendor/ to the vendor app's dev server
//	SAMPLE_API_URL   proxy /v1/     to the sample API (dev sample data)
func withFrontends(apiHandler http.Handler) http.Handler {
	publicDir := envOr("PUBLIC_DIR", defaultPublicDir)
	vendorDir := envOr("VENDOR_DIR", defaultVendorDir)
	publicDev := os.Getenv("PUBLIC_DEV_URL")
	vendorDev := os.Getenv("VENDOR_DEV_URL")
	sampleAPI := os.Getenv("SAMPLE_API_URL")

	mux := http.NewServeMux()

	// API: an in-process handler in production; in dev, optionally the sample API
	// so the catalog has sample data before ingest is wired up.
	if sampleAPI != "" {
		log.Printf("frontends(dev): proxying %s/ to sample API %s", api.BasePath, sampleAPI)
		mux.Handle(api.BasePath+"/", devProxy(sampleAPI))
	} else {
		mux.Handle(api.BasePath+"/", apiHandler)
	}

	switch {
	case vendorDev != "":
		log.Printf("frontends(dev): proxying /vendor/ to %s", vendorDev)
		mux.Handle("/vendor/", devProxy(vendorDev))
	case dirHasIndex(vendorDir):
		// The public token is injected into index.html at serve time, so the
		// token is a runtime env var and changing it needs no rebuild.
		mux.Handle("/vendor/", spaHandler{
			root:         vendorDir,
			prefix:       "/vendor/",
			configScript: stytchConfigScript(os.Getenv("STYTCH_PUBLIC_TOKEN")),
		})
	default:
		log.Printf("frontends: vendor build not found at %s, /vendor disabled", vendorDir)
	}

	switch {
	case publicDev != "":
		log.Printf("frontends(dev): proxying / to %s", publicDev)
		mux.Handle("/", devProxy(publicDev))
	case dirHasIndex(publicDir):
		mux.Handle("/", spaHandler{root: publicDir, prefix: "/"})
	default:
		log.Printf("frontends: public build not found at %s, / disabled", publicDir)
	}

	return mux
}

// devProxy reverse-proxies to a local dev server (a Vite dev server or the
// sample API). It rewrites the Host header to the target so Vite accepts the request,
// and httputil handles the WebSocket upgrade that HMR rides on.
func devProxy(target string) http.Handler {
	u, err := url.Parse(target)
	if err != nil {
		log.Fatalf("frontends(dev): invalid proxy target %q: %v", target, err)
	}
	proxy := httputil.NewSingleHostReverseProxy(u)
	director := proxy.Director
	proxy.Director = func(r *http.Request) {
		director(r)
		r.Host = u.Host
	}
	return proxy
}

// stytchConfigScript renders the runtime config script injected into the vendor
// app's index.html. Empty token yields an empty string (no injection).
func stytchConfigScript(token string) string {
	if token == "" {
		return ""
	}
	// json.Marshal escapes the value, so it is safe to embed in the page.
	b, _ := json.Marshal(map[string]string{"stytchPublicToken": token})
	return "<script>window.__APP_CONFIG__=" + string(b) + "</script>"
}

// newAuthMiddleware builds the Stytch session-JWT middleware from the
// environment. When STYTCH_PROJECT_ID is unset, auth is disabled and nil is
// returned so the API still serves unprotected routes.
func newAuthMiddleware() *auth.Middleware {
	if os.Getenv("STYTCH_PROJECT_ID") == "" {
		log.Print("auth: no STYTCH_PROJECT_ID in the environment, vendor sign-in is disabled")
		return nil
	}
	cfg, err := auth.LoadConfigFromEnv()
	if err != nil {
		log.Printf("auth: invalid configuration, vendor sign-in is disabled: %v", err)
		return nil
	}
	verifier, err := auth.New(context.Background(), cfg)
	if err != nil {
		log.Printf("auth: verifier unavailable, vendor sign-in is disabled: %v", err)
		return nil
	}
	log.Print("auth: Stytch session validation enabled")
	return auth.NewMiddleware(verifier)
}

// corsAllowedOrigins returns the cross-origin web apps allowed to call the API
// from a browser. It defaults to api.DefaultAllowedOrigins and can be overridden
// with a comma-separated CORS_ALLOWED_ORIGINS list (each an exact scheme+host
// origin, e.g. "https://slicer.maxsopp.de").
func corsAllowedOrigins() []string {
	raw := os.Getenv("CORS_ALLOWED_ORIGINS")
	if raw == "" {
		return api.DefaultAllowedOrigins
	}
	var origins []string
	for _, o := range strings.Split(raw, ",") {
		if o = strings.TrimSpace(o); o != "" {
			origins = append(origins, o)
		}
	}
	return origins
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func dirHasIndex(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "index.html"))
	return err == nil && !info.IsDir()
}

// spaHandler serves static files from root, falling back to index.html so the
// Angular client-side router can handle unknown paths. When configScript is
// set, it is injected into index.html before </head> for runtime config.
type spaHandler struct {
	root         string
	prefix       string
	configScript string
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rel := strings.TrimPrefix(r.URL.Path, h.prefix)
	// filepath.Join cleans the path, so ".." segments cannot escape root.
	file := filepath.Join(h.root, filepath.FromSlash(rel))
	if info, err := os.Stat(file); err == nil && !info.IsDir() {
		http.ServeFile(w, r, file)
		return
	}
	h.serveIndex(w, r)
}

// serveIndex writes index.html, injecting configScript before </head> when set.
func (h spaHandler) serveIndex(w http.ResponseWriter, r *http.Request) {
	index := filepath.Join(h.root, "index.html")
	if h.configScript == "" {
		http.ServeFile(w, r, index)
		return
	}
	body, err := os.ReadFile(index)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	html := strings.Replace(string(body), "</head>", h.configScript+"</head>", 1)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = io.WriteString(w, html)
}

// newGitHubClient builds the bot client from the environment, which the deploy
// platform populates from its secret store.
//
//	GITHUB_APP_ID               the numeric App ID
//	GITHUB_APP_INSTALLATION_ID  the installation on the presets repository
//	GITHUB_APP_PRIVATE_KEY      the App private key, PEM or base64 of PEM
//	GITHUB_APP_PRIVATE_KEY_FILE path to the key instead, for mounted secrets
//	GITHUB_PRESETS_REPOSITORY   owner/name to verify, default ColdCrabby/presets
//	GITHUB_API_BASE_URL         override the API root, for GitHub Enterprise
//
// Missing credentials are not fatal. Nothing served today needs the bot — the
// catalog and the public browse path must not be held offline by a credential
// only vendor submissions will use. It returns ErrNotConfigured so the caller
// can say so once and move on.
func newGitHubClient() (*ghapp.Client, error) {
	appID := os.Getenv("GITHUB_APP_ID")
	installationID := os.Getenv("GITHUB_APP_INSTALLATION_ID")
	inlineKey := os.Getenv("GITHUB_APP_PRIVATE_KEY")
	keyFile := os.Getenv("GITHUB_APP_PRIVATE_KEY_FILE")

	if appID == "" && installationID == "" && inlineKey == "" && keyFile == "" {
		return nil, ghapp.ErrNotConfigured
	}

	app, err := ghapp.ParseID("GITHUB_APP_ID", appID)
	if err != nil {
		return nil, err
	}
	installation, err := ghapp.ParseID("GITHUB_APP_INSTALLATION_ID", installationID)
	if err != nil {
		return nil, err
	}
	key, err := ghapp.ResolvePrivateKey(inlineKey, keyFile)
	if err != nil {
		return nil, err
	}

	repository := os.Getenv("GITHUB_PRESETS_REPOSITORY")
	if repository == "" {
		repository = defaultPresetsRepository
	}

	client, err := ghapp.New(ghapp.Config{
		AppID:          app,
		InstallationID: installation,
		PrivateKeyPEM:  key,
		BaseURL:        os.Getenv("GITHUB_API_BASE_URL"),
		Repository:     repository,
	})
	if err != nil {
		return nil, err
	}

	// The preflight is advisory: a wrong or over-broad installation is worth
	// shouting about at startup rather than discovering on a vendor's first
	// submission, but GitHub being briefly unreachable is not a reason to
	// refuse to serve the catalog.
	ctx, cancel := context.WithTimeout(context.Background(), installationCheckTimeout)
	defer cancel()
	if err := client.VerifyInstallation(ctx); err != nil {
		log.Printf("github: installation preflight failed, submissions will not work: %v", err)
	} else {
		log.Printf("github: authenticated as app %d on %s", app, repository)
	}

	return client, nil
}
