# Compatibility and schema policy

Struktly is pre-1.0. Machine-readable formats use explicit schema versions.

## Schema identifiers

Every versioned context document Struktly generates carries a schema identifier:

- Markdown files: a `schema` key in the OKF frontmatter, e.g. `schema: struktly/packet/v2`.
- JSON documents: a top-level `"schema"` field, e.g. `"schema": "struktly/snapshot/v1"`.

Portable task Markdown under `.struktly/tasks/` uses `struktly/task/v1`; its
frontmatter and required body sections are defined in
[task-format.md](task-format.md). Task discovery uses the separate
`struktly/tasks/v1` JSON document.

JSON Schema definitions live in [`schemas/`](../schemas/), one file per
identifier; that directory is the list. It was repeated here as prose and had
already fallen behind.

Not all of them are emitted. `struktly/config/v1` and
`struktly/capability-requirements/v1` are input declarations a consumer writes
and this CLI only reads, so they are published and validated but are not
reported by `capabilities --json`.

Two identifiers are Markdown-only and have no JSON Schema file, because the
documents they name are presentation rather than a machine surface:
`struktly/project-context/v1` for the scan summary and
`struktly/agent-instructions/v1` for instruction drafts. Both are reported by
`capabilities --json` so a consumer can see every identifier this build emits.

## One live version per format

Pre-1.0, exactly one version of each format is supported: the one the current
binary emits and reads. Struktly does not ship transition readers, does not keep
superseded schema files, and does not accept a format it no longer emits.

The reason is that there is nothing on the other side of the contract yet. A
version bump buys a consumer the ability to negotiate, and a retained schema
buys someone the ability to check a document they pinned earlier. With no
external consumers and nothing pinned, both cost real work — a two-version
reader to maintain, a `schema:` line to change in every existing file — and buy
nothing. Carrying them anyway would be compatibility theatre: the appearance of
a guarantee, with no one holding the other end.

So a breaking change is made in place, and the version identifier only moves
when keeping the old name would make a document's meaning ambiguous. That is
what happened to `struktly/task/v1`, which stopped accepting `priority: normal`
in place; the break is recorded in [`CHANGELOG.md`](../CHANGELOG.md) rather than
absorbed by a `task/v2`.

**This rule expires at 1.0, or at the first external consumer, whichever comes
first.** From that point a breaking change needs a new version and a transition
window, because from that point the guarantee is real.

## Change rules

- **Within an output version**: changes are additive only (new fields, new optional
  sections). Consumers must ignore unknown fields. Repository configuration is an
  input declaration and rejects unknown keys so misspellings fail validation.
- **Breaking changes** (removing/renaming fields, changing meaning): pre-1.0, made in place and documented in [`CHANGELOG.md`](../CHANGELOG.md); the version identifier moves only when the old name would otherwise be ambiguous. See the section above for when this stops applying.
- **JSON is the stable machine surface.** Markdown rendering is presentation and may evolve within a schema version; do not parse markdown when a JSON form exists.
- **CLI surface**: versioned machine commands and flags change only with an explicit schema or capability update.
- **Command language**: `context` is the primary name for request-scoped packet generation. `brief` remains a compatibility alias.
- **`intel` is outside this policy.** `struktly intel` passes its arguments to the desktop platform's own binary; its subcommands, output, and exit codes are versioned by that product, not here. The only part this repository guarantees is the handover itself: arguments and environment unchanged, the platform's exit code returned, exit 127 when the platform is not installed, and exit 126 when the binary found for it cannot be run. See [integration-contract.md](integration-contract.md).
- **MCP tool names** (`context_scan`, `context_brief`) are a compatibility surface; renames follow the breaking-change rule.

## The negotiated contract

