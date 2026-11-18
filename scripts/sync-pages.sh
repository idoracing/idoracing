#!/usr/bin/env bash
# scripts/sync-pages.sh
# Synchronizes the docs/ directory from idoracing/idoracing to the
# root of idoracing/idoracing.github.io (pages main).
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
REMOTE_NAME="${1:-pages}"
REMOTE_URL="git@github.com:idoracing/idoracing.github.io.git"

# Verify or configure the pages remote
if ! git -C "$REPO_ROOT" remote get-url "$REMOTE_NAME" >/dev/null 2>&1; then
	echo "[sync-pages] Remote '$REMOTE_NAME' not found. Adding $REMOTE_URL..."
	git -C "$REPO_ROOT" remote add "$REMOTE_NAME" "$REMOTE_URL"
fi

echo "[sync-pages] Extracting docs/ subtree commit..."
COMMIT_SHA=$(git -C "$REPO_ROOT" subtree split --prefix docs HEAD 2>/dev/null | tail -n 1)

if [ -z "$COMMIT_SHA" ]; then
	echo "[sync-pages] ERROR: Could not generate subtree split for docs/." >&2
	exit 1
fi

echo "[sync-pages] Pushing docs/ ($COMMIT_SHA) to $REMOTE_NAME main..."
IDORACING_PAGES_SYNC=1 git -C "$REPO_ROOT" push "$REMOTE_NAME" "$COMMIT_SHA:refs/heads/main"

echo "[sync-pages] Done! idoracing.github.io is now up to date."
