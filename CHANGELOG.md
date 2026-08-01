# Changelog

Notable changes will be recorded here.

## Unreleased

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
