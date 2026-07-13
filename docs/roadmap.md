# CLI roadmap

This is the authoritative roadmap for `github.com/struktly/struktly`.

## Current focus

- Keep the CLI installable with `go install` and useful offline in any Git
  repository.
- Stabilize documented JSON schemas, CLI stream behavior, and MCP tool names.
- Improve deterministic context selection, diagnostics, and cross-platform
  coverage.
- Make portable `.struktly/` declarations easy to review and safe to hand off
  between coding-agent sessions.

## Deliberately out of scope

The repository does not plan a hosted service, GUI, worktree manager, terminal,
agent runner, provider integration, or private-product implementation. Those are
separate products and are not prerequisites for using or contributing to this CLI.

Proposals should start with an issue describing the user-facing CLI contract,
compatibility impact, and the smallest useful change.
