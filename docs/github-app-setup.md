# GitHub App Setup — The Presets Bot

How the bot GitHub App is configured, how its credentials reach the API, and how
to rotate its private key. The App is the identity that commits a vendor's
validated change to a branch and opens a pull request against
[`ColdCrabby/presets`](https://github.com/ColdCrabby/presets).

The design rationale is in
[vendor-workflow.md](./vendor-workflow.md#change-to-pull-request) and
[ARCHITECTURE.md](../ARCHITECTURE.md#secrets). This document is the operational
checklist. The client itself lives in `internal/github`.

**The bot proposes; humans merge.** Every choice below follows from that. An App
that could merge its own pull requests would quietly void the review model the
whole system rests on.

## Table of Contents

1. [App Settings](#app-settings)
2. [Permissions](#permissions)
3. [Installation](#installation)
4. [Environment Variables](#environment-variables)
5. [Supplying the Private Key](#supplying-the-private-key)
6. [Startup Preflight](#startup-preflight)
7. [Token Lifetime](#token-lifetime)
8. [Rate Limits](#rate-limits)
9. [Rotating the Private Key](#rotating-the-private-key)

---

## App Settings

Registered once, under the `ColdCrabby` organization, at
**Settings → Developer settings → GitHub Apps**.

| Setting                            | Value                                                             |
| ---------------------------------- | ----------------------------------------------------------------- |
| Name                               | The bot identity that will author every preset commit             |
| Homepage URL                       | This repository                                                   |
| Callback URL                       | Not used — vendors authenticate with Stytch, never with GitHub    |
| Request user authorization (OAuth) | **off** — the bot acts as an installation, not on a user's behalf |
| Webhook                            | Optional here. Ingest's `push` webhook is a separate concern (#8) |
| Where can it be installed          | Only on this account                                              |

Vendor staff may not have a GitHub account at all — a filament company's
materials engineer should not need one to correct a nozzle temperature. That is
why the App authenticates as an installation and there is no user authorization
flow to configure.

---

## Permissions

Repository permissions, and nothing else:

| Permission      | Level     | Why                                       |
| --------------- | --------- | ----------------------------------------- |
| `contents`      | **Write** | Create branches, blobs, trees and commits |
| `pull_requests` | **Write** | Open and update pull requests             |
| `metadata`      | Read      | Mandatory for every App                   |

Organization permissions: **none**. Account permissions: **none**.

**Nothing else.** The startup preflight is deny-by-default: any permission
granted beyond the three above is reported as excess, whether or not it looks
dangerous. A blocklist would only ever be as current as the last time someone
read GitHub's permission catalogue, and several unremarkable-sounding grants
defeat the review model outright:

| Permission                                | Why not                                                         |
| ----------------------------------------- | --------------------------------------------------------------- |
| `administration`                          | Would let the bot edit or remove branch protection on `main`    |
| `workflows`                               | The bot must not be able to alter the CI that gates its own PRs |
| `checks` / `statuses`                     | Would let the bot satisfy a required status check itself        |
| `organization_administration`             | Nothing the bot does concerns org settings                      |
| `repository_hooks` / `organization_hooks` | Webhook configuration is an operator action, not a bot one      |

`contents: write` is the permission that lets the App push a branch. It is also,
on an unprotected branch, enough to push straight to `main` — GitHub has no
"branches except main" scope. **Branch protection on `main` is therefore the real
control**, not the permission set: require a pull request, require review from
`CODEOWNERS`, require status checks, and do not add the App to any bypass list.
The absence of `administration` is what stops the bot from editing that rule.

`internal/github` refuses to consider the installation healthy if any permission
outside the required three is present — see
[Startup Preflight](#startup-preflight). One caveat worth knowing: the check
reads the permissions through `go-github`'s typed struct, so a permission GitHub
introduces after that dependency's release is invisible to it. Keeping
`go-github` current is part of keeping this check honest.

---

## Installation

Install the App on `ColdCrabby/presets` only, via **Only select repositories**.
An App installed across the whole organization would hold `contents: write` on
every repository the org owns, including this one, for no benefit.

After installing, note the **installation ID** from the installation's settings
URL: `.../settings/installations/<installation_id>`. It is not secret, but it is
required — tokens are minted per installation, and it is what scopes the bot's
authority.

---

## Environment Variables

Read in `cmd/server/main.go` with plain `os.Getenv` and passed into
`github.Config`. There is no config file and no secrets client: the deploy
platform (Fly.io, Cloud Run, Railway) injects these from its own secret store at
process start.

| Variable                      | Required | Default                  | Purpose                                           |
| ----------------------------- | -------- | ------------------------ | ------------------------------------------------- |
| `GITHUB_APP_ID`               | yes      | —                        | Numeric App ID (not the client ID)                |
| `GITHUB_APP_INSTALLATION_ID`  | yes      | —                        | The installation on `ColdCrabby/presets`          |
| `GITHUB_APP_PRIVATE_KEY`      | one of   | —                        | The private key, PEM or base64-encoded PEM        |
| `GITHUB_APP_PRIVATE_KEY_FILE` | one of   | —                        | Path to the key file, for mounted secrets         |
| `GITHUB_PRESETS_REPOSITORY`   | no       | `ColdCrabby/presets`     | `owner/name` verified to be in installation scope |
| `GITHUB_API_BASE_URL`         | no       | `https://api.github.com` | Override the API root (GitHub Enterprise)         |

Only the private key is secret. The IDs and repository name are safe to log, and
`github.Config`'s `String()` redacts the key so the whole config can be printed
during debugging without leaking it.

**If none of these are set the API still starts.** It logs that vendor
submissions are disabled and serves the catalog normally. The public browse path
has no business being held offline by a credential only vendor writes need.

---

## Supplying the Private Key

Generate the key in the App's settings (**Private keys → Generate a private
key**). GitHub downloads a PKCS#1 PEM once and keeps only the fingerprint — if
it is lost, generate a new one; it cannot be re-downloaded.

Never commit it. `.env` is gitignored, but the key does not belong in the repo,
the image, or a build argument in any form.

Both PKCS#1 (`BEGIN RSA PRIVATE KEY`) and PKCS#8 (`BEGIN PRIVATE KEY`) are
accepted, in either of two shapes:

**Inline.** A PEM is multi-line, and several secret stores and CI systems mangle
or refuse embedded newlines, so a base64 encoding of the whole PEM is also
accepted — often it is the only form that survives the trip:

```sh
# Fly.io — literal PEM
fly secrets set GITHUB_APP_PRIVATE_KEY="$(cat app.private-key.pem)"

# Anywhere newlines are a problem — single-line base64 of the same PEM
GITHUB_APP_PRIVATE_KEY="$(base64 < app.private-key.pem | tr -d '\n')"
```

**As a file.** For platforms that mount secrets as files (Cloud Run secret
volumes, Kubernetes secrets), point at the path instead:

```sh
GITHUB_APP_PRIVATE_KEY_FILE=/secrets/github-app.pem
```

Set exactly one. Setting both is rejected at startup rather than resolved by a
precedence rule nobody would remember.

The key is parsed during config validation, so a truncated or newline-mangled
value fails immediately with a clear message — not minutes later as an opaque
`401` from the token endpoint.

---

## Startup Preflight

`Client.VerifyInstallation` runs once at startup, with a 15-second timeout. It:

1. mints an installation token, proving the key and both IDs agree;
2. compares the **granted** permissions against the required set — reporting
   both what is missing and anything granted beyond it;
3. confirms `GITHUB_PRESETS_REPOSITORY` is in the installation's scope.

Run it before the client is shared with concurrent callers: reading the
installation's permissions touches transport state the transport does not itself
guard.

Failure is logged, not fatal. A misconfigured or over-broad App is worth
shouting about at boot rather than discovering on a vendor's first submission,
but a momentary GitHub outage is not a reason to stop serving the catalog.

The scope check earns its place: an App installed on the wrong account, or on
"selected repositories" that omit the presets repo, authenticates perfectly and
then returns `404` on the first real call. The preflight turns that into a
startup error naming the repositories the installation can actually reach.

---

## Token Lifetime

The App's private key signs a short-lived App JWT, which is exchanged for an
**installation access token valid for about one hour**. `ghinstallation`'s
transport mints one on first use and replaces it shortly before expiry, so a
long-running process stays authenticated without a restart and no caller has to
track expiry.

A revoked or rotated-away key surfaces as a `*github.PermissionError` from
`Client.Token` whose message says the key was rejected — which is what a stale
deployment looks like after a rotation.

---

## Rate Limits

An installation on a single repository gets a modest hourly quota, and GitHub
additionally applies **secondary** limits to bursts of writes. Both are normalised
into `*github.RateLimitedError`, carrying the kind (primary or secondary), the
quota counters, the reset time, and any server-supplied `Retry-After`:

```go
var limited *github.RateLimitedError
if errors.As(err, &limited) {
    wait := limited.RetryAfter(time.Now()) // 0 when GitHub gave no hint
}
```

Errors from calls made through `Client.REST()` should be passed through
`github.Classify` so a rate limit hit is reported the same way from every call
site. The distinction matters to the caller: a primary limit refills at a known
instant, while a secondary limit is a penalty for sending too much at once and
the only honest advice is the server's own `Retry-After`.

Surfacing it is the UI's job — the slicer and the admin app render "rate
exceeded, try again at …" from these fields.

---

## Rotating the Private Key

**A GitHub App can hold two private keys at once, and both work.** That overlap
is the entire rotation mechanism, so no previous-secret fallback exists in the
code — building one would duplicate, less reliably, something GitHub already
guarantees.

1. **Generate** a second private key in the App's settings. Both are now valid.
2. **Update the secret** in the platform's secret store with the new PEM.
3. **Deploy / restart** so the process picks it up. Watch for
   `github: authenticated as app <id> on ColdCrabby/presets` in the logs — that
   line only appears after a successful token mint and preflight.
4. **Verify** a real submission opens a pull request, or re-run the preflight.
5. **Delete the old key** in the App's settings. Only now does the old PEM stop
   working.
6. **Destroy local copies** of the downloaded PEM file.

Do not delete the old key before step 4. Between steps 1 and 5 either key is
accepted, so the rotation needs no synchronized restart and no downtime window.

Rotate on a schedule (annually is reasonable for a key with this narrow a blast
radius), and immediately on any suspected exposure or when someone with access
to the secret store leaves.

## Reference

- [Authenticating as a GitHub App installation](https://docs.github.com/apps/creating-github-apps/authenticating-with-a-github-app/authenticating-as-a-github-app-installation)
- [Rate limits for the REST API](https://docs.github.com/rest/using-the-rest-api/rate-limits-for-the-rest-api)
- [vendor-workflow.md](./vendor-workflow.md#change-to-pull-request) — how a change becomes a pull request
- [auth-setup.md](./auth-setup.md) — the other half of the credential story: vendor identity via Stytch
