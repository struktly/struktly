# Changelog

Notable changes will be recorded here.

## Unreleased

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
