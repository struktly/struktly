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
`explain`, and `validate` require a Git repository. `diff` and `verify` require
none: a packet and a Record bundle are self-describing, so each command is a
pure function of the documents it reads and works wherever they can be read. `doctor` runs anywhere and
reports what it found: it writes its report, then exits 1 if any check has
status `fail`, so a missing repository or an invalid configuration is visible
both in the document and in the exit code. `context` also
requires a commit at `HEAD`; it collects live context and does not consume
rendered scan Markdown. `brief` is a compatibility alias for `context`.

Machine modes write one JSON document to stdout:

| Invocation | Schema |
|---|---|
| `init --json` | `struktly/init-result/v1` |
| `scan --json` | `struktly/snapshot/v1` |
| `suggest-instructions --json` | `struktly/instruction-suggestions/v1` |
| `context --json <request>` | `struktly/packet/v2` |
| `tasks --json` | `struktly/tasks/v1` |
| `tasks archive --json` | `struktly/task-archive/v1` |
| `tasks complete <id> --json` | `struktly/task-transition/v1` |
| `status --json` | `struktly/status/v1` |
| `explain --json <path>` | `struktly/explanation/v1` |
| `validate --json` | `struktly/validation/v1` |
| `doctor --json` | `struktly/doctor/v1` |
| `capabilities --json` | `struktly/capabilities/v1` |
| `diff --json <before> <after>` | `struktly/packet-diff/v1` |
| `verify --json <bundle>` | `struktly/record-verification/v1` |
| `version --json` | `struktly/version/v1` |

In machine mode, successful diagnostics such as a generated packet path go to
stderr. With `--no-write`, successful commands leave stderr empty. stdout never
mixes prose with JSON.

`context --stdout <request>` writes Markdown to stdout and its output path to stderr.
Other default modes write plain text for developers.
`validate` checks both `.struktly/config.json` and every canonical task under
`.struktly/tasks/*.md`.

Programs should inspect `capabilities --json` before depending on additive CLI
features. The document that command emits is the authoritative list of them. It
used to be repeated here as prose and had already fallen behind the binary,
which is the failure mode this whole section exists to prevent.

### Requiring capabilities

A consumer that gates on this CLI — refusing to ship against a build that
cannot serve it — states what it needs once, as data, and lets the binary
answer:

```sh
struktly capabilities --require required.json
```

`required.json` is a `struktly/capability-requirements/v1` document, written by
the consumer and never emitted by this CLI:

```json
{
  "schema": "struktly/capability-requirements/v1",
  "commands": ["context", "tasks"],
  "schemas": ["struktly/packet/v2", "struktly/tasks/v1"],
  "features": ["context.no_write", "structured_errors"]
}
```

Each of the three categories is optional and unknown keys are refused. A
document that requires nothing is an invalid invocation rather than a pass,
because it would be satisfied by any build.

| Exit | Meaning |
|---:|---|
| 0 | Every required entry is advertised. |
| 1 | `capabilities_unsatisfied`, and the message names every missing entry rather than the first. |
| 2 | The flag was given without a path, or the requirements file is unreadable, malformed, or empty. Nothing was checked and nothing is written to stdout. |

The capabilities document is written on exit 0 and exit 1 alike, so a gate that
fails still records what it was given. A build advertises
`capabilities.require` when it supports being asked, so the mechanism is
detectable through the mechanism it replaces.

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

`context` and `explain` accept `--scope <dir>`, a repository-relative directory
that narrows which files the request considers:

```sh
struktly context --scope services/api --json --no-write "<coding request>"
```

Scope narrows and never widens. Repository identity, branch and revisions stay
the repository's, because a service inside a monorepo is not a separate
repository; the scope is recorded in the packet's `scope` field and is part of
packet identity, so the same request at two scopes yields two packets. Every
exclusion and security rule still applies, so naming a scope cannot admit a file
that would otherwise be refused.

Two kinds of file above the scope remain eligible, because they govern the
subtree rather than merely sitting above it: repository declarations under
`.struktly/`, and agent instruction files in an ancestor directory. They keep
their own selection reasons, so the packet already says why they are present.
A scope that is not a directory inside the repository fails with
`invalid_invocation`. `explain --scope` reports `out_of_scope` for a path outside
it. Advertised as the `context.scope` capability.

`context` accepts `--seed <path>`, repeatable, naming files the caller already
knows are relevant:

```sh
struktly context --seed internal/http/router.go --json --no-write "<request>"
```

