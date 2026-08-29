// Package github holds the bot GitHub App client that opens pull requests
// against ColdCrabby/presets on a vendor's behalf.
//
// The bot proposes; humans merge. The App is granted only enough authority to
// push a branch and open a pull request, and VerifyInstallation asserts that at
// startup — including the absence of the permissions that would let it edit
// branch protection on main. Preventing a merge is ultimately the repository's
// branch rules; this package's job is to make an over-granted App loud rather
// than convenient.
//
// Authentication is by installation token, minted from the App private key and
// refreshed by the transport before its roughly hourly expiry, so no restart is
// needed to stay signed in. Credentials come from the environment, populated
// from managed secret storage — see docs/github-app-setup.md for the App's
// expected settings and the key rotation procedure.
//
// Building change sets (blobs, trees, commits, deterministic branch names and
// idempotent pull requests) is a separate concern that depends on
// authorization; see docs/vendor-workflow.md.
package github
