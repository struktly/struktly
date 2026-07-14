# Compatibility and schema policy

Struktly is pre-1.0. Machine-readable formats use explicit schema versions.

## Schema identifiers

Every versioned context document Struktly generates carries a schema identifier:

- Markdown files: a `schema` key in the OKF frontmatter, e.g. `schema: struktly/packet/v1`.
- JSON documents: a top-level `"schema"` field, e.g. `"schema": "struktly/snapshot/v1"`.

The optional `run`, `memory`, and `evidence` record formats are experimental.
They are not versioned machine contracts yet.

Portable task Markdown under `.struktly/tasks/` uses `struktly/task/v1`; its
frontmatter and required headings are defined in [task-format.md](task-format.md).

JSON Schema definitions live in [`schemas/`](../schemas/). Current schemas are
`struktly/{snapshot,packet,config,error,status,validation,doctor,explanation}/v1`.

## Change rules

- **Within an output version**: changes are additive only (new fields, new optional
  sections). Consumers must ignore unknown fields. Repository configuration is an
  input declaration and rejects unknown keys so misspellings fail validation.
- **Breaking changes** (removing/renaming fields, changing meaning): bump the schema version (`v1` → `v2`) and document the change in [`CHANGELOG.md`](../CHANGELOG.md). Where cheap, readers tolerate the previous version for one transition release.
- **JSON is the stable machine surface.** Markdown rendering is presentation and may evolve within a schema version; do not parse markdown when a JSON form exists.
- **CLI surface**: existing commands and flags are stable and only extended additively. Commands labeled *Experimental* in `--help` may change without a major-version signal until the label is removed.
- **MCP wire names** (tools, resource URIs) are a compatibility surface once released; renames follow the breaking-change rule.

## Context-packet identity

`struktly/packet/v1` is additive: legacy v1 readers can still decode the original
fields, while current readers use `repository`, `items`, decisions, limits, and
`packet_hash`. Each item has repository-relative provenance and a SHA-256 content
hash. `packet_hash` covers all deterministic packet fields and excludes
`generated_at`, `metadata.generated_at`, the legacy
`metadata.absolute_git_root` field, the repository display name, and presentation
Markdown. The legacy field is always `.` in portable v1 output; it is retained
only to preserve the v1 shape.

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
  evidence.md             # append-only ledger
  memory/approved/        # portable, human-approved knowledge
  tasks/                  # portable task handoffs (struktly/task/v1)
```

Generated scans and packets are ignored by this repository's default Git rules;
users may export or commit them deliberately. New run records, event logs, and
pending memory candidates are runtime state outside the repository. The default
base is the OS user configuration directory under `struktly/state/`, keyed by
checkout; `STRUKTLY_STATE_DIR` overrides it. Legacy `.struktly/runs/` and
`.struktly/memory/candidates/` data is read-only compatibility input.

The complete command, stream, exit-code, and security contract is documented in
[integration-contract.md](integration-contract.md).