A seed is the one selection reason the CLI does not derive, and it is checked
hardest for that reason. Naming a file gets it considered, never included: every
exclusion still applies, so a seed pointing at a sensitive filename or a
detected secret is refused and recorded like any other candidate. "Reviewed"
describes the caller's judgement about relevance, not a claim that a file is
safe to disclose.

Seeds are canonicalized, deduplicated and recorded in the packet's `seeds`
field whether or not each survived selection, so a caller can distinguish a seed
that was excluded from one that was never requested. They are part of packet
identity. They outrank every derived reason, so a tight `--max-items` spends
itself on what the caller named first. At most 40 may be given. A seed that is
not a file inside the repository, or that falls outside `--scope`, fails with
`invalid_invocation`; a directory seed is refused with a pointer to `--scope`.
Advertised as the `context.seeds` capability.

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
`invalid_config`, `invalid_task`, `invalid_packet`, `invalid_invocation`,
`diagnostic_failed`, `verification_failed`, `capabilities_unsatisfied`,
`tasks_unarchived`, `task_not_found`, `task_already_archived`, `canceled`, and
`operation_failed`. Messages are for people; automation must branch on
`error.code` and the process exit code.

| Exit | Meaning |
|---:|---|
| 0 | Operation completed; inspect structured diagnostic statuses where applicable. |
| 1 | Repository, configuration, filesystem, Git, or other operational failure. |
| 2 | Invalid command, flag, argument count, or mutually exclusive flags. |
| 126 | `intel` only: a binary was found for the platform but cannot be run. |
| 127 | `intel` only: the Struktly desktop platform is not installed, so there was nothing to drive. |
| 130 | Operation canceled through the command context or process signal. |

SIGINT and SIGTERM cancel the root command context. Cancellation is cooperative:
Git-backed packet selection observes it, while an in-process filesystem scan
finishes its current operation before returning. A signal received during a file
replacement can leave an already-written generated artifact; callers may safely
rerun the command. `--no-write` avoids this artifact case.
The MCP server currently accepts cancellation notifications but does not
interrupt an in-flight tool call. A request longer than 4 MiB is
answered with a JSON-RPC `-32600` error and a null id; the server keeps serving
subsequent requests.

## Driving the desktop platform

`struktly intel [arguments...]` is a pass-through to the headless entrypoint of
the installed Struktly desktop app, and is the one command in this CLI whose
output is not this CLI's. Arguments are not parsed, interpreted, or defaulted:
the resolved `intel` binary is given them verbatim along with the process
environment, and its exit code is returned unchanged. On unix the process is
replaced with `exec(2)`, so nothing sits between the caller and the platform.

Consequences a caller should rely on:

- **The output contract belongs to the platform.** Its subcommands, its JSON,
  its schema identifiers, and its exit codes are versioned by the desktop
  product and documented there. Nothing under `schemas/` describes them, and
  `capabilities --json` does not advertise `intel`, because this CLI cannot
  make promises about a program it does not contain.
- **`--root` and `--json-errors` do not apply.** They are the CLI's flags; after
  `intel` every argument is the platform's, including `--json` and `-h`.
- **Exit 127 means the platform is absent.** The binary is resolved as
  `$STRUKTLY_INTEL`, then `intel` beside the running `struktly` executable, then
  the platform's known install location (on macOS,
  `/Applications/Struktly.app/Contents/MacOS`), then `intel` on `PATH` — an
  ambiguous source last, because the resolved binary receives the caller's
  arguments and environment. If none resolves, one sentence is written to stderr — never
  a `struktly/error/v1` document, even when `--json` appears in the arguments —
  and the command exits 127 without running anything. Any other exit code came
  from the platform, whose ladder is documented with the binary itself — the
  package comment of `cmd/intel/main.go` in the platform repository — and is
  deliberately not restated here, because a copy of a contract this repository
  does not own would rot against the product it describes. 127 is chosen because
  it is outside the 0-4 ladder the platform documents, and because it is the
  shell's own code for a command that does not exist. A resolved binary that
  cannot be executed exits 126 and names the path; neither case produces a
  `struktly/error/v1` document.

This command does not weaken the boundary the rest of this document describes:
the CLI still calls no model, links no platform code, and speaks to no platform
process. It locates a binary and gets out of the way.

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
Schema files are under [`schemas/`](../schemas/), and every document this CLI
emits is validated against them in the test suite, so a schema and the output it
describes cannot drift apart unnoticed.

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

Markdown documents are matched on their first heading, reported as the
`title_match` reason. A filename is a guess at a document's subject and often a
bad one — a decision record called `0001-record.md` is titled "ADR 0001: Record
architecture decisions" — so the title is both better evidence and something
`explain` can quote back. A title must carry at least two distinct request words
to count; the matched title is recorded in the item's `provenance.location` as
`titled:<heading>`. Advertised as the `context.title_matching` capability.

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

