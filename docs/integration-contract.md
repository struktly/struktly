# CLI integration contract

This document defines the noninteractive contract for programs that invoke the
Struktly CLI. Human-readable output is for people and may be reworded; JSON is
the machine interface.

## Invocation and streams

All commands accept `--root <path>` and run without prompts. `brief`, `status`,
`explain`, `validate`, and `doctor` require a Git repository. `brief` also requires
a commit at `HEAD`; it collects live context and does not consume rendered scan Markdown.

Machine modes write one JSON document to stdout:

| Invocation | Schema |
|---|---|
| `scan --json` | `struktly/snapshot/v1` |
| `brief --json <task>` | `struktly/packet/v1` |
| `status --json` | `struktly/status/v1` |
| `explain --json <path>` | `struktly/explanation/v1` |
| `validate --json` | `struktly/validation/v1` |
| `doctor --json` | `struktly/doctor/v1` |

In machine mode, successful diagnostics such as the packet output path go to
stderr. stdout never mixes prose with JSON. `run` and `memory` subcommands also
emit JSON, but their record shapes are not yet versioned schemas.

`brief --stdout <task>` writes Markdown to stdout and its output path to stderr.
Other default modes write plain text for developers.
`validate` checks both `.struktly/config.json` and every canonical task under
`.struktly/tasks/*.md`.

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

Stable error codes currently include `not_git_repository`, `invalid_config`,
`invalid_task`, `invalid_invocation`, `canceled`, and `operation_failed`. Messages are for people;
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
rerun the command.
The experimental MCP server currently accepts cancellation notifications but
does not interrupt an in-flight tool call.

## Packet determinism and versioning

`struktly/packet/v1` hashes all deterministic packet fields, including repository
identity, branch and revisions, task, sorted context items, compatibility fields,
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
Task-word filename matches can add source files. Ordering is repository-relative
lexicographic order.

The CLI never emits the content of:

- Git-ignored files or `.git` internals;
- dependency, build, cache, generated runtime, or local state paths;
- sensitive filenames or high-confidence detected secret content;
- symlinks, non-regular files, binary files, or invalid UTF-8.

The current fixed limits are 40 items, 64 KiB per selected text file, and
512 KiB total selected content. Oversized UTF-8 text is truncated on a valid rune
boundary; `content_hash` still hashes the complete source file. Exclusions and
truncations carry stable reason codes in the packet. `explain --json <path>` uses
the same classifier and reports `included` or `excluded` with its reason.

There is no flag in v1 to include Git-ignored or security-excluded content.

## Portable and runtime state

Portable repository declarations and approved knowledge live under `.struktly/`.
Generated scans and packet exports also live there. Credentials, raw provider
sessions, caches, run event logs, and pending memory do not.

Portable task handoffs live under `.struktly/tasks/` and follow
[`struktly/task/v1`](task-format.md). They may name an agent, opaque session ID,
and resume command. Provider session contents and execution logs remain runtime state.

Experimental work records, events, and pending memory candidates use a per-user directory under
the OS user configuration directory at `struktly/state/repositories/<checkout-key>`.
Set `STRUKTLY_STATE_DIR` to choose the base directory. Existing repo-local legacy
records remain readable; new writes never target them.

## Performance characteristics

Git enumeration is linear in the number of tracked and non-ignored paths. The
classifier reads only selected candidates, retains at most 512 KiB of text, and
streams complete selected files once to compute hashes. `scan` walks the repository
outside ignored and deprioritized directories. No context command performs a
network request or model call.
