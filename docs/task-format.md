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
priority: normal
created: 2026-07-13
agent: unassigned
---

# Add request timeout middleware

## Pick up this task

State how a person or agent should begin. When the task resumes an existing
agent session, add `agent_session` and `resume_command` to the frontmatter and
repeat the command here.

## Objective

Describe the concrete outcome.

## Constraints

- List boundaries that must remain true.

## Required outcomes

- [ ] List observable deliverables.

## Execution plan

1. Implement one coherent slice.
2. Verify it.

## Definition of done

State the checks and evidence required before completion.
```

## Frontmatter contract

| Field | Requirement |
|---|---|
| `type` | Required; exactly `task`. |
| `schema` | Required; exactly `struktly/task/v1`. |
| `id` | Required; lowercase letters, digits, and single hyphens; must match the filename. |
| `title` | Required, non-empty human title. |
| `status` | Required; `draft`, `ready`, `in-progress`, `blocked`, `done`, or `canceled`. |
| `priority` | Required; `low`, `normal`, `high`, or `critical`. |
| `created` | Required ISO date (`YYYY-MM-DD`). |
| `updated` | Optional ISO date. |
| `agent` | Required; agent name or `unassigned`. |
| `agent_model` | Optional model name recorded for handoff continuity. |
| `reasoning_effort` | Optional agent reasoning configuration. |
| `agent_session` | Optional opaque session identifier; requires `resume_command`. |
| `resume_command` | Optional single-line pickup command; requires `agent_session`. |

The v1 frontmatter is deliberately flat so Struktly can validate it with the Go
standard library. Quote values containing spaces with JSON-style double quotes.
Unknown fields are rejected so misspellings fail early.

## Validation and context

`struktly validate` checks every `.struktly/tasks/*.md` file in addition to
`.struktly/config.json`. A brief may include a task file when words from the
requested task match its filename. Included task files appear in the packet with
their path, selection reason, and content hash.

Within `struktly/task/v1`, required field and heading meanings are stable. Additive
format changes may introduce optional fields or sections. Breaking changes require
a new task schema version.
