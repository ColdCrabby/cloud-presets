package github

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// Config is everything needed to authenticate as an installation of the bot
// GitHub App. Unlike the Stytch verifier config, one field here is genuinely
// secret: PrivateKeyPEM. It is deliberately []byte rather than string so it is
// awkward to interpolate into a log line by accident, and String redacts it.
//
// Values come from the environment, populated at deploy time from the
// platform's secret store. Nothing here is read from the repository or baked
// into the image.
type Config struct {
	// AppID is the numeric GitHub App ID (not the client ID).
	AppID int64

	// InstallationID identifies this App's installation on the presets
	// repository's owner. Tokens are minted per installation, so this is what
	// scopes the bot's authority.
	InstallationID int64

	// PrivateKeyPEM is the App's private key in PKCS#1 or PKCS#8 PEM form. It
	// signs the short-lived App JWT used to request installation tokens.
	PrivateKeyPEM []byte

	// BaseURL overrides the GitHub REST API root. Empty means github.com.
	// Set it for GitHub Enterprise or to point tests at a local server.
	BaseURL string

	// Repository is the "owner/name" the installation is expected to cover,
	// checked by VerifyInstallation. Empty skips the repository scope check.
	Repository string

	// Transport is the underlying round tripper the installation transport
	// wraps. Nil means http.DefaultTransport. Mainly a test seam.
	Transport http.RoundTripper
}

// validate reports whether the config can produce a working client. It parses
// the key rather than trusting its shape, because a truncated or
// wrongly-escaped PEM is the single most common deployment mistake here and the
// failure it otherwise produces — a 401 on the first token request, minutes
// later — points nowhere useful.
func (c Config) validate() error {
	if c.AppID <= 0 {
		return fmt.Errorf("%w: AppID must be a positive integer", ErrInvalidConfig)
	}
	if c.InstallationID <= 0 {
		return fmt.Errorf("%w: InstallationID must be a positive integer", ErrInvalidConfig)
	}
	if len(c.PrivateKeyPEM) == 0 {
		return fmt.Errorf("%w: PrivateKeyPEM is required", ErrInvalidConfig)
	}
	if err := checkPrivateKey(c.PrivateKeyPEM); err != nil {
		return err
	}
	return nil
}

// String renders the config for logs with the private key redacted. Only its
// presence and length are reported, which is enough to tell a missing key from
// a truncated one without disclosing any of it.
func (c Config) String() string {
	key := "absent"
	if n := len(c.PrivateKeyPEM); n > 0 {
		key = fmt.Sprintf("redacted (%d bytes)", n)
	}
	base := c.BaseURL
	if base == "" {
		base = "https://api.github.com/ (default)"
	}
	return fmt.Sprintf("github.Config{AppID:%d InstallationID:%d PrivateKeyPEM:%s BaseURL:%s Repository:%q}",
		c.AppID, c.InstallationID, key, base, c.Repository)
}

// checkPrivateKey parses the PEM and confirms it holds an RSA private key.
func checkPrivateKey(keyPEM []byte) error {
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return fmt.Errorf("%w: private key is not valid PEM (check that newlines survived the environment)", ErrInvalidConfig)
	}
	if _, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return nil
	}
	if _, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		return nil
	}
	return fmt.Errorf("%w: private key PEM block %q is neither PKCS#1 nor PKCS#8", ErrInvalidConfig, block.Type)
}

// ParseID converts a numeric identifier from the environment. It exists so the
// caller reporting a bad GITHUB_APP_ID names the variable in the message.
func ParseID(name, raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("%w: %s is required", ErrInvalidConfig, name)
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %s must be a number, got %q", ErrInvalidConfig, name, raw)
	}
	return id, nil
}

// ResolvePrivateKey turns the two ways a platform can hand over the App's
// private key into PEM bytes: inline, or as a path to a mounted file. Exactly
// one must be supplied.
//
// Inline values may additionally be base64-encoded. That is not ceremony: a
// PEM is multi-line, and several secret stores and CI systems mangle or refuse
// embedded newlines, so a single-line encoding is often the only form that
// survives the trip. Both shapes are accepted so an operator does not have to
// discover which one their platform tolerates.
func ResolvePrivateKey(inline, path string) ([]byte, error) {
	inline = strings.TrimSpace(inline)
	path = strings.TrimSpace(path)

	switch {
	case inline != "" && path != "":
		return nil, fmt.Errorf("%w: set the private key inline or as a file path, not both", ErrInvalidConfig)
	case inline != "":
		return decodeInlineKey(inline), nil
	case path != "":
		key, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("%w: reading private key file: %w", ErrInvalidConfig, err)
		}
		return key, nil
	default:
		return nil, fmt.Errorf("%w: no private key supplied", ErrInvalidConfig)
	}
}

// decodeInlineKey returns the PEM as given, or its base64 decoding when the
// value is not already PEM. A value that decodes to nothing PEM-shaped is
// returned untouched so validation reports the original input.
func decodeInlineKey(inline string) []byte {
	if strings.HasPrefix(inline, "-----BEGIN") {
		return []byte(inline)
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(inline), ""))
	if err == nil && strings.HasPrefix(string(decoded), "-----BEGIN") {
		return decoded
	}
	return []byte(inline)
}
