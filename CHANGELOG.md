# Changelog

Notable changes will be recorded here.

## Unreleased

- **Changed `struktly/task/v1` validation without bumping the schema version.**
  `priority: normal` is no longer accepted; the ladder is `low`, `medium`,
  `high`, `critical`. This shipped unrecorded and is a breaking change for any
  task file that used `normal`. Whether it should instead have become
  `struktly/task/v2` is an open compatibility decision; see below.
- Relaxed `struktly/task/v1` so `priority`, `created` and `agent` are optional,
  and so unknown frontmatter keys are preserved under `extensions` rather than
  rejected. Both widen what validates and break no existing valid file.
- Replaced the six fixed required body headings with two required sections —
  an objective and a done-condition — each accepting the spellings repositories
  actually use. Measured against the 58-file task corpus in the Platform
  repository, the previous rule rejected 56 of them.
- Fixed `validate` rejecting the `index.md` and `log.md` names OKF v0.2 reserves,
  and made it report every invalid task file instead of only the first.
- Fixed emitted task JSON carrying `"priority": ""`, `"created": ""` and
  `"agent": ""`, which `schemas/tasks.v1.json` rejects.
- Improved context selection with token-aware path ranking (including camel-case
  matching) and caller-tightened packet limits with deterministic aggregated
  truncation exclusions.

## v0.2.0 - 2026-08-01

- Added `tasks --json` with exact task hashes, parsed contracts, partial invalid
  results, explicit compatibility notes, and the `struktly/tasks/v1` schema.
- Moved current context generation to `struktly/packet/v2`, removing experimental
  evidence and approved-memory fields while retaining the historical v1 schema.
- Removed the experimental `run`, `memory`, and `evidence` commands and the MCP
  `evidence_record` tool; Platform owns those product-state concerns.

## v0.1.2 - 2026-07-16

- Added side-effect-free JSON generation for context packets and scans.
- Added approved-revision checks with structured `repository_changed` errors.
- Added a versioned CLI capabilities document.
- Made `context` the primary packet command while preserving `brief` as an alias.

## v0.1.1 - 2026-07-15

- Initial public release of the local repository-context CLI.
- Deterministic Markdown and versioned JSON context packets.
- Git-aware selection, secret and binary exclusions, and portable packet hashes.
- Structured status, explanation, validation, diagnostics, and error output.
- MCP tools for scanning, context selection, and optional evidence records.
