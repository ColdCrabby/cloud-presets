package auth

import "errors"

// Errors returned by the verifier. They are deliberately coarse: a handler or
// middleware only needs to distinguish "the caller presented no usable identity"
// (all of these) from "the caller has a valid identity but lacks authority"
// (a 403, decided later against the Git manifest — see docs/vendor-workflow.md).
//
// Every error here maps to a 401. Authorization failures are a separate concern
// and are not represented in this package.
var (
	// ErrMissingToken is returned when no bearer token is present on the request.
	ErrMissingToken = errors.New("auth: missing session token")

	// ErrMalformedToken is returned when the token is not a well-formed JWT.
	ErrMalformedToken = errors.New("auth: malformed session token")

	// ErrInvalidSignature is returned when no cached JWKS key verifies the token
	// signature. During key rotation the cache holds both the old and new key,
	// so this only fires for genuinely bad signatures, not routine rotation.
	ErrInvalidSignature = errors.New("auth: invalid token signature")

	// ErrExpired is returned when the token's exp is in the past, or when it is
	// older than the configured maximum age. Stytch session JWTs live five
	// minutes; the admin app treats this as "refresh and retry once".
	ErrExpired = errors.New("auth: session token expired")

	// ErrInvalidClaims is returned when a required registered claim (iss, aud,
	// iat) is missing or does not match the expected value.
	ErrInvalidClaims = errors.New("auth: invalid token claims")
)
