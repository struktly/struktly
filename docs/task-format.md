# Portable task format

A portable task handoff is a Markdown file under `.struktly/tasks/`. Each file
is named `<id>.md` and identified by `schema: struktly/task/v1`.

These files are repository-owned instructions, not live execution records. They
may identify an agent session and a resume command, but they must not contain
credentials, chat history, caches, logs, or high-volume runtime events. Live
state remains outside the repository.

## Canonical shape

```markdown
---
type: task
schema: struktly/task/v1
id: add-request-timeout
title: "Add request timeout middleware"
status: ready
priority: medium
created: 2026-07-13
---

# Add request timeout middleware

## Objective

Describe the concrete outcome.

## Required outcomes

- [ ] List observable deliverables.

## Non-goals

- List what this task deliberately does not do.

## Definition of done

State the checks and evidence required before completion.
```

Add whatever else the task needs — constraints, an execution plan, a pickup
note, background — under any heading you like. Those sections are yours; the
format does not prescribe them.

## Frontmatter contract

| Field | Requirement |
|---|---|
| `type` | Required; exactly `task`. |
| `schema` | Required; exactly `struktly/task/v1`. |
| `id` | Required; lowercase letters, digits, and single hyphens; must match the filename. |
| `title` | Required, non-empty human title. |
| `status` | Required; `draft`, `ready`, `in-progress`, `blocked`, `done`, or `canceled`. |
| `priority` | Optional; `low`, `medium`, `high`, or `critical`. |
| `created` | Optional ISO date (`YYYY-MM-DD`). |
| `updated` | Optional ISO date. |
| `agent` | Optional agent name. |
| `agent_model` | Optional model name recorded for handoff continuity. |
| `reasoning_effort` | Optional agent reasoning configuration. |
| `agent_session` | Optional opaque session identifier; requires `resume_command`. |
| `resume_command` | Optional single-line pickup command; requires `agent_session`. |

Required is what makes a file a task at all: what it is, which contract it
speaks, its identity, what it is called, and where it stands. The rest describes
a task rather than constituting one, so an unranked or unassigned task is an
ordinary state rather than an error.

The v1 frontmatter is deliberately flat so Struktly can validate it with the Go
standard library. Quote values containing spaces with JSON-style double quotes.
Unknown fields are preserved under `extensions` rather than rejected, as OKF
v0.2 §4.1 requires; a repository can annotate its own tasks with keys this
parser does not define.

## Body contract

A task body needs two sections, in any order, with any other prose around them:

| Section | Accepted headings |
|---|---|
| Objective | `## Objective`, `## Mission`, `## Outcome` |
| Required outcomes | `## Required outcomes`, `## Success`, `## Success criteria`, `## Acceptance criteria`, `## Done when`, `## Definition of done`, `## Requirements` |

The first heading in each row is canonical; the others are recorded in
`compatibility_notes` so a reader can see which heading a value came from.
`## Non-goals` populates `non_goals` when present. Code spans under
`## Definition of done`, `## Verification`, or `## Required checks` become
`required_checks`.

## Validation and context

`struktly validate` checks every `.struktly/tasks/*.md` file in addition to
`.struktly/config.json`, and reports every invalid file rather than stopping at
the first. It skips the `index.md` and `log.md` names OKF v0.2 §8 and §9 reserve
for a directory listing and an update history. A brief may include a task file
when words from the requested task match its filename. Included task files
appear in the packet with their path, selection reason, and content hash.

`struktly tasks --json` provides tolerant machine discovery as
`struktly/tasks/v1`. It reports valid siblings even when another file is
malformed, includes the SHA-256 of each task's exact bytes, and maps body sections
to `outcome`, `done_when`, `non_goals`, and `required_checks`. Historical body
headings remain discoverable with explicit `compatibility_notes`; strict
`validate` additionally requires the two body sections above. Discovery also preserves
historical dotted IDs with a compatibility note while strict validation rejects
them. Other invalid frontmatter, unsafe files, and unsupported statuses appear
under `invalid` rather than being silently normalized.

Within `struktly/task/v1`, required field and heading meanings are stable. Additive
format changes may introduce optional fields or sections. Breaking changes require
a new task schema version.
