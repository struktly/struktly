# Struktly

**The local, Git-native control plane for coding-agent work.**

Struktly is a standalone CLI that inspects a Git repository and builds durable,
reviewable context for coding agents. It uses local files and Git metadata only:
no account, GUI, server, model provider, or network call is required.

It works *alongside* Claude Code, Codex, and Cursor. It is not another agent.

Repository boundary: this repository owns the open-source CLI and its stable
formats. It is self-contained; no other Struktly repository is required to use
or contribute to it.

## What it does

Struktly keeps repository context inspectable and reusable across agent sessions:

```
scan → brief → agent works → evidence → approved memory → next brief starts warm
```

Portable declarations and approved knowledge live under `.struktly/`. Runtime
state stays in a per-user directory outside the repository.

## Trust model

- **No model calls.** Context generation is deterministic, filesystem- and Git-derived, and works offline.
- **No active-instruction writes.** Struktly never changes `AGENTS.md`, `CLAUDE.md`, or Cursor rules; it writes suggested drafts you promote by hand.
- **Memory is human-gated.** Agents can propose memory candidates; only entries you explicitly approve are injected into future briefs.
- **Security exclusions are explicit.** Git-ignored files, secret content, binaries, symlinks, dependencies, build output, and runtime state are not included in packets.

## Install

```sh
go install github.com/struktly/struktly/cmd/struktly@latest
```

Requires Go 1.25+ and Git. No account or configuration is required.

Confirm the installed build:

```sh
struktly version
```

## Quickstart

From a Git repository:

```sh
# 1. Scaffold .struktly/config.json, direction.md, and constraints.md,
#    then run the first scan.
struktly init

# 2. Generate a task-scoped context packet for your agent
struktly brief "add request timeout middleware"

# 3. Paste the packet path into your agent session
#    (.struktly/context-packets/<timestamp>-....md)
#    Or print the packet straight to stdout for piping into a prompt
#    (the "wrote" note goes to stderr):
struktly brief --stdout "add request timeout middleware"

# Every brief writes a complete struktly/packet/v1 JSON sibling. --json
# prints that packet on stdout and keeps diagnostics on stderr:
struktly brief --json "add request timeout middleware"

# 4. After the work, record what was verified — add --run-checks and
#    struktly executes the checks itself and records real exit codes;
#    the entry becomes proof, not claims
struktly evidence \
  --task "add request timeout middleware" \
  --agent "claude-code" \
  --outcome "middleware added, all handlers covered" \
  --checks "go test ./..." --run-checks

# 5. Inspect or validate the repository context boundary.
struktly status
struktly explain --task "add request timeout middleware" middleware/timeout.go
struktly validate
struktly doctor

# 6. Optional: draft agent instruction files from what scan learned
struktly suggest-instructions
```

`suggest-instructions` writes drafts to `.struktly/agent-instructions/` (`AGENTS.suggested.md`, `CLAUDE.suggested.md`, `CURSOR.suggested.md`). Review, then copy into your active instruction files. Re-run `struktly scan` any time to refresh `.struktly/project-context.md`.

The packet JSON contains Git identity, selected file contents, per-item
provenance and SHA-256 hashes, declared and detected checks, exclusions,
truncations, fixed limits, and a stable packet hash. Generation time and the
legacy root metadata are excluded from that hash; portable root fields use `.`
instead of exposing a checkout path.

For scripts, use each command's `--json` mode. Passing `--json` or
`--json-errors` also makes failures use `struktly/error/v1` on stderr. The full
stdout/stderr, exit-code, cancellation, security, limit, and performance
contract is [documented here](docs/integration-contract.md).

To wire struktly into Claude Code, Cursor, or Codex sessions, see [docs/agent-hooks.md](docs/agent-hooks.md).

## The loop in 90 seconds

This is the whole product. Session 1 starts cold:

```
$ struktly brief "add request timeout middleware"
## Task
add request timeout middleware
## Suggested Files To Inspect
- middleware/timeout.go
(no memory section — nothing has been learned yet)
```

The agent does the work. You record evidence and approve one learning:

```
$ struktly evidence --task "add request timeout middleware" --agent claude-code \
    --outcome "middleware added" --checks "go test ./..." --result pass
appended to .struktly/evidence.md

$ struktly memory candidate \
    --content "All middleware must have a matching _test.go file." --scope repository
$ struktly memory approve mem-20260710-...
```

