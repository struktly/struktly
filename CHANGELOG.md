# Changelog

Notable changes will be recorded here.

## Unreleased

- **Added declaration rendering for oversized Go files.** A Go source file that
  does not fit its byte budget is now included as its declarations — package
  clause, imports, types, values, and every function signature with its doc
  comment — instead of being cut at a byte offset. Measured on this
  repository's own sources the skeleton is 69% to 82% smaller than the file, so
  files that previously arrived as a truncated fragment now arrive complete.
  Such items carry `"rendering": "declarations"` in `struktly/packet/v2`, which
  a consumer must check before reading a body-less function as one that does
  nothing, and the capability is advertised as
  `context.declaration_rendering`. Rendering reads and secret-scans the whole
  file, so a secret anywhere in it excludes the file rather than summarizing
  it. Files that do not parse fall back to byte truncation.
- **Changed `doctor` to report failures instead of refusing to run.** Its
  `git_repository` check could only ever report "pass", because the command
  returned an error before producing a report whenever the repository would not
  resolve. It now writes its report anywhere and exits 1 if any check failed,
  which also fixes an invalid configuration being reported with exit 0.
  The new error code is `diagnostic_failed`.
- Removed the retained `schemas/packet.v1.json`. It existed to verify
  already-pinned packet provenance, which nothing has pinned, and it was the
  last place carrying a superseded format alongside the live one.
- Added a `schema` field to `version --json` (`struktly/version/v1`, with a
  JSON Schema file); it was the only machine output without one.
- Added the missing identifiers to `capabilities --json`: the `init`, `mcp`,
  `suggest-instructions` and `version` commands, and the Markdown-only
  `struktly/project-context/v1` and `struktly/agent-instructions/v1` schemas.
- Fixed tracked files under `build/` or `dist/` being dropped from a packet with
  nothing recording it; they are now reported as `default_excluded`.
- Fixed excerpt truncation splitting multi-byte UTF-8 runes.
- Pinned `govulncheck` to v1.6.0, the last unpinned supply-chain input in
  otherwise SHA-pinned CI.
- The release workflow now fails when `GITLEAKS_LICENSE` is unset instead of
  silently skipping the secret history scan. Publishing without it requires the
  explicit `allow_missing_secret_scan` input, and the release notes record it.
- Git-ignored the generated `.struktly/project-context.md` and
  `.struktly/agent-instructions/`, and dropped stale ignore entries for the
  `run` and `memory` features removed in v0.2.0.
- Added `status` to the README command table.
- **Fixed the exit-code contract for invalid invocations.** An invalid flag
  value such as `--max-items abc` or `--json=bogus` exited 1 as
  `operation_failed` where the contract promises 2 and `invalid_invocation`.
  Classification now uses typed errors rather than searching the message text.
- **Changed `init`, `scan` and `suggest-instructions` to anchor `--root` at the
  Git repository top level**, matching every other command. In a monorepo,
  `init --root services/api` previously wrote a configuration that
  `context --root services/api` never read. Directories outside Git are
  unaffected.
- Fixed `init`, `scan`, `suggest-instructions` and `mcp` silently accepting
  stray positional arguments: `struktly scan repo-b` scanned the working
  directory. They now exit 2 like their siblings.
- Fixed root-anchored `.gitignore` patterns such as `/generated` being dead in
  the non-Git scan, which claimed to apply root-level patterns.
- Fixed an over-broad MCP request killing the server with no JSON-RPC error;
  an oversize message is now answered and the server keeps serving.
- Fixed a config file that cannot be read being reported as `invalid_config`.
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
- **`struktly/task/v1` no longer accepts `priority: normal`;** the ladder is
  `low`, `medium`, `high`, `critical`. This is a breaking change for any task
  file that used `normal`, and it shipped unrecorded. It stays inside `task/v1`
  rather than becoming `task/v2`: pre-1.0 with no external consumers, Struktly
  supports exactly one version of each format and makes breaking changes in
  place. See [`docs/compatibility.md`](docs/compatibility.md), which now states
  that rule and when it expires.
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

## v0.2.1 - 2026-08-04

- Bounded and ranked context packets: token-aware path ranking including
  camel-case matching, caller-tightened `--max-items`, `--max-file-bytes` and
  `--max-total-bytes` overrides, and deterministic aggregated truncation
  exclusions. This release shipped without a changelog entry; recorded here
  after the fact.

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
