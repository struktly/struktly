# Changelog

Format: [Keep a Changelog](https://keepachangelog.com/). Struktly is pre-1.0; see [docs/compatibility.md](docs/compatibility.md) for what stability means here.

## [Unreleased]

### Added

- `struktly version` reports module version and available build metadata for bug
  reports and release verification.
- Versioned schemas for snapshots, packets, repository configuration, structured
  errors, status, validation, diagnostics, and selection explanations.
- A deterministic `struktly/packet/v1` core with Git identity, selected content,
  per-item provenance and SHA-256, explicit decisions and limits, and a stable
  packet hash.
- Strict `.struktly/config.json` declarations created by `struktly init`.
- Experimental `status`, `explain`, `validate`, and `doctor` commands with JSON modes.
- Structured CLI errors and exit-code classification.
- MCP `structuredContent` for `context_brief` while retaining Markdown text.
- Git-authoritative ignore handling plus secret, binary, symlink, and oversized-file tests.
- A validated `struktly/task/v1` Markdown contract under `.struktly/tasks/` for
  portable agent handoffs and resume metadata.

### Changed

- Portable scan and packet output uses `.` for repository-root metadata instead
  of exposing an absolute checkout path.
- New run/event state and pending memory candidates live outside repositories in
  per-user state. Legacy repo-local records remain read-only compatibility input;
  approved memory stays portable.
- Generated Markdown now includes schema frontmatter.
- README and compatibility documentation now describe only implemented behavior.

### Since v0.1.0 (shipped untagged as v0.2/v0.3 work)

- OKF v0.1 frontmatter on all generated markdown; frontmatter stripped from user files on read.
- Context packet redesign: task-word file matching, single ranked Verification Commands list, Struktly Setup section, boilerplate risks removed.
- `struktly init` (scaffolds direction/constraints + first scan), `brief --stdout`.
- Verified evidence: `evidence --run-checks` executes checks and records real exit codes.
- `struktly mcp`: stdio MCP server with `context_scan`, `context_brief`, `evidence_record`.
- `hack/demo.sh` runs the full continuity loop ending on the brief#1/brief#2 Approved Memory contrast.

## [0.1.0] — 2026-07-10

- Initial extraction from froppa/struktly.io: `scan`, `brief`, `evidence`, `suggest-instructions`, `run`, `memory`; cobra-only dependencies; MIT.
