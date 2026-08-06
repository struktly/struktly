# Changelog

Notable changes will be recorded here.

## Unreleased

- **Closed a secret-disclosure hole in `.struktly/` guidance files.**
  `direction.md`, `constraints.md` and `decisions.md` were read whole and copied
  into the packet's corresponding fields without a secret scan, while the
  selection path scans only the first 64 KiB of a file. A secret past that offset
  was neither detected nor excluded and shipped in full, while the packet's own
  truncation record for the same file reported that only 64 KiB was included.
  These fields now carry the selected item's content, so they inherit the secret
  scan, the per-file limit, and the packet budget.
- Added a secret scan to `suggest-instructions`, which excerpted the same
  guidance files into draft instruction files on disk with no check at all.
- Fixed sensitive-path matching missing directory conventions: `*secret*` could
  not match `secrets/db.txt`, because `filepath.Match` does not cross `/`. The
  Git-backed selection and the non-Git walk disagreed about the same tree; both
  now exclude, which is what the walk already did.
- Fixed the non-Git scan reading through symlinked files while reporting that
  symlinks are excluded.
- Fixed truncation caused by the packet budget being recorded as
  `content_limit`, which reads as the per-file limit; it is now `total_limit`.
  Item-limit omission counts no longer include files that could never have been
  included for another reason.
- Added `github_pat_`, Slack, Stripe and Google API token shapes to secret
  detection.
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
