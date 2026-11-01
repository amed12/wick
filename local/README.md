# local/ — our wick

This directory is **ours**. Nothing here exists upstream, so it never conflicts.
It documents how this checkout stays a usable local product while the central
repo keeps moving, and it holds the tooling that makes a sync cheap.

## The shape

```
upstream/master      central release          (fetch only)
upstream/develop     central integration      (fetch only, the usual sync source)
        │
        ├── up/<topic>          upstream-able work: PRs, generic seams, fixes
        │
master               OUR product             upstream + local features
        ├── feat/plans          local feature (this repo's planning workspace)
        ├── feat/slack-dm-only  local feature (or a PR branch)
        └── ...
```

Rules:

1. **`master` is the product.** It is the branch we build from.
2. **Features are branches.** `master` receives merges, never long-lived commits
   made directly on it. That keeps "what is ours" a list of merge commits, which
   is what you review after a sync.
3. **Never rebase `master`.** Rebase rewrites the merge commits and makes the next
   upstream merge fight history. Merge upstream in, instead.
4. **Upstream-able work starts from `upstream/develop`**, not from `master`. A PR
   branch carries only the change and nothing local.
5. **Local-only work stays local.** The planning workspace is product, not an
   upstream patch. Keeping it out of the PR queue is what stops "merged upstream"
   and "our copy" from becoming two competing versions of the same code.

## Sync

```bash
local/sync-upstream.sh              # fetch + merge upstream/develop into master + check
local/sync-upstream.sh --no-check   # merge only
UPSTREAM_BRANCH=master local/sync-upstream.sh
```

The script:

- refuses to run on a dirty tree (so a half-finished edit never rides along),
- fetches the central repo and merges it into `master` (fast-forward when possible,
  merge commit when not),
- on conflict, prints **only the seam files** (below) that need a human, and stops
  with the tree intact,
- runs `templ generate`, `go build ./internal/... .`, and the plan/ticket tests.

`git rerere` and `merge.conflictStyle=zdiff3` are enabled by the script; a conflict
you resolved once resolves itself the next time.

The remotes this checkout uses:

```
origin    https://github.com/amed12/wick.git   fork — push local work here
upstream  https://github.com/yogasw/wick.git   central — fetch only (push = DISABLED)
```

Recreate them on a fresh clone:

```bash
git remote add origin   https://github.com/amed12/wick.git
git remote add upstream https://github.com/yogasw/wick.git
git remote set-url --push upstream DISABLED
git fetch origin --prune && git fetch upstream --prune
```

If `upstream` is absent, the script falls back to `origin` — but with `origin`
pointing at the fork, that would sync the fork, not the central repo, so keep both.

### Hooks (local — no CI minutes)

GitHub Actions is deliberately **not** used for this. The gate is local:

```bash
local/install-hooks.sh        # once per clone
```

That sets `core.hooksPath=$ROOT/.githooks` and
`branch.master.mergeOptions=--no-ff`.

`.githooks/pre-merge-commit` then, on any merge into `master`:

1. fetches `upstream/develop` (configurable via `UPSTREAM_REMOTE` /
   `UPSTREAM_BRANCH`),
2. allows the merge when its result contains that upstream commit — either
   `master` already has it, or the branch being merged brings it in,
3. otherwise aborts and tells you to `git merge --abort` + run
   `local/sync-upstream.sh`.

Why `pre-merge-commit` and not "pre-merge": Git has no pre-merge hook. It runs
after the merge is resolved but **before the commit**, which is the last point a
gate can stop it. `--no-ff` on `master` makes sure it also runs for merges that
would otherwise fast-forward.

Escape hatches:

```bash
git merge --no-verify                 # bypass the gate once
WICK_HOOK_OFFLINE=1 git merge <ref>   # skip the fetch, keep the ancestry check
```

The build/test check is not in the hook — a full `go build` + SPA build on every
merge is slower than the merge itself. `local/sync-upstream.sh` runs it after a
sync, and the shipped `pre-push` hook can be re-enabled if you want it there.

## Seams — the only files a sync can conflict with

Everything else we add is a new file or directory. `local/seams.txt` lists the
upstream files we edited, and each edit is marked `LOCAL(<feature>)` so a conflict
is a two-minute read rather than an archaeology dig.

| Seam | What we change | Size |
|---|---|---|
| `internal/agents/config/layout.go` | plan storage paths | ~16 lines |
| `internal/agents/ticket/ticket.go` | `Kind`, `ParentID`, `PlanID`, `Acceptance`, `DependsOn` | ~60 lines |
| `internal/agents/ticketprompt/pointer.go` (+ its test) | plan excerpt + done-when in the prompt | ~30 lines |
| `internal/pkg/api/server.go` | aigen kind + connector registration | 3 lines |
| `internal/tools/agents/handler.go` | `registerPlanRoutes`, `registerSPAPlan` | 2 lines |
| `internal/tools/agents/spa_handler.go` | plan SPA mount (appended) | ~20 lines |
| `internal/tools/agents/view/layout.templ` | `Plans` icon case | ~8 lines |
| `internal/tools/agents/view/nav_more.go` | one nav candidate | 1 line |
| `fe/package.json`, `fe/scripts/dev.mjs` | workspace + dev entry | 1 line each |
| `AGENTS.md`, `docs/.vitepress/config.ts` | docs/skill entries | 1 line each |

New files (no conflict ever):

```
internal/agents/plan/**
internal/connectors/plans/**
internal/tools/agents/api_plans.go
internal/tools/agents/view/plan_spa.templ
fe/agents/plan/**
internal/agents/skillsync/builtin/wick-plans/**
.claude/skills/plan-module/**
docs/guide/plans.md
docs/connectors/plans.md
local/**
```

## Generated files

`web/public/css/app.css`, `fe/package-lock.json`, `internal/**/dist/**` and
`*_templ.go` are build output. After a merge, regenerate rather than merge them:

```bash
templ generate
./bin/tailwindcss -i web/src/input.css -o web/public/css/app.css --minify
cd fe && npm install && npm --workspace=@wick-fe/agents-plan run build
```

If they conflict often enough to annoy, add a merge driver that keeps ours and
regenerates:

```bash
git config merge.regenerate.name "keep ours, regenerate after"
git config merge.regenerate.driver "cp %A %A"
printf 'web/public/css/app.css merge=regenerate\nfe/package-lock.json merge=regenerate\n' >> .gitattributes
```

Do this only for files you **always** regenerate; never for source.

## Contributing upstream — always available

Contribution work does not mix with product work:

```bash
git fetch upstream
git switch -c up/<topic> upstream/develop
# ... atomic commits, conventional messages ...
gh pr create --repo yogasw/wick --base develop
```

What goes upstream:

- bug fixes and docs corrections,
- **generic extension points** (a hook, a registry entry point, a config seam) —
  the same seams this fork needs, written upstream-first so the local feature can
  shrink onto them,
- tests for behaviour upstream already has.

What stays local: the planning product, the opinionated UI, anything whose value
is "how we run wick", not "what wick should do for everyone".

After a PR merges, delete `up/<topic>` and let the next sync bring it in. If you
kept the same change locally to unblock yourself in the meantime, the sync will
show it as already-present; resolve by taking upstream's copy and dropping the
local commit, so there is exactly one version.

## Commit convention

| Prefix | Meaning |
|---|---|
| `local:` | ours, will never be a PR |
| `feat:`, `fix:` | upstream-able, conventional-commit style |
| `merge:` | a sync or integration merge |

Tag local releases as `v<upstream>-local.<n>` so a bug report can name a build.