Session 2 starts warm — the next brief carries the approved memory automatically:

```
$ struktly brief "add structured request logging"
## Approved Memory
- All middleware must have a matching _test.go file. (scope: repository)
```

No model calls, no sync, no service. Approved memory is a reviewable JSON record in Git.

## Runs and memory

Group scans, briefs, and evidence into a durable run record, and carry approved learnings across sessions:

```sh
struktly run create --goal "harden HTTP layer"      # returns a run id
struktly scan --run <run-id>
struktly brief --run <run-id> "add rate limiting"
struktly run show <run-id>

struktly memory candidate --content "Handlers must use the shared errorResponse helper." \
  --scope repository --tags conventions
struktly memory candidates                           # review the queue
struktly memory approve <candidate-id>               # approved memory appears in future briefs
```

Runs, event logs, and pending memory candidates are runtime state. New state is
stored outside the repository in the OS user configuration directory, keyed by
checkout. Set `STRUKTLY_STATE_DIR` to override the base directory. Legacy
`.struktly/runs/` and `.struktly/memory/candidates/` records are readable but
never receive new writes. Approved memory remains portable under
`.struktly/memory/approved/`.

## MCP

The same binary is an MCP server: `struktly mcp` serves `context_scan`,
`context_brief`, and `evidence_record` over stdio. `context_brief` returns the
Markdown packet for compatibility and the complete packet as MCP
`structuredContent`, so consumers do not need to parse prose. Register it with
Claude Code:

```sh
claude mcp add struktly -- struktly mcp --root .
```

## Steering the output

Struktly reads portable repository declarations under `.struktly/`:

| File | Used for |
|---|---|
| `.struktly/direction.md` | Product direction; its `## Non-goals` section is surfaced to agents as "do not build" guardrails |
| `.struktly/constraints.md` | Excerpted verbatim into briefs and instruction drafts |
| `.struktly/decisions.md` | Decisions with `**Status:** accepted` are surfaced as active decisions |
| `.struktly/config.json` | Additive context include/exclude patterns and required/suggested checks |
| `.struktly/tasks/*.md` | Versioned, reviewable task handoffs with agent and resume metadata |

`struktly init` scaffolds config and templates for direction and constraints (it
never overwrites files you already have). Built-in includes remain active;
configured excludes take precedence. Paths are repository-relative slash globs.
Portable tasks follow the [`struktly/task/v1` Markdown contract](docs/task-format.md).

## OKF

Every markdown file struktly generates — `project-context.md`, context packets,
agent-instruction drafts, `evidence.md` — carries [Open Knowledge Format v0.1](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md)
frontmatter with `type`, `schema`, title, description, and timestamp. If you add
your own frontmatter to direction, constraints, or decisions, Struktly strips it
from rendered excerpts while preserving the source file as a hashed packet item.

## Demo

Try it on a repo you don't own:

```sh
./hack/demo.sh https://github.com/go-chi/chi
```

This clones the repo cold and runs the full loop: `scan` → brief #1 →
`evidence` → `memory candidate` + `approve` → brief #2.

## Status

Alpha (v0). Formats are versioned and governed by an explicit policy — see [docs/compatibility.md](docs/compatibility.md).

| Capability | Status |
|---|---|
| `init`, `scan`, `brief`, `evidence`, `suggest-instructions`, `run`, `memory` | Released |
| OKF frontmatter on generated markdown | Released |
| `evidence --run-checks` (verified evidence) | Released |
| `struktly/packet/v1`, `struktly/snapshot/v1`, `struktly/config/v1` | Released |
| `status`, `explain`, `validate`, `doctor` | Experimental |
| `struktly mcp` tools and packet `structuredContent` | Experimental |

Known limits:

- `brief` requires a Git commit; `scan` is an explicit repository-inventory export.
- Packet selection caps each file at 64 KiB, the packet at 40 items and 512 KiB total, and reports every truncation.
- `scan` deprioritizes `legacy/`, `archive/`, `deprecated/`, `testdata/` and similar directories.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Short version: keep it local-first, deterministic, and stdlib-first — `go test ./...` must pass and cold-repo leak-guard tests keep Struktly-internal content out of generated output.

This project follows the [Code of Conduct](CODE_OF_CONDUCT.md). To report a
security vulnerability privately, follow [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE)
