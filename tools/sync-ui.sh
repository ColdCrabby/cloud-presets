#!/usr/bin/env sh
# Sync the shared @coldcrabby/ui source into a git-ignored vendored checkout.
#
# The frontends consume @coldcrabby/ui as *source* (no dist, no npm release):
# each app maps a tsconfig path to `<checkout>/src/public-api.ts` and adds
# `<checkout>/src/styles` to its Sass includePaths. This script keeps that
# checkout on the latest `main`. It is a real working clone, so a local edit to
# the shared UI can be committed here and pushed straight to ColdCrabby/ui.
set -eu

# HTTPS so CI/deploy hosts (e.g. Render) with no SSH key can clone the public
# repo; override with UI_REPO_URL=git@github.com:ColdCrabby/ui.git to push.
REPO_URL="${UI_REPO_URL:-https://github.com/ColdCrabby/ui.git}"
DEST=".coldcrabby-ui"
BRANCH="main"

root_dir=$(cd "$(dirname "$0")/.." && pwd)
cd "$root_dir"

if [ -d "$DEST/.git" ]; then
  echo "Updating $DEST from origin/$BRANCH ..."
  # A persisted CI checkout may carry an old (e.g. SSH) origin that no longer
  # authenticates; re-point it so the fetch below cannot silently freeze the
  # vendored UI at a stale commit and ship outdated assets.
  git -C "$DEST" remote set-url origin "$REPO_URL"
  git -C "$DEST" fetch --quiet origin "$BRANCH"
  # Fast-forward only: never clobber local commits waiting to be pushed upstream.
  if ! git -C "$DEST" merge --ff-only "origin/$BRANCH" >/dev/null 2>&1; then
    echo "  ! $DEST has local changes that diverge from origin/$BRANCH." >&2
    echo "    Leaving it untouched - reconcile it by hand." >&2
  fi
elif [ -d "$DEST" ]; then
  # The directory exists but is not a git checkout: pnpm pre-creates this
  # workspace member (its node_modules) before this postinstall runs, and a
  # plain `git clone` refuses a non-empty target. Initialise in place instead.
  echo "Initialising $DEST in place from $REPO_URL ..."
  git -C "$DEST" init -q
  git -C "$DEST" remote add origin "$REPO_URL" 2>/dev/null \
    || git -C "$DEST" remote set-url origin "$REPO_URL"
  git -C "$DEST" fetch --quiet origin "$BRANCH"
  git -C "$DEST" checkout -q -f -B "$BRANCH" "origin/$BRANCH"
else
  echo "Cloning $REPO_URL into $DEST ..."
  git clone --branch "$BRANCH" "$REPO_URL" "$DEST"
fi
