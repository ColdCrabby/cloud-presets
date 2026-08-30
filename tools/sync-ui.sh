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
  git -C "$DEST" fetch --quiet origin "$BRANCH"
  # Fast-forward only: never clobber local commits waiting to be pushed upstream.
  if ! git -C "$DEST" merge --ff-only "origin/$BRANCH" >/dev/null 2>&1; then
    echo "  ! $DEST has local changes that diverge from origin/$BRANCH." >&2
    echo "    Leaving it untouched - reconcile it by hand." >&2
  fi
else
  echo "Cloning $REPO_URL into $DEST ..."
  git clone --branch "$BRANCH" "$REPO_URL" "$DEST"
fi