After the request's own candidates are selected, a second pass adds repository
files that the selected code calls, reported as the `import_neighbor` reason
with the supplied identifiers in `provenance.location` as `provides:<names>`.
The unit is the identifier, not the package: `files.CleanRoot` reaches whichever
file declares `CleanRoot` and leaves its siblings alone, because a Go import
names a directory and a directory is not a unit of relevance. Only first-degree
imports, only from files the request matched by name or the caller seeded, and
only into budget the request itself did not use — an import neighbour is a
reason to look rather than evidence about the request, so it never displaces a
direct match. Scope and every exclusion still apply. Advertised as the
`context.import_neighbors` capability.

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

## Comparing packets

`diff <before.json> <after.json>` reports what changed between two
`struktly/packet/v2` documents: packet hash, repository identity and revisions,
limits, selected items added, removed or changed, required and suggested checks,
exclusions and truncations. A changed item names the fields that moved —
`content_hash`, `reason`, `rendering`, byte counts, and the matched declarations
in `provenance.location`.

The diff names what was selected and why and never reproduces file content, so
comparing two packets cannot disclose what reading either of them would not.
Both inputs must declare `struktly/packet/v2`; anything else fails with
`invalid_packet`.

## Verifying a Record

`verify <bundle.json>` checks an exported `struktly/record-bundle/v1` document
without any other Struktly component. It validates the bundle against the
published schema, re-derives the SHA-256 of the sealed bytes and compares it
with the digest recorded when they were sealed, and confirms the manifest and
the sealed Record describe the same revision. `--json` emits
`struktly/record-verification/v1`.

The report is written whether or not the bundle verifies; the exit code then
says what it found. Exit 0 means every check passed. Exit 1 with
`verification_failed` means a check failed, and the report names which. Exit 2
means the bundle could not be read or is not JSON, and nothing was checked.

What an intact result proves is exact and narrow — the bytes are the ones the
digest describes — and the report's `unverifiable` list says what it does not:
never whether the work was correct, and never anything the Record itself
states it could not capture. Judgements carried inside an intact payload are
reported, not re-evaluated, and three states stay apart: a list that is `null`
was not available, an empty list was read and found empty, and a list with
entries is what the payload carried.

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

Task lifecycle transitions are CLI operations, and none of them requires a Git
repository. [task-format.md](task-format.md) states the location invariant they
enforce: the live `.struktly/tasks/` directory carries no `done` or `canceled`
task, finished tasks live under `archive/`, and frontmatter wins when the two
disagree.

`tasks complete <id>` resolves `<id>` against the frontmatter `id` of live
tasks, sets `status: done` and today's `updated` date, files the task under
`archive/`, and repairs Markdown links in both directions — links out of the
moved task and links into it, plus repository-root path citations in Markdown
and Go sources. The ordering is part of the contract: inbound repairs are
written first, then the completed content lands at the archive path, and the
live file is removed only after that write succeeds, so a failure at any step
leaves the original live file intact and rerunning the command converges. An
unknown id fails with `task_not_found`; an id already filed under `archive/`,
or whose archive slot is occupied, fails with `task_already_archived`. `--json`
emits `struktly/task-transition/v1`; the `transition` field names the
operation so future transitions can share the schema.

`tasks archive` files every already-finished task that is misfiled in the live
directory, with the same link repair — the migration and cleanup case.
`tasks archive --check` is the conformance gate: it writes its report and then
exits 1 with `tasks_unarchived` while the invariant is violated, so CI can
branch on it. `--json` emits `struktly/task-archive/v1` in both modes; combined
with `--check` the document reports what a mutating run would do, without
writing any of it. Reserved OKF names (`index.md`, `log.md`) are never treated
as tasks or moved.

Chats, executions, sessions, approvals, evidence, memory, checks, and review
history are Platform state and are never created or read by this CLI.

## Performance characteristics

Git enumeration is linear in the number of tracked and non-ignored paths. The
classifier reads only selected candidates, retains at most 512 KiB of text, and
streams complete selected files once to compute hashes. A Go candidate that
exceeds its byte budget is additionally read and parsed once, bounded at 1 MiB.
Content matching reads every eligible Go source and Markdown document once per
request, bounded at 5000 files in total; reaching that bound is reported as a
packet warning. Measured on a 147-file Go repository this adds roughly 50 ms,
and title extraction reads only the first 8 KiB of a document. `scan` walks the repository
outside ignored and deprioritized directories. No context command performs a
network request or model call.
