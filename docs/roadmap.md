# CLI scope and roadmap

Struktly CLI is the open-source context engine shared by developers, coding
agents, and the Struktly desktop app. It turns a coding request plus live Git
repository state into a deterministic, inspectable context packet.

The CLI does not own chats, provider sessions, working copies, approvals,
checks, evidence, memory, request intelligence, routing, or review history.

Most of those are Platform concerns. Several are no longer anybody's single
product: the deterministic decision engine that classifies a request, admits
capabilities and chooses a rung is now its own component, and so is the local
inference runtime that owns model placement, supervision and prefix reuse. Both
are separate closed components that Platform consumes rather than owns. Whether
either is ever published is an open question, and a different one from the
extraction that has already happened.

None of that moves this repository's boundary, and one part of it is worth
saying plainly now that there are more components nearby to reach for: this is
the only published module in the set, so it depends on none of the others.
`struktly intel` remains a process handover to the installed desktop platform,
no implementation is duplicated here, and `boundary_test.go` fails the build on
an import of an unpublished sibling rather than leaving it for whoever next runs
`go install`.

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
- A boundary held by tests rather than by prose: no package here imports the
  network, no dependency brings one in unrecorded, and the installed command
  links cobra and what cobra brings.
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
- Caller-supplied seed paths, which outrank derived reasons and are still subject
  to every exclusion.
- A labelled selection corpus with a recorded report, per-case determinism, and
  schema conformance for every emitted document.
- Document title matching, so a file whose name has drifted from its subject is
  still reachable by what it says it is about.
- Import-neighbor expansion, which follows the identifiers selected code calls
  rather than the packages it imports.

## Next context-quality slices

The recorded context-quality slices are done. Further work should start by
adding cases to the selection corpus, since every slice above changed shape
after being measured against it.

Context quality work must remain inspectable and deterministic. No roadmap item
requires an LLM, network call, or proprietary service inside the CLI.
