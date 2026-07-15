# CLI scope and roadmap

Struktly CLI is the open-source context engine shared by developers, coding
agents, and the Struktly desktop app. It turns a coding request plus live Git
repository state into a deterministic, inspectable context packet.

The CLI does not own chats, provider sessions, working copies, approvals,
checks, or review history. Those are desktop-app concerns. The experimental
execution-era commands remain available for compatibility, but they are not the
direction of the standalone product.

## Command model

- `context <request>` selects the files and repository guidance relevant to one
  coding request and returns `struktly/packet/v1`.
- `scan` creates a general repository summary for people and diagnostics. It is
  optional; `context` always reads live repository state.
- `explain <path>` diagnoses the selector's decision for one file. It does not
  create context or change configuration.

## Current foundation

- Git-native repository identity and revision pinning.
- Deterministic packet identity and versioned JSON schemas.
- Explicit provenance, exclusions, truncation, and content hashes.
- Secret, binary, ignored-file, symlink, and size protections.
- Side-effect-free machine invocation and structured errors.
- Portable repository declarations and task handoffs under `.struktly/`.

## Next context-quality slices

1. **Packet comparison.** Explain what changed between two packet files: Git
   revision, selected files, content hashes, checks, exclusions, and packet hash.
2. **Monorepo scope.** Let callers name a repository-relative package or service
   without weakening repository identity or security rules.
3. **Explicit seeds.** Let a caller supply reviewed starting paths alongside the
   request, with each seed recorded in packet provenance.
4. **Code-aware deterministic selection.** Add language-specific import and
   symbol-neighbor expansion behind stable reason codes; keep filename matching
   as the portable baseline.
5. **Quality corpus and budgets.** Measure selection relevance, secret exclusion,
   determinism, latency, and packet size on representative repositories before
   adding caching or more heuristics.

Context quality work must remain inspectable and deterministic. No roadmap item
requires an LLM, network call, or proprietary service inside the CLI.
