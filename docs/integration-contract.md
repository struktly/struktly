# CLI integration contract

This document defines the noninteractive contract for programs that invoke the
Struktly CLI. Human-readable output is for people and may be reworded; JSON is
the machine interface.

## Invocation and streams

All commands accept `--root <path>` and run without prompts, and reject
positional arguments they do not define. `--root` inside a Git working tree
resolves to the repository top level for every command, including `init`,
`scan` and `suggest-instructions`, so the files one command writes are the files
another reads. Outside Git, `--root` is used literally. `context`, `status`,
`explain`, and `validate` require a Git repository. `doctor` runs anywhere and
reports what it found: it writes its report, then exits 1 if any check has
status `fail`, so a missing repository or an invalid configuration is visible
both in the document and in the exit code. `context` also
requires a commit at `HEAD`; it collects live context and does not consume
rendered scan Markdown. `brief` is a compatibility alias for `context`.

Machine modes write one JSON document to stdout:

| Invocation | Schema |
|---|---|
| `scan --json` | `struktly/snapshot/v1` |
| `context --json <request>` | `struktly/packet/v2` |
| `tasks --json` | `struktly/tasks/v1` |
| `status --json` | `struktly/status/v1` |
| `explain --json <path>` | `struktly/explanation/v1` |
| `validate --json` | `struktly/validation/v1` |
| `doctor --json` | `struktly/doctor/v1` |
| `capabilities --json` | `struktly/capabilities/v1` |
| `version --json` | `struktly/version/v1` |

In machine mode, successful diagnostics such as a generated packet path go to
stderr. With `--no-write`, successful commands leave stderr empty. stdout never
mixes prose with JSON.

`context --stdout <request>` writes Markdown to stdout and its output path to stderr.
Other default modes write plain text for developers.
`validate` checks both `.struktly/config.json` and every canonical task under
`.struktly/tasks/*.md`.

Programs should inspect `capabilities --json` before depending on additive CLI
features. The current stable feature identifiers are
`context.cancellation`, `context.expect_base_revision`, `context.limits`,
`context.no_write`, `scan.no_write`, `structured_errors`, and `tasks.partial_results`.

For side-effect-free packet generation, invoke:

```sh
struktly context --json --no-write \
  --expect-base-revision "$APPROVED_HEAD" \
  "<coding request>"
```

`--no-write` requires `--json`. It suppresses both Markdown and JSON exports
under `.struktly/`. `scan --json --no-write`
similarly returns a snapshot without writing generated files.

`--expect-base-revision <sha>` checks Git `HEAD` before and after context
selection. A mismatch fails the command instead of returning a packet for a
different revision. Callers that need repository-owned exports can omit
`--no-write`; the revision check still applies.

`context` also accepts optional limit overrides:

```sh
struktly context \
  --max-items 40 \
  --max-file-bytes 65536 \
  --max-total-bytes 524288 \
  --json --no-write \
  "<coding request>"
```

These values may only tighten the built-in packet defaults (40, 64 KiB, and
512 KiB). Overly large or non-positive values fail with a clear validation error.

## Errors and exit codes

Passing a command's `--json` flag, or the global `--json-errors` flag, emits this
document on stderr for a failed invocation:

```json
{
  "schema": "struktly/error/v1",
  "error": {
    "code": "not_git_repository",
    "message": "not a Git repository: /path"
  }
}
```

Stable error codes currently include `not_git_repository`, `repository_changed`,
`invalid_config`, `invalid_task`, `invalid_invocation`, `diagnostic_failed`,
`canceled`, and `operation_failed`. Messages are for people;
automation must branch on `error.code` and the process exit code.

| Exit | Meaning |
|---:|---|
| 0 | Operation completed; inspect structured diagnostic statuses where applicable. |
| 1 | Repository, configuration, filesystem, Git, or other operational failure. |
| 2 | Invalid command, flag, argument count, or mutually exclusive flags. |
| 130 | Operation canceled through the command context or process signal. |

SIGINT and SIGTERM cancel the root command context. Cancellation is cooperative:
Git-backed packet selection observes it, while an in-process filesystem scan
finishes its current operation before returning. A signal received during a file
replacement can leave an already-written generated artifact; callers may safely
rerun the command. `--no-write` avoids this artifact case.
The experimental MCP server currently accepts cancellation notifications but
does not interrupt an in-flight tool call. A request longer than 4 MiB is
answered with a JSON-RPC `-32600` error and a null id; the server keeps serving
subsequent requests.

## Packet determinism and versioning

`struktly/packet/v2` hashes all deterministic packet fields, including repository
identity, branch and revisions, task, sorted context items, repository guidance,
instructions, checks, warnings, exclusions, truncations, and fixed limits. Every
selected item carries its source, Git revision, selection method, confidence,
full-content SHA-256, and byte counts.
`base_revision` is the `HEAD` against which the working tree was inspected; item
hashes, rather than that revision, identify dirty and untracked file content.

