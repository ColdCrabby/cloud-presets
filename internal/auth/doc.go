// Package auth verifies Stytch B2B session JWTs offline against a cached
// JWKS, with no per-request network call. Mapping an authenticated
// organization to authority over a vendor namespace (the 403 case) is a
// later, catalog-dependent addition. See docs/auth-setup.md.
package auth
