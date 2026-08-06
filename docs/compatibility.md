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

JSON Schema definitions live in [`schemas/`](../schemas/). Current schemas are
`struktly/packet/v2`, `struktly/tasks/v1`, and
`struktly/{snapshot,config,error,status,validation,doctor,explanation,capabilities,version}/v1`.

Two identifiers are Markdown-only and have no JSON Schema file, because the
documents they name are presentation rather than a machine surface:
`struktly/project-context/v1` for the scan summary and
`struktly/agent-instructions/v1` for instruction drafts. Both are reported by
`capabilities --json` so a consumer can see every identifier this build emits.
The historical `packet.v1.json` remains available to verify already-pinned
packet provenance, but current binaries generate and advertise packet/v2.

### Open decision: the `task/v1` priority break

`struktly/task/v1` stopped accepting `priority: normal` without a schema bump,
which the breaking-change rule below does not permit. The change is recorded in
[`CHANGELOG.md`](../CHANGELOG.md) but the contract question is unresolved, and
this document should not pretend otherwise. Two ways to close it:

- **Bump to `struktly/task/v2` and accept `v1` for one transition release.**
  Keeps the rule below literally true. Costs a `schema:` line change in every
  existing task file, a two-version reader, a `schemas/tasks.v2.json`, and a new
  entry in the capabilities document.
- **Amend the rule with a pre-1.0 carve-out** for input declarations — formats
  Struktly reads rather than emits — allowing them to tighten within a version
  when the change is recorded in the changelog. Costs nothing in files, but
  weakens the guarantee for every future break.

Pick one before v1.0. Until then `priority: normal` fails validation.

## Change rules

- **Within an output version**: changes are additive only (new fields, new optional
  sections). Consumers must ignore unknown fields. Repository configuration is an
  input declaration and rejects unknown keys so misspellings fail validation.
- **Breaking changes** (removing/renaming fields, changing meaning): bump the schema version (`v1` → `v2`) and document the change in [`CHANGELOG.md`](../CHANGELOG.md). Where cheap, readers tolerate the previous version for one transition release.
- **JSON is the stable machine surface.** Markdown rendering is presentation and may evolve within a schema version; do not parse markdown when a JSON form exists.
- **CLI surface**: versioned machine commands and flags change only with an explicit schema or capability update.
- **Command language**: `context` is the primary name for request-scoped packet generation. `brief` remains a compatibility alias.
- **MCP wire names** (tools, resource URIs) are a compatibility surface once released; renames follow the breaking-change rule.

## Context-packet identity

`struktly/packet/v2` contains repository context only. It removed the experimental
`evidence` and `approved_memory` fields from v1 because Platform owns those
records. Each item has repository-relative provenance and a SHA-256 content hash.
`packet_hash` covers all deterministic packet fields and excludes
`generated_at`, `metadata.generated_at`, the legacy
`metadata.absolute_git_root` field, the repository display name, and presentation
Markdown. `metadata.absolute_git_root` is always `.` in portable output.

## Repository and runtime layout

`.struktly/` contains portable declarations, approved knowledge, and explicit
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
  tasks/                  # portable task handoffs (struktly/task/v1)
```

Generated scans and packets are ignored by this repository's default Git rules;
users may export or commit them deliberately. Chats, executions, evidence,
memory, and other product state belong to Platform and are outside this CLI.

The complete command, stream, exit-code, and security contract is documented in
[integration-contract.md](integration-contract.md).
