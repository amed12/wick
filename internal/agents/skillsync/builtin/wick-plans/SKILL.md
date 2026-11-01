---
name: wick-plans
description: Use when the session's prompt names a plan (P-XXXX), when the user talks about a PRD, a roadmap, features, phases, or "what should I build next" — or when work needs a written goal before it starts. Covers the plan → feature → task model, how the plans connector turns a PRD into tickets, and how to keep progress honest by updating the ticket you are actually working on.
---

# Plans

A **plan** is one phase of a project: a PRD, the features it decomposes into, and
the tasks under each feature. The plan is the "why and what"; the tickets are the
"who is doing it and how far along".

Use the `plans` connector (`plan_list`, `plan_get`, `plan_create`,
`plan_update_prd`, `plan_add_features`, `plan_set_status`, `plan_context_add`).
The tasks and features it creates are ordinary tickets — work them through the
`tickets` connector, not this one.

## Read before you write

1. `plan_get` — the full PRD, every feature, each task's status, acceptance
   criteria and dependencies. Your prompt carries an excerpt of the plan and the
   ticket's done-when contract; the connector answer is the source of truth.
2. Pick a task whose dependencies are done and whose status is not terminal.
3. `ticket_get` on that task before touching code. The plan says what, the ticket
   says where it stands.

Never re-plan work that already exists. Creating the same breakdown twice creates
duplicate tickets — check `plan_get` first.

## Writing a plan

- `plan_create` starts the next phase. Give it the user's goal in their own
  words; the phase number is assigned for you.
- `plan_update_prd` replaces the document and records a version. Keep the
  structure the workspace expects: Overview, Goals, Non-goals, Users,
  Requirements, Core features, User flow, Architecture, Data model, Risks.
  Concrete beats long — an agent has to act on it.
- If the project has ticket mode off, the work exists but the board is hidden.
  Turn it on first with the tickets connector (`ticket_settings_set`,
  `enabled=true`) so the user can see what the plan produced.
- `plan_add_features` is the only step that creates work. `features` is a JSON
  array:

  ```json
  [{"title":"Auth","acceptance":"- [ ] login works","tasks":[{"title":"Login page","acceptance":"- [ ] form submits"}]}]
  ```

  Validation runs first: a bad payload leaves nothing behind.
- `plan_add_task` appends one task to an existing feature. Use it for a late
  addition; never re-run `plan_add_features` for one task.
- `plan_context_add` attaches the material the PRD must honour (a file path, a
  URL, a note id, or the text itself). Context is what makes a PRD specific
  instead of generic.

## Keep progress honest

Progress is computed from ticket status, not typed into the plan. The only way
to move a feature's bar is to move its tasks:

- `ticket_update` with `status` when you start, when you are blocked, and when
  you are done. Do not mark done because the code compiles — mark done when the
  acceptance checklist is actually true.
- Leave a note (`notes_add`) for anything a later session would have to
  rediscover.
- When every task in the phase is done, ask before opening the next phase:
  `plan_set_status` to `done`, then `plan_create` for what comes next.

## When there is no plan

If the user asks for something substantial and no plan exists, say so and offer
to write one first. A one-line fix does not need a PRD; a feature that spans
several files does. Do not invent a plan for a typo.
