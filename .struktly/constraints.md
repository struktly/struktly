---
type: constraints
title: "Struktly Constraints"
description: "Repo-owned constraints excerpted into every struktly context packet."
timestamp: 2026-07-10T18:30:00Z
---

# Struktly Constraints

- Local-first: no command requires an account, server, model provider, or network call.
- Deterministic: context output is derived from local files and Git metadata.
- Portable declarations, approved memory, evidence, and task handoffs belong under
  `.struktly/`; sessions, credentials, caches, logs, and pending runtime state do not.
- Never modify source files or active agent instructions. Suggested instruction
  drafts are the only instruction output and require manual promotion.
- Respect Git ignores and exclude secrets, binaries, symlinks, dependencies,
  build output, and runtime state from generated context.
- Keep dependencies stdlib-first. Cobra is the current command dependency; a new
  runtime dependency needs a documented justification.
- Versioned machine formats follow `docs/compatibility.md`. Preserve existing
  behavior additively within a schema version.
- Keep the CLI independently installable and useful without another repository.

## Non-goals

- Agent execution or provider integration.
- A terminal, worktree manager, desktop/server binary, web UI, HTTP API, or hosted service.
- Policy engines, approvals, packs, code generation, or opaque runtime persistence.
