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
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ColdCrabby/cloud-presets/internal/api"
	"github.com/ColdCrabby/cloud-presets/internal/auth"
	"github.com/ColdCrabby/cloud-presets/internal/catalog"
	ghapp "github.com/ColdCrabby/cloud-presets/internal/github"
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

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		if port := os.Getenv("PORT"); port != "" {
			addr = ":" + port
		} else {
			addr = ":8080"
		}
	}

	if _, err := newGitHubClient(); err != nil {
		if errors.Is(err, ghapp.ErrNotConfigured) {
			log.Print("github: no App credentials in the environment, vendor submissions are disabled")
		} else {
			log.Printf("github: client unavailable, vendor submissions are disabled: %v", err)
		}
	}

	holder := catalog.NewHolder()
	_, apiHandler := api.New(holder, newAuthMiddleware())

	handler := withFrontends(apiHandler)

	log.Printf("preset cloud API listening on %s (catalog not ready until first ingest)", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

// withFrontends mounts the API under /v1 and serves the two built Angular apps:
// the public app at / and the vendor-admin app at /vendor/. Missing build dirs
// are skipped so the API still serves on its own.
func withFrontends(apiHandler http.Handler) http.Handler {
	publicDir := envOr("PUBLIC_DIR", defaultPublicDir)
	vendorDir := envOr("VENDOR_DIR", defaultVendorDir)

	mux := http.NewServeMux()
	mux.Handle(api.BasePath+"/", apiHandler)

	if dirHasIndex(vendorDir) {
		// The public token is injected into index.html at serve time, so the
		// token is a runtime env var and changing it needs no rebuild.
		mux.Handle("/vendor/", spaHandler{
			root:         vendorDir,
			prefix:       "/vendor/",
			configScript: stytchConfigScript(os.Getenv("STYTCH_PUBLIC_TOKEN")),
		})
	} else {
		log.Printf("frontends: vendor build not found at %s, /vendor disabled", vendorDir)
	}

	if dirHasIndex(publicDir) {
		mux.Handle("/", spaHandler{root: publicDir, prefix: "/"})
	} else {
		log.Printf("frontends: public build not found at %s, / disabled", publicDir)
	}

	return mux
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
