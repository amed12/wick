#!/usr/bin/env bash
# Merge the central repo into OUR master, cheaply.
#
#   local/sync-upstream.sh              # fetch + merge + build/test
#   local/sync-upstream.sh --no-check   # merge only
#   UPSTREAM_BRANCH=master local/sync-upstream.sh
#
# On conflict it prints only the seam files from local/seams.txt — and anything
# unexpected, which is the signal that upstream moved a file we had appended to.
# See local/README.md for the branch model.
set -euo pipefail

UPSTREAM_REMOTE="${UPSTREAM_REMOTE:-upstream}"
UPSTREAM_BRANCH="${UPSTREAM_BRANCH:-develop}"
LOCAL_BRANCH="${LOCAL_BRANCH:-master}"
CHECK=1

for arg in "$@"; do
  case "$arg" in
    --no-check) CHECK=0 ;;
    -h|--help)
      sed -n '2,10p' "$0" | sed 's/^# \{0,1\}//'
      exit 0
      ;;
    *) echo "unknown argument: $arg" >&2; exit 2 ;;
  esac
done

say() { printf '\n\033[1m%s\033[0m\n' "$*"; }
fail() { printf '\n\033[31m%s\033[0m\n' "$*" >&2; exit 1; }

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

# ── remote ───────────────────────────────────────────────────────────────────
if ! git remote get-url "$UPSTREAM_REMOTE" >/dev/null 2>&1; then
  if [ "$UPSTREAM_REMOTE" = "upstream" ] && git remote get-url origin >/dev/null 2>&1; then
    echo "note: no 'upstream' remote — using origin (add one to keep your fork separate)"
    UPSTREAM_REMOTE=origin
  else
    fail "remote '$UPSTREAM_REMOTE' is not configured"
  fi
fi

# A dirty tree means someone is mid-edit; a merge on top of that is unreviewable.
if [ -n "$(git status --porcelain)" ]; then
  fail "working tree is dirty — commit or stash first (local/README.md explains why)"
fi

say "fetching $UPSTREAM_REMOTE/$UPSTREAM_BRANCH"
git fetch --prune "$UPSTREAM_REMOTE"
git rev-parse --verify --quiet "$UPSTREAM_REMOTE/$UPSTREAM_BRANCH" >/dev/null \
  || fail "$UPSTREAM_REMOTE/$UPSTREAM_BRANCH does not exist"

git switch "$LOCAL_BRANCH" >/dev/null
# Resolve a conflict once, resolve it forever; zdiff3 shows the common context
# so a two-line seam edit is obvious.
git config rerere.enabled true
git config merge.conflictStyle zdiff3

BEFORE="$(git rev-parse HEAD)"
TARGET="$UPSTREAM_REMOTE/$UPSTREAM_BRANCH"
AHEAD="$(git rev-list --count "$TARGET..$BEFORE")"
BEHIND="$(git rev-list --count "$BEFORE..$TARGET")"
echo "  $LOCAL_BRANCH: $AHEAD local commit(s), $BEHIND upstream commit(s) to absorb"

if [ "$BEHIND" -eq 0 ]; then
  say "already up to date"
  exit 0
fi

say "merging $TARGET into $LOCAL_BRANCH"
if ! git merge --no-edit "$TARGET"; then
  echo
  echo "conflicts:"
  git diff --name-only --diff-filter=U | while IFS= read -r f; do
    if grep -qxF "$f" local/seams.txt 2>/dev/null; then
      echo "  seam       $f"
    else
      echo "  UNEXPECTED $f   <- upstream touched a file we only append to; read it"
    fi
  done
  echo
  echo "Everything else merged cleanly. Resolve the files above, then:"
  echo "  git add <files> && git commit"
  echo "  local/sync-upstream.sh --no-check"
  exit 1
fi

say "merged"
git diff --stat "$BEFORE"..HEAD

if [ "$CHECK" -eq 1 ]; then
  say "checking"
  if command -v templ >/dev/null 2>&1; then
    templ generate >/dev/null
    echo "  templ generate      ok"
  else
    echo "  templ generate      SKIPPED (templ not on PATH)"
  fi

  go build ./internal/... . >/dev/null
  echo "  go build            ok"

  go test ./internal/agents/plan ./internal/agents/ticket ./internal/agents/ticketprompt >/dev/null
  echo "  plan/ticket tests   ok"

  echo
  echo "Run the wider gate before shipping:"
  echo "  go test ./internal/tools/agents ./internal/agents/skillsync"
  echo "  cd fe && npm --workspace=@wick-fe/agents-plan run check"
fi

say "done — rebuild generated assets if the merge touched them:"
echo "  ./bin/tailwindcss -i web/src/input.css -o web/public/css/app.css --minify"
echo "  cd fe && npm install && npm --workspace=@wick-fe/agents-plan run build"
