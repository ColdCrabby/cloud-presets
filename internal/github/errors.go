package github

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	gh "github.com/google/go-github/v88/github"
)

// Sentinel errors for conditions that are not usefully described by a struct.
var (
	// ErrInvalidConfig is returned when the App credentials are missing or
	// malformed. It is a startup problem, never a request-path one.
	ErrInvalidConfig = errors.New("github: invalid configuration")

	// ErrNotConfigured is returned by callers that hold no client because no
	// GitHub App credentials were supplied. The API keeps serving the catalog
	// without them; only vendor submissions need the bot.
	ErrNotConfigured = errors.New("github: app not configured")
)

// RateLimitKind distinguishes GitHub's two throttles, which need different
// responses. A primary limit is a fixed hourly budget that refills at a known
// instant; a secondary limit is a dynamic penalty for sending too much at once,
// and the only honest advice is the server's own Retry-After.
type RateLimitKind string

const (
	// RateLimitPrimary is the hourly quota for the installation.
	RateLimitPrimary RateLimitKind = "primary"

	// RateLimitSecondary is the abuse / concurrency throttle.
	RateLimitSecondary RateLimitKind = "secondary"
)

// RateLimitedError reports that GitHub refused the call because of a rate
// limit. It carries enough to render a truthful message — the slicer and the
// admin app show "rate exceeded, try again at X" rather than a generic failure
// — and to set a Retry-After header without re-deriving anything from the raw
// response.
type RateLimitedError struct {
	// Kind is which throttle fired.
	Kind RateLimitKind

	// Limit, Remaining and Used are the primary quota counters. They are zero
	// for a secondary limit, which GitHub does not quantify.
	Limit     int
	Remaining int
	Used      int

	// Resource names the quota bucket ("core", "search", "graphql"), when
	// GitHub reports one.
	Resource string

	// Reset is when a primary quota refills. Zero for a secondary limit.
	Reset time.Time

	// Retry is the server-supplied Retry-After, when present. Zero means
	// GitHub gave no explicit hint.
	Retry time.Duration

	// Message is GitHub's own explanation.
	Message string

	err error
}

func (e *RateLimitedError) Error() string {
	var b strings.Builder
	fmt.Fprintf(&b, "github: %s rate limit exceeded", e.Kind)
	if e.Resource != "" {
		fmt.Fprintf(&b, " on %s", e.Resource)
	}
	if e.Limit > 0 {
		fmt.Fprintf(&b, " (%d/%d used)", e.Used, e.Limit)
	}
	if !e.Reset.IsZero() {
		fmt.Fprintf(&b, ", resets at %s", e.Reset.UTC().Format(time.RFC3339))
	}
	if e.Message != "" {
		fmt.Fprintf(&b, ": %s", e.Message)
	}
	return b.String()
}

func (e *RateLimitedError) Unwrap() error { return e.err }

// RetryAfter is how long to wait before trying again, measured from now. It
// prefers GitHub's explicit hint over the reset timestamp, and never returns a
// negative duration — a reset already in the past means retry immediately.
func (e *RateLimitedError) RetryAfter(now time.Time) time.Duration {
	if e.Retry > 0 {
		return e.Retry
	}
	if !e.Reset.IsZero() {
		if d := e.Reset.Sub(now); d > 0 {
			return d
		}
	}
	return 0
}

// PermissionError reports that the call was authenticated but not allowed:
// either the installation lacks a permission, or the resource is protected.
// This is the error that surfaces when someone narrows the App's grants without
// updating the code, so it says what was attempted rather than only echoing
// GitHub.
type PermissionError struct {
	// Op is what the client was doing, in plain words.
	Op string

	// Message is GitHub's explanation, when the failure came from an API call.
	Message string

	err error
}

func (e *PermissionError) Error() string {
	msg := fmt.Sprintf("github: not permitted to %s", e.Op)
	if e.Message != "" {
		msg += ": " + e.Message
	}
	return msg
}

func (e *PermissionError) Unwrap() error { return e.err }

// NotFoundError reports a 404. GitHub also returns 404 rather than 403 for
// resources an installation cannot see, so this can mean "absent" or "invisible
// to the bot" — the message says as much instead of asserting the first.
type NotFoundError struct {
	// Resource is what was being addressed, e.g. "ColdCrabby/presets".
	Resource string

	err error
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("github: %s not found, or not visible to this installation", e.Resource)
}

func (e *NotFoundError) Unwrap() error { return e.err }

// Classify normalises an error from a go-github call into this package's types
// so callers can branch with errors.As instead of matching on status codes.
// resp may be nil. A nil err returns nil.
//
// Callers that reach for the underlying go-github client via REST should run
// its errors through here, so that a rate limit hit from any call site is
// reported identically.
func Classify(op string, resp *gh.Response, err error) error {
	if err == nil {
		return nil
	}

	var rle *gh.RateLimitError
	if errors.As(err, &rle) {
		return &RateLimitedError{
			Kind:      RateLimitPrimary,
			Limit:     rle.Rate.Limit,
			Remaining: rle.Rate.Remaining,
			Used:      rle.Rate.Used,
			Resource:  rle.Rate.Resource,
			Reset:     rle.Rate.Reset.Time,
			Message:   rle.Message,
			err:       err,
		}
	}

	var arle *gh.AbuseRateLimitError
	if errors.As(err, &arle) {
		limited := &RateLimitedError{
			Kind:    RateLimitSecondary,
			Message: arle.Message,
			err:     err,
		}
		if arle.RetryAfter != nil {
			limited.Retry = *arle.RetryAfter
		}
		return limited
	}

	// A 403 carrying an exhausted quota is a rate limit even when go-github
	// did not type it as one, which happens for responses that omit the
	// documentation URL go-github keys on.
	if resp != nil && resp.StatusCode == http.StatusForbidden && resp.Rate.Limit > 0 && resp.Rate.Remaining == 0 {
		return &RateLimitedError{
			Kind:      RateLimitPrimary,
			Limit:     resp.Rate.Limit,
			Remaining: resp.Rate.Remaining,
			Used:      resp.Rate.Used,
			Resource:  resp.Rate.Resource,
			Reset:     resp.Rate.Reset.Time,
			Message:   messageOf(err),
			err:       err,
		}
	}

	if resp != nil {
		switch resp.StatusCode {
		case http.StatusNotFound:
			return &NotFoundError{Resource: op, err: err}
		case http.StatusUnauthorized, http.StatusForbidden:
			return &PermissionError{Op: op, Message: messageOf(err), err: err}
		}
	}

	return fmt.Errorf("github: %s: %w", op, err)
}

// messageOf extracts GitHub's own message from an error response, if there is
// one, so classified errors do not lose the API's explanation.
func messageOf(err error) string {
	var er *gh.ErrorResponse
	if errors.As(err, &er) {
		return er.Message
	}
	return ""
}
