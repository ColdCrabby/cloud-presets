package github

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/bradleyfalzon/ghinstallation/v2"
	gh "github.com/google/go-github/v88/github"
)

// Client is the bot's authenticated GitHub client. It authenticates as an
// installation of the App: the private key signs a short-lived App JWT, which
// is exchanged for an installation access token that lives about an hour.
//
// Refresh is not the caller's problem. The installation transport mints a token
// on the first request and replaces it shortly before expiry, so a long-running
// process never has to restart to stay authenticated and no code path has to
// remember when the last token was issued.
//
// A Client is safe for concurrent use: the transport serialises token minting
// and refresh. VerifyInstallation is the exception — it reads transport state
// that the transport does not lock, so run it at startup before the client is
// shared.
type Client struct {
	rest      *gh.Client
	transport *ghinstallation.Transport
	cfg       Config
}

// New builds a client for the configured installation. It validates the
// credentials but performs no network call, so a server can construct the
// client at startup without depending on GitHub being reachable at that moment.
// Use VerifyInstallation for an explicit preflight.
func New(cfg Config) (*Client, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	base := cfg.Transport
	if base == nil {
		base = http.DefaultTransport
	}

	transport, err := ghinstallation.New(base, cfg.AppID, cfg.InstallationID, cfg.PrivateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("%w: building installation transport: %w", ErrInvalidConfig, err)
	}

	opts := []gh.ClientOptionsFunc{gh.WithHTTPClient(&http.Client{
		Transport: tokenErrorTransport{base: transport},
	})}
	if cfg.BaseURL != "" {
		// The token endpoint is called by the transport itself, so the base URL
		// has to be set in both places or the client would talk to one host and
		// authenticate against another.
		apiRoot := strings.TrimRight(cfg.BaseURL, "/")
		transport.BaseURL = apiRoot
		restRoot := apiRoot + "/"
		opts = append(opts, gh.WithURLs(&restRoot, nil))
	}

	rest, err := gh.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("%w: building REST client: %w", ErrInvalidConfig, err)
	}

	return &Client{rest: rest, transport: transport, cfg: cfg}, nil
}

// REST exposes the underlying go-github client for the pull-request work that
// builds on this package. Errors from it should be passed through Classify so
// rate limiting is reported consistently.
func (c *Client) REST() *gh.Client { return c.rest }

// AppID returns the configured App ID.
func (c *Client) AppID() int64 { return c.cfg.AppID }

// InstallationID returns the configured installation ID.
func (c *Client) InstallationID() int64 { return c.cfg.InstallationID }

// Token returns a valid installation access token, minting or refreshing it if
// needed. Most callers should use REST instead; this exists for the few places
// that need to hand a token to something that is not the REST client, such as a
// Git transport.
func (c *Client) Token(ctx context.Context) (string, error) {
	token, err := c.transport.Token(ctx)
	if err != nil {
		return "", classifyTokenError(err)
	}
	return token, nil
}

// tokenErrorTransport turns a failed token mint during ordinary REST traffic
// into one of this package's errors.
//
// The installation transport refreshes the token inside RoundTrip, so a revoked
// or rotated-away key fails there rather than at a call to Token — and without
// this wrapper it would reach the caller as an opaque transport failure with
// its response body still open, once per request. Errors that are not token
// mints (a dropped connection, a cancelled context) pass through untouched.
type tokenErrorTransport struct {
	base *ghinstallation.Transport
}

func (t tokenErrorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)

	var httpErr *ghinstallation.HTTPError
	if err != nil && errors.As(err, &httpErr) {
		return nil, classifyTokenError(err)
	}
	return resp, err
}

// classifyTokenError maps a token-minting failure onto this package's errors.
// The transport reports HTTP failures as its own type rather than a go-github
// one, so a 401 from a revoked key would otherwise arrive as an opaque string.
//
// It also closes the response body. The transport deliberately leaves it open
// on the error path so the caller can read it, and every caller in this package
// funnels through here — a rejected key is retried on every request, so leaving
// it open would leak a connection per attempt rather than once.
func classifyTokenError(err error) error {
	var httpErr *ghinstallation.HTTPError
	if errors.As(err, &httpErr) && httpErr.Response != nil {
		if httpErr.Response.Body != nil {
			httpErr.Response.Body.Close()
		}
		switch httpErr.Response.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return &PermissionError{
				Op:      "mint an installation token",
				Message: "the App private key was rejected — it may have been revoked or rotated",
				err:     err,
			}
		case http.StatusNotFound:
			return &NotFoundError{Resource: "the App installation", err: err}
		}
	}
	return fmt.Errorf("github: minting installation token: %w", err)
}
