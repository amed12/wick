# Plans

A **plan** is one phase of a project: the PRD, the features it decomposes into,
and the tasks under each feature. It is the surface where an idea becomes work an
agent can pick up.

Open it at **Plans** in the Agents sidebar (`/tools/agents/plan`).

## The model

```
Project
└── Plan 1  (this phase's PRD)
    ├── Feature  → ticket, kind=feature
    │   ├── Task → ticket, parent=feature
    │   └── Task
    └── Feature
```

Features and tasks are **ordinary tickets**. That is deliberate: the board, notes,
sessions, webhooks and MCP tools keep working unchanged, and progress is a query
over ticket status rather than a second source of truth that can drift. Deleting
a plan leaves its tickets on the board — deleting a document must not erase work
in flight.

## Prerequisites

Generation runs through the shared `aigen` queue, so it borrows a normal agent
pool slot and needs a **structured-output provider**. Set one on the Providers
page; without one, Generate reports the provider error instead of silently
failing. Everything else (writing the PRD by hand, breaking it into features,
the board, the roadmap) works with no provider at all.

## The loop

1. **Create a plan** — give it a title and the idea in your own words.
2. **Add context** (optional) — a file path, a URL, a note id, or pasted text.
   This is what the generator reads; it is what makes a PRD specific.
3. **Generate** — one call produces the PRD and the feature/task breakdown.
   Review the draft, then apply it.
4. **Roadmap** — each feature is a node with its task progress. Click one to read
   the tasks, acceptance criteria and the done-when contract.
5. **Execute** — "Kerjakan dengan agent" opens the board on that ticket. The agent
   starts a session there, reads the plan excerpt injected into its prompt, and
   updates the ticket as it works.
6. **Close the phase** — when every task is done, set the plan to `done` and
   start the next one from what shipped.

## Editing the PRD

Preview and Edit markdown are the same document. Saving records a **version**;
an unchanged document is a no-op, so autosaving does not fill the history.
Versions live in the plan record (`versions[]`), newest last, capped at 50.

## What the agent sees

When a session is attached to a ticket that belongs to a plan, its system prompt
gains:

- the phase (`Plan N — title`) and the plan summary;
- a bounded excerpt of the PRD, with a pointer to the `plans` connector for the
  full document;
- the ticket's acceptance criteria.

That is the whole "no copy-pasted prompt" idea: the agent is already inside the
plan. The `plans` connector lets it read the rest, and the `tickets` connector is
how it reports progress back.

## Agent access

The `plans` connector is registered at boot and gated like every other connector
by the Team feature scope. Ops:

| Op | What it does |
|---|---|
| `plan_list` | phases with feature counts and task progress |
| `plan_get` | full PRD + feature/task tree |
| `plan_create` | start the next phase |
| `plan_update_prd` | replace the document, record a version |
| `plan_add_features` | create feature + task tickets |
| `plan_add_task` | add one task to an existing feature |
| `plan_set_status` | draft / active / done |
| `plan_context_add` | attach a source item |

`plan_add_features` is marked destructive because it creates tickets. It is off
by default on every connector row; enable it for an agent you trust to plan.

## Storage

```
<data>/agents/projects/<projectID>/plans/<planID>/plan.json
```

A plan id is a short, quotable code (`P-7QK2`). The phase number is assigned from
the highest existing number, so plans read 1, 2, 3 in the order they were started.
