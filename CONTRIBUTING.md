# Contributing

Thanks for your interest. Struktly is in alpha; small, focused contributions land fastest.

## Ground rules

These are product invariants, not style preferences. PRs that break them will be declined:

1. **No model calls.** Every command must be deterministic and work offline. Output is derived from the filesystem only.
2. **Explicit state boundary.** Portable declarations and approved knowledge are
   written under `.struktly/`. Per-user runtime state (runs, pending memory,
   caches, logs, and credentials) stays outside the repository. Struktly never
   modifies a user's source code or active instruction files (`AGENTS.md`,
   `CLAUDE.md`, or Cursor rules).
3. **Stdlib-first.** The only external dependency is cobra. New dependencies need a strong justification.
4. **No Struktly-internal content in generated output.** Cold-repo leak-guard tests (`TestBriefColdRepoOmitsStruktlyInternalContent`, `TestSuggestInstructionsColdRepoOmitsStruktlyInternalContent`) enforce this — repo-specific content comes from the scanned repo's own `.struktly/` files, never from Go string literals.

## Workflow

```sh
make lint   # gofmt + go vet
make test   # go test ./...
make build
```

All three must pass before a PR. CI runs the same checks.

This repo dogfoods itself: read `.struktly/direction.md`, `.struktly/constraints.md`, and `docs/roadmap.md` before proposing features — the Non-goals list is load-bearing. If you use an AI agent to contribute, run `struktly scan && struktly brief "<your task>"` first and hand it the packet.

## Reporting issues

For scan/brief quality problems, include the repo (or a minimal reproduction of its layout) and the generated `.struktly/project-context.md` or context packet — the output is deterministic, so that's a complete reproduction.

For security vulnerabilities, do not open a public issue; use the private route
in [SECURITY.md](SECURITY.md). All project interactions follow the
[Code of Conduct](CODE_OF_CONDUCT.md).
