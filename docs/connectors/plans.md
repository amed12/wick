# Plans

The `plans` connector exposes a project's planning documents over MCP: the PRD
for one phase, the features it decomposes into, and the tasks under each feature.

Features and tasks are **tickets**. The connector only owns the document and the
breakdown that creates them; an agent updates progress through the
[tickets](/connectors/tickets) connector, so the board stays the single source of
truth.

| | |
|---|---|
| Key | `plans` |
| Configs | none — plans live on wick's own disk |
| Tags | `connector`, `platform` |
| Storage | `projects/<projectID>/plans/<planID>/plan.json` |

## Operations

### `plan_list`

List a project's plans in phase order (Plan 1, Plan 2, …), each with its feature
count and task progress. `project_id` may be omitted when a calling session
exists — it defaults to that session's project.

### `plan_get`

Read one plan: the full PRD markdown, every feature with its tasks, each task's
status, acceptance criteria and dependencies. Use this before starting work.

### `plan_create`

Start the next phase. Give it a title and the user's idea. The phase number is
assigned from the highest existing number. Returns the new plan id.

### `plan_update_prd`

Replace the PRD markdown and record a version. `note` says what changed. An
unchanged document is a no-op.

### `plan_add_features`

Create the feature and task tickets from a breakdown:

```json
[
  {
    "title": "Auth",
    "body": "Daftar, login, reset password",
    "acceptance": "- [ ] bisa login",
    "tasks": [
      { "title": "Halaman login", "acceptance": "- [ ] form muncul" }
    ]
  }
]
```

Validation runs before any write; a bad payload leaves nothing behind. Creating
the same breakdown twice creates duplicate tickets, so call `plan_get` first.

### `plan_add_task`

Add one task to an existing feature of the plan — the common "add one more
thing" request after a breakdown, without re-applying the whole spec (which
would duplicate every sibling). Returns the new ticket id.

### `plan_set_status`

Move a plan between `draft`, `active` and `done`.

### `plan_context_add`

Attach source material the PRD must honour: `kind=file` with a path relative to
the project, `kind=url`, `kind=note`, or `kind=text` with the content itself.

## Access

Calls run as the caller's identity. When `project_id` is given explicitly the
connector resolves it through the same project-target check as the tickets
connector; when it is omitted the calling session's project is used. A session in
a ticket room reaches only that room's ticket, as everywhere else.
