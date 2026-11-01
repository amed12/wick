---
name: plan-module
description: Use for any work on the planning workspace — the PRD/roadmap/feature breakdown under internal/agents/plan, the /tools/agents/plan SPA, the plans MCP connector, or the ticket kind/parent/plan fields. Covers why features and tasks are tickets rather than a second store, where a plan lives on disk, the progress contract, and which files to touch.
---

# Plan module

A **plan** is one phase of a project: a versioned PRD plus the features and tasks
it decomposes into. One project has many plans over its life (Plan 1, Plan 2, …).

## The one rule

**Features and tasks are tickets.** A feature is a ticket with `Kind=feature`; a
task is a ticket with `ParentID=<feature id>`. The plan only stores the document,
the ordered feature ids, and the context refs.

This is not an implementation detail — it is why the feature exists at all. The
board, notes, sessions, webhooks and every MCP ticket op keep working, and
progress is a query over `ticket.Status`, never a number someone typed into the
plan. If you find yourself adding a "percent done" field to `Plan`, stop.

Consequences:

- Deleting a plan keeps its tickets (`plan.Delete` removes only the plan dir).
- Moving a task's status is the ONLY way progress changes.
- `Apply` is all-or-nothing: it validates every spec first and rolls back the
  tickets it created if a later write fails.

## Files

| File | Role |
|---|---|
| `internal/agents/plan/plan.go` | entity, store, versions, context, status validation |
| `internal/agents/plan/breakdown.go` | `FeatureSpec`/`TaskSpec`, `Apply`, `Build` (roadmap), `Unplanned` |
| `internal/agents/plan/generate.go` | `aigen` kind `plan-prd` — one call returns PRD + breakdown |
| `internal/tools/agents/api_plans.go` | HTTP handlers under `/api/projects/{id}/plans…` |
| `internal/tools/agents/spa_handler.go` | `registerSPAPlan` — mounts the SPA at `/plan/` |
| `internal/tools/agents/view/plan_spa.templ` | thin Go shell (`templ generate` after edits) |
| `internal/connectors/plans/connector.go` | MCP ops for agents |
| `fe/agents/plan/` | Svelte 5 SPA (PRD editor, roadmap, feature detail) |
| `internal/agents/ticketprompt/pointer.go` | injects the plan excerpt + acceptance into a session prompt |

## Storage

```
<agents base>/projects/<projectID>/plans/<planID>/plan.json
```

`planID` is a short quotable code (`P-7QK2`). `Number` is the phase number,
assigned as `max(existing)+1`. Versions are inline in `plan.json`
(`versions[]`, capped at `MaxVersions`); `SetPRD` skips an unchanged document so
an autosave does not fill history.

## Progress

`Build` loads the project's tickets, keeps those with `PlanID == plan.ID`, groups
tasks by `ParentID`, and counts `Done` where status equals the project's terminal
status (`project.TicketConfig.TerminalStatus`). Never cache this — the board is
the source of truth and the roadmap must reflect a drag on the board immediately.

## Agent-side contract

- `ticketprompt.Pointer` prepends `Plan N — title`, the summary, a bounded PRD
  excerpt, and the ticket's `Acceptance` when a session sits on a planned ticket.
- The `plans` connector is read/write for the document and breakdown; progress
  goes through the `tickets` connector.
- `plan_add_features` creates tickets in bulk. It validates first and rolls
  back on failure; it is not destructive-marked because `ticket_create` is not
  either, but a duplicate call still duplicates work.

## Gotchas

- `plan` imports `ticket`; never the reverse (cycle). `ticketprompt` may import
  both because neither imports it.
- New templ file ⇒ run `templ generate` before `go build`.
- New SPA ⇒ build it (`npm --workspace=@wick-fe/agents-plan run build`) before
  the Go binary will embed `dist/plan`; add it to `fe/scripts/dev.mjs` too.
- The SPA's asset base is `/tools/agents/plan/`; `registerSPAPlan` reuses the
  shared multi-app handler by prepending the `plan` app segment.
- Tailwind scans `fe/agents/*/src`; classes using colours not in
  `tailwind.config.js` are silently dropped. Check the palette before using
  `amber-400`, `sky-400`, etc.
