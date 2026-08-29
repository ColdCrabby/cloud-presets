// Command server runs the Cold Crabby preset cloud HTTP API.
//
// It serves the Huma v2 API on the standard library http.ServeMux via the
// humago adapter — no router dependency. The catalog starts empty, so the
// server reports not ready until ingest (a later foundation-wave issue)
// publishes the first revision.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/ColdCrabby/cloud-presets/internal/api"
	"github.com/ColdCrabby/cloud-presets/internal/catalog"
	ghapp "github.com/ColdCrabby/cloud-presets/internal/github"
)

// defaultPresetsRepository is the repository the bot proposes changes to.
const defaultPresetsRepository = "ColdCrabby/presets"

// installationCheckTimeout bounds the startup preflight so an unreachable or
// slow GitHub cannot hold the server off its listener.
const installationCheckTimeout = 15 * time.Second

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}

	if _, err := newGitHubClient(); err != nil {
		if errors.Is(err, ghapp.ErrNotConfigured) {
			log.Print("github: no App credentials in the environment, vendor submissions are disabled")
		} else {
			log.Printf("github: client unavailable, vendor submissions are disabled: %v", err)
		}
	}

	holder := catalog.NewHolder()
	_, handler := api.New(holder)

	log.Printf("preset cloud API listening on %s (catalog not ready until first ingest)", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
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
