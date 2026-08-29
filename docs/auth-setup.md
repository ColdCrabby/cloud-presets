# Auth Setup — Stytch B2B and Offline JWT Validation

How to configure the Stytch B2B project and run the API's session-token
middleware. This covers **identity only** — validating _who_ is calling and
surfacing their organization to handlers. Mapping an organization to _authority
over a vendor namespace_ (the `403` on ownership mismatch) is a separate concern
decided against the Git manifest, described in
[vendor-workflow.md](./vendor-workflow.md#authorization).

The design rationale lives in
[ARCHITECTURE.md](../ARCHITECTURE.md#identity--ownership-without-a-database) and
[vendor-workflow.md](./vendor-workflow.md#token-validation). This document is the
operational checklist.

## Table of Contents

1. [Stytch B2B Project Configuration](#stytch-b2b-project-configuration)
2. [Environment Variables](#environment-variables)
3. [What the Middleware Checks](#what-the-middleware-checks)
4. [JWKS Rotation](#jwks-rotation)
5. [401 vs 403](#401-vs-403)
6. [Wiring It Into the API](#wiring-it-into-the-api)

---

## Stytch B2B Project Configuration

Do this once in the [Stytch dashboard](https://stytch.com/dashboard). These steps
are console configuration, not code.

1. **Create a B2B project.** Choose the _B2B SaaS Authentication_ product, not
   Consumer. The organization/member model is what maps to vendor companies and
   their staff.
2. **Enable OAuth providers.** Turn on **GitHub** and **Google**. Vendor staff
   sign in with an identity they already have; there are no passwords (a password
   store would reintroduce the database this system deliberately omits).
3. **Enable the Discovery flow.** A person may belong to more than one vendor, so
   sign-in resolves the list of organizations they can access and exchanges for a
   session scoped to the one they choose. The session's `organization_id` claim —
   not any client parameter — is the caller's namespace.
4. **Configure redirect URLs** for the Vendor Admin app's login and discovery
   callback routes, per environment (test and live have separate URLs).
5. **Note the project identifiers.** The **project ID** is the JWT audience and
   is used to derive the issuer and JWKS URL. Copy it into `STYTCH_PROJECT_ID`.
6. **Store secrets in managed storage.** The Stytch **secret** (used by the
   frontend/back-of-house flows, not by this offline validator) and any other
   credentials live in managed secret storage, injected as environment variables
   at deploy time — **never** committed to the repo or baked into the image. The
   JWT validator itself needs no secret: it only fetches _public_ JWKS.

Test vs. live: Stytch serves test-environment JWKS from `test.stytch.com` and
live from `api.stytch.com`. Set `STYTCH_JWKS_URL` explicitly for the test
environment (see below).

---

## Environment Variables

The middleware reads its configuration from the environment via
`auth.LoadConfigFromEnv()`. Only `STYTCH_PROJECT_ID` is required; the rest have
sensible defaults derived from it.

| Variable               | Required | Default                                                    | Purpose                                                                                      |
| ---------------------- | -------- | ---------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| `STYTCH_PROJECT_ID`    | **yes**  | —                                                          | B2B project ID; the expected `aud` claim, and the basis for the default issuer and JWKS URL. |
| `STYTCH_ISSUER`        | no       | `stytch.com/<project_id>`                                  | Expected `iss` claim. Override for a custom domain.                                          |
| `STYTCH_AUDIENCE`      | no       | `<project_id>`                                             | Expected `aud` claim.                                                                        |
| `STYTCH_JWKS_URL`      | no       | `https://api.stytch.com/v1/b2b/sessions/jwks/<project_id>` | Public signing keys. Point at `https://test.stytch.com/...` for the test environment.        |
| `STYTCH_MAX_TOKEN_AGE` | no       | `5m30s`                                                    | Maximum age from `iat`, on top of `exp`. A Go duration string.                               |

Because every value here is a public identifier or URL, this configuration is
safe to log. The secrets that _are_ sensitive belong to other parts of the
system (GitHub App key, webhook HMAC) and are out of scope for token validation.

---

## What the Middleware Checks

Every request to a vendor endpoint carries an `Authorization: Bearer <jwt>`
header. Validation happens **offline** — no network call to Stytch on the request
path — against the cached JWKS:

- **Signature** against a cached public key selected by the token's `kid`.
- **`iss`** equals the configured issuer.
- **`aud`** includes the configured audience (the project ID).
- **`exp`** is not in the past (with a small clock-skew allowance).
- **`iat`** is present, not in the future, and not older than the maximum token
  age.

On success the handler can read the caller's identity:

```go
claims, ok := auth.ClaimsFromContext(r.Context())
// claims.OrganizationID, claims.OrganizationSlug, claims.Roles, claims.Subject
```

`organization_id` is the authoritative namespace for authorization; `roles`
distinguishes proposing a change from administering the organization.

Stytch session JWTs have a **fixed five-minute lifetime** regardless of session
duration — the long-lived artifact is the opaque session token, which the
frontend uses to mint a fresh JWT continuously. A validator that assumed a
long-lived JWT would be wrong here.

---

## JWKS Rotation

Signing keys rotate roughly **every six months, with a one-month overlap** during
which Stytch publishes both the outgoing and incoming key. The middleware handles
this without a restart:

- Keys are fetched once at startup and **refreshed in the background** (default
  every 15 minutes), so the request path never fetches.
- Because the cached set holds _every_ published key, a token signed by either
  key during the overlap verifies.
- If a token arrives signed by a key newer than the last background refresh, the
  middleware triggers a single **throttled** refresh and retries once, so a fresh
  key is picked up promptly without letting bad-signature traffic amplify into a
  fetch storm.

---

## 401 vs 403

This split is load-bearing for the admin app and must not be collapsed:

- **`401`** — missing, expired, malformed, or otherwise invalid session JWT. The
  admin app **silently refreshes and retries once**. The middleware sets a
  `WWW-Authenticate: Bearer` header and returns a JSON body describing the
  reason.
- **`403`** — a valid identity that does not own the target vendor namespace.
  This is decided later, against the Git manifest, and needs an **explanation**,
  not a retry.

Collapsing both into one status would make the admin app retry forever on a
permission error. This middleware only ever produces `401`; the `403` belongs to
the authorization step.

---

## Wiring It Into the API

```go
cfg, err := auth.LoadConfigFromEnv()
if err != nil {
    log.Fatal(err)
}

verifier, err := auth.New(ctx, cfg) // ctx governs the background refresh goroutine
if err != nil {
    log.Fatal(err)
}

mw := auth.NewMiddleware(verifier)

// Protect vendor routes; public routes stay unwrapped.
mux.Handle("/vendor/", mw.RequireAuth(vendorHandler))
```

The middleware is plain `net/http`, so it composes with the stdlib
`http.ServeMux` adapter that Huma runs on — no router dependency.