`packet_hash` is `sha256:` plus the lowercase digest of the canonical JSON
serialization of that core. It excludes the hash field itself, generation time,
the legacy `metadata.absolute_git_root` field (always `.` in portable output),
presentation Markdown, and other volatile metadata. Equivalent
selected repository state and task therefore produce the same identity even when
generated at different times or checkout paths.

Within a schema version, fields are added only. Consumers must ignore unknown
fields. Breaking meaning or field changes require a new schema version. JSON
Schema files are under [`schemas/`](../schemas/).

`snapshot/v1` is deterministically ordered. Its repository root is always `.` so
portable output does not disclose a local checkout path; `generated_at` and
measured `stats.duration_ms` are intentionally volatile.

## Selection, exclusions, and limits

The context selector asks Git for tracked and non-ignored files using
`git ls-files --cached --others --exclude-standard`. Built-in selection rules are
always active; `.struktly/config.json` adds include rules and declares excludes.
Request matching tokenizes normalized relative paths and file names across
path separators, punctuation, snake/kebab boundaries, and camel case before
scoring overlap. Common words and action verbs — `add`, `fix`, `update` and
similar — are dropped, because a request names an action and a subject and only
the subject identifies code.

Go sources are additionally matched on the identifiers they declare, reported as
the `symbol_match` reason: functions and methods, method receiver types, and
named types, constants and variables. A request word must account for at least
half of an identifier's tokens to count, so `Validate` matches a validation
request and `TestMCPSurvivesAnOversizeRequest` does not match a request about
requests. The matched declarations are recorded in the item's
`provenance.location` as `declares:<names>` and reported by
`explain --json <path>`, so every symbol-driven selection can be justified.
Matching only ever adds candidates: files already excluded are never indexed, and
a repository in another language selects exactly what it selected before.
Advertised as the `context.symbol_matching` capability.

Candidates are ranked deterministically by reason and relevance, then item count
and byte limits are applied. Path and symbol evidence add up, and symbol
evidence outranks a filename match alone.

The CLI never emits the content of:

- Git-ignored files or `.git` internals;
- dependency, build, cache, generated runtime, or local state paths — a tracked
  file under one of these that would otherwise have been selected is recorded as
  a `default_excluded` exclusion rather than omitted silently;
- sensitive filenames or high-confidence detected secret content;
- symlinks, non-regular files, binary files, or invalid UTF-8.

The current fixed limits are 40 items, 64 KiB per selected text file, and
512 KiB total selected content. `max_total_bytes` is enforced against selected
content bytes, not packet JSON size. Oversized UTF-8 text is truncated on a valid
rune boundary; `content_hash` still hashes the complete source file. A truncation
names the limit that caused it: `content_limit` for the per-file limit and
`total_limit` for the packet budget.

A Go source file that does not fit is rendered as declarations rather than cut
at a byte offset: package clause, imports, types, values, and every function
signature with its doc comment, with function bodies omitted. Such an item
carries `"rendering": "declarations"`, and a consumer must branch on that field
before reading a body-less function as one that does nothing. The field is
absent for verbatim content. Rendering declarations reads and secret-scans the
whole file rather than the per-file prefix, so a file with a secret anywhere in
it is excluded rather than summarized. Files that do not parse, are not Go, or
exceed 1 MiB fall back to byte truncation. Advertised as the
`context.declaration_rendering` capability. Exclusions from item and total limits are
summarized with counts and stable reason codes in the packet, and count only
candidates that were otherwise includable — a file excluded as a secret or a
sensitive name is reported under its own reason, not as one that did not fit.
`explain --json <path>` uses the same classifier and reports `included` or
`excluded` with its reason.

`direction`, `constraints` and `decisions` carry the selected content of the
corresponding `.struktly/` file, so they are subject to the same secret scan,
per-file limit, and packet budget as any other selected item. The CLI does not
emit content it did not scan.

There is no flag to include Git-ignored or security-excluded content.

## Portable and runtime state

Portable repository declarations and user-owned guidance live under `.struktly/`.
Generated scans and packet exports also live there. Credentials and product
runtime state do not.

Portable task handoffs live under `.struktly/tasks/` and follow
[`struktly/task/v1`](task-format.md). They may name an agent, opaque session ID,
and resume command. Provider session contents and execution logs remain runtime state.

`tasks --json` emits `struktly/tasks/v1`. Safely readable declarations appear in
canonical path order under `tasks`; malformed files appear under `invalid` and
do not hide valid siblings. Each task includes the exact file SHA-256 and a
body-derived contract. A missing body contract is represented by empty fields.

Chats, executions, sessions, approvals, evidence, memory, checks, and review
history are Platform state and are never created or read by this CLI.

## Performance characteristics

Git enumeration is linear in the number of tracked and non-ignored paths. The
classifier reads only selected candidates, retains at most 512 KiB of text, and
streams complete selected files once to compute hashes. A Go candidate that
exceeds its byte budget is additionally read and parsed once, bounded at 1 MiB.
Symbol matching parses every eligible Go source once per request, bounded at
5000 files; reaching that bound is reported as a packet warning. Measured on a
147-file Go repository this adds roughly 50 ms. `scan` walks the repository
outside ignored and deprioritized directories. No context command performs a
network request or model call.
