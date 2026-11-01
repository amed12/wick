#!/usr/bin/env bash
# Turn on the local git gates for this clone.
#
#   local/install-hooks.sh
#
# Idempotent. Run it once per clone (hooks are local, not shared).
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

# Versioned hooks live in .githooks/; without this Git never reads them.
git config core.hooksPath "$ROOT/.githooks"
chmod +x "$ROOT"/.githooks/*

# pre-merge-commit does not run for a fast-forward merge. Forcing a merge
# commit on master makes the gate unconditional and gives every sync an
# auditable merge commit.
git config branch.master.mergeOptions --no-ff

# The gate fetches this remote. Push stays disabled: central is read-only.
if ! git remote get-url upstream >/dev/null 2>&1; then
  git remote add upstream https://github.com/yogasw/wick.git
  git remote set-url --push upstream DISABLED
  echo "added fetch-only remote 'upstream'"
fi

echo "hooks:    core.hooksPath = $(git config --get core.hooksPath)"
echo "merge:    branch.master.mergeOptions = $(git config --get branch.master.mergeOptions)"
echo "upstream: $(git remote get-url upstream)"
echo
echo "Gate: merging into master fetches upstream/develop and refuses a stale merge."
echo "Bypass once: git merge --no-verify"