A consumer that drives this binary as a component runs `capabilities --json`
first and refuses to start when something it needs is absent. Which entries
that covers is held by a test rather than by this document:
`TestCapabilitiesCommandReportsContextContract` in
[`cmd/struktly/main_test.go`](../cmd/struktly/main_test.go) names every command,
schema, and feature a consumer is entitled to find. Removing one fails here, at
the change that removed it, instead of in somebody else's build weeks later.

The advertised lists are also held to the binary rather than to themselves.
`TestAdvertisedCommandsExistInCobraTree` requires every advertised command to
resolve in the command tree, and every command in the tree to be advertised or
named as outside the contract. `TestAdvertisedSchemasExistAndAreEnforceable`
requires every advertised schema to be a file in `schemas/` or a declared
Markdown-only identifier, and every file to be advertised or declared
input-only. Both are in [`cmd/struktly/contract_test.go`](../cmd/struktly/contract_test.go).
The feature list is still maintained by hand;
[`.struktly/tasks/bind-features-to-proof.md`](../.struktly/tasks/bind-features-to-proof.md)
is the work that changes that.

The list is deliberately not repeated in this file. A contract written down
twice eventually disagrees with itself, and the copy without the test is the one
that goes stale.

A consumer does not have to restate that set to check it. `capabilities
--require <path>` takes a `struktly/capability-requirements/v1` document — the
consumer's own list, which this CLI never supplies a default for — and answers
with an exit code, naming every entry it cannot serve. See
[integration-contract.md](integration-contract.md).

Everything else `capabilities` reports is additive surface that may still move
pre-1.0. Ask `capabilities`; do not infer support from a version number.

## The negotiated contract

A consumer that drives this binary as a component runs `capabilities --json`
first and refuses to start when something it needs is absent. Which entries
that covers is held by a test rather than by this document:
`TestCapabilitiesCommandReportsContextContract` in
[`cmd/struktly/main_test.go`](../cmd/struktly/main_test.go) names every command,
schema, and feature a consumer is entitled to find. Removing one fails here, at
the change that removed it, instead of in somebody else's build weeks later.

The list is deliberately not repeated in this file. A contract written down
twice eventually disagrees with itself, and the copy without the test is the one
that goes stale.

A consumer does not have to restate that set to check it. `capabilities
--require <path>` takes a `struktly/capability-requirements/v1` document — the
consumer's own list, which this CLI never supplies a default for — and answers
with an exit code, naming every entry it cannot serve. See
[integration-contract.md](integration-contract.md).

Everything else `capabilities` reports is additive surface that may still move
pre-1.0. Ask `capabilities`; do not infer support from a version number.

## Context-packet identity

`struktly/packet/v2` contains repository context only. It removed the experimental
`evidence` and `approved_memory` fields from v1 because Platform owns those
records; the v1 schema file is not retained. Each item has repository-relative provenance and a SHA-256 content hash.
`packet_hash` covers all deterministic packet fields and excludes
`generated_at`, `metadata.generated_at`, the legacy
`metadata.absolute_git_root` field, the repository display name, and presentation
Markdown. `metadata.absolute_git_root` is always `.` in portable output.

## Repository and runtime layout

`.struktly/` contains portable declarations, user-owned guidance, and explicit
export artifacts:

```
.struktly/
  project-context.md      # human-readable scan result
  scans/latest.json       # machine-readable snapshot (struktly/snapshot/v1)
  context-packets/        # task-scoped packets
  agent-instructions/     # generated drafts; manually promoted
  config.json             # struktly/config/v1 selection and check declarations
  direction.md            # user-owned
  constraints.md          # user-owned
  decisions.md            # user-owned
  tasks/                  # live portable task handoffs (struktly/task/v1)
  tasks/archive/          # finished (done or canceled) task handoffs
```

Generated scans and packets are ignored by this repository's default Git rules;
users may export or commit them deliberately. Chats, executions, evidence,
memory, and other product state belong to Platform and are outside this CLI.

The complete command, stream, exit-code, and security contract is documented in
[integration-contract.md](integration-contract.md).
