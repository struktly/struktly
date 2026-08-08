# CLI scope and roadmap

Struktly CLI is the open-source context engine shared by developers, coding
agents, and the Struktly desktop app. It turns a coding request plus live Git
repository state into a deterministic, inspectable context packet.

The CLI does not own chats, provider sessions, working copies, approvals,
checks, evidence, memory, request intelligence, routing, or review history.
Those are Platform concerns.

## Command model

- `context <request>` selects the files and repository guidance relevant to one
  coding request and returns `struktly/packet/v2`.
- `scan` creates a general repository summary for people and diagnostics. It is
  optional; `context` always reads live repository state.
- `tasks` returns repository-owned task declarations as `struktly/tasks/v1`.
- `explain <path>` diagnoses the selector's decision for one file. It does not
  create context or change configuration.

## Current foundation

- Git-native repository identity and revision pinning.
- Deterministic packet identity and versioned JSON schemas.
- Explicit provenance, exclusions, truncation, and content hashes.
- Secret, binary, ignored-file, symlink, and size protections.
- Side-effect-free machine invocation and structured errors.
- Portable repository declarations and task handoffs under `.struktly/`.
- Declaration rendering for Go sources that exceed their byte budget, so a large
  file is summarized by its API rather than cut at an offset.
- Symbol matching for Go sources, so a request reaches the file declaring what it
  names rather than only the file named after it.
- Packet comparison, so a change to a repository, a configuration or the selector
  itself can be read as a list of what moved.
- Request scoping to a package or service, without changing what the packet says
  the repository is.

## Next context-quality slices

1. **Explicit seeds.** Let a caller supply reviewed starting paths alongside the
   request, with each seed recorded in packet provenance.
2. **Code-aware deterministic selection.** Symbol matching has landed behind the
   `symbol_match` reason code, with filename matching still the portable
   baseline. Import-neighbor expansion is the remaining half. Slice 3 should
   come first: symbol matching was tuned twice against measured output on real
   requests, and the untuned version selected mostly noise.
3. **Quality corpus and budgets.** Measure selection relevance, secret exclusion,
   determinism, latency, and packet size on representative repositories before
   adding caching or more heuristics. `diff` is the comparison half of this;
   what remains is a labelled corpus and recorded budgets.

Context quality work must remain inspectable and deterministic. No roadmap item
requires an LLM, network call, or proprietary service inside the CLI.
