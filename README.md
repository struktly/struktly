<p align="center">
  <img src="https://avatars.githubusercontent.com/u/301897861?s=192&v=4" width="96" height="96" alt="Struktly icon">
</p>

<h1 align="center">Struktly CLI</h1>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white" alt="Go 1.25 or newer">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-2ea44f" alt="MIT license"></a>
</p>

`struktly` is the part of Struktly that can be checked from outside. Before
work, it decides what a coding agent is given: it reads a local Git repository
and produces a context packet — the files and repository guidance selected for
one coding request, with a record of what was selected, what was excluded, and
why. After work, `verify` checks that an exported Struktly Record is the one
that was sealed, by arithmetic rather than by trusting whoever sent it. Between
the two, `capabilities` states what a build supports, and that statement is
held to the binary by test.

This repository is public so those decisions can be audited. The
[Struktly desktop app](https://struktly.app/) is a separate, closed product that
runs and reviews agent work; it uses this CLI as its context layer, and anyone
it hands a Record to can run `verify` without it. What you can read here is what
the app uses to build context — so the selection behavior, the exclusions, and
the limits are checkable by anyone, not taken on trust.

The CLI runs locally. It does not call a model, upload source code, or start a
coding agent, and a test holds the installed binary to that.

## Verify it yourself

Struktly requires Go 1.25 or newer and Git.

```sh
git clone https://github.com/struktly/struktly
cd struktly
make test
make build
```

Or install the published build directly:

```sh
go install github.com/struktly/struktly/cmd/struktly@latest
struktly version
```

Run it against a repository you know well and read what it selected:

```sh
struktly context --stdout "add request timeout middleware"
```

What comes back is a Markdown packet. Abridged, on this repository:

```markdown
## Packet details

- Packet hash: `sha256:...`
- HEAD revision: `...`
- Scope: `whole repository`

## Suggested checks

- `go test ./...`
- `make lint`
- `make test`

## Relevant documentation

- `README.md`
- `docs/integration-contract.md`

## Files to inspect

- `.github/pull_request_template.md`
- `README.md`

## Included files

### `README.md`
...
```

Nobody told it that `make test` runs this repository's tests or that
`docs/integration-contract.md` is worth reading — it asked Git what is tracked,
matched the request against symbols and declarations, and applied its selection
rules — the built-in ones here, plus whatever `.struktly/config.json` adds in a
repository that has one. `--json` gives the same selection with the
reason recorded per file (`symbol_match`, `task_match`, `selection_rule`)
alongside the exclusions that applied and any truncation the limits caused.

That is the difference from pasting paths yourself: not that the files are
better chosen, but that the choice is written down and re-checkable. The same
request on the same revision produces the same `packet_hash`, so two runs that
disagree disagree about the repository and not about the tool.

The packet is a plain artifact. `context` writes a Markdown file and a
`struktly/packet/v2` JSON file under `.struktly/context-packets/`. Use `--json`
for the structured packet, and `--json --no-write` when nothing may be written to
the repository. `brief` remains a compatible alias.

Two commands exist specifically to make the selection inspectable:

```sh
# Why would this file be included or excluded?
struktly explain internal/http/router.go

# What schemas and machine-interface features does this build support?
struktly capabilities
```

Because the packet is just text, you can confirm that what reaches an agent is
exactly what the packet contains:

```sh
struktly context --stdout "add request timeout middleware" | claude -p
struktly context --stdout "add request timeout middleware" | codex exec -
```

Struktly supplies context only. Claude Code, Codex, or another caller still owns
its permissions and execution behavior.

## Commands

| Command | What it does |
|---|---|
| `init` | Create repository configuration and run the first scan. |
| `context <request>` | Build a request-specific packet from live repository state. Use `--scope <dir>` to narrow it to one package or service, and `--seed <path>` to name files you already know matter. |
| `scan` | Write a general repository summary. It is optional and not a prerequisite for `context`. |
| `tasks` | Emit safely readable repository task declarations and per-file invalid results. |
| `tasks complete <id>` | Set a task's status to `done`, file it under `tasks/archive/`, and repair links, in one atomic transition. |
| `tasks archive` | File already-finished tasks under `tasks/archive/`; `--check` gates CI on the location invariant. |
| `status` | Report repository, configuration, and portable-file state. *(experimental)* |
| `explain <path>` | Diagnose why one path would be included or excluded. *(experimental)* |
| `diff <before> <after>` | Report what changed between two context packets. |
| `validate` | Validate configuration and portable task files. *(experimental)* |
| `doctor` | Check the repository and local CLI setup; exits 1 if a check fails. *(experimental)* |
| `capabilities` | Report supported commands, schemas and machine-interface features; `--require <file>` fails unless a consumer's list is satisfied. |
| `suggest-instructions` | Draft agent instruction files for human review. |
| `verify <bundle>` | Check that an exported Struktly Record is intact, without Struktly. |
| `version` | Print version and build metadata. |
| `mcp` | Expose repository scanning and request-specific context over MCP stdio. |
| `intel` | Pass a command through to the installed Struktly desktop app; exits 127 when it is absent. |

Run `struktly <command> --help` for flags. Commands marked experimental print
that word in their own help; their output may change without the compatibility
guarantees in [`docs/compatibility.md`](docs/compatibility.md).

## What enters a packet

Struktly asks Git for tracked and non-ignored files, then applies repository
configuration and token-aware request matching. It skips sensitive filenames,
detected secrets, binaries, symlinks, dependencies, build output, and local
runtime state. Packets record selected items, relevant exclusions, and truncation
caused by applied limits.

The built-in limits are 40 files, 64 KiB per file, and 512 KiB total. They are
exposed as `context.limits` and can only be tightened with `--max-items`,
`--max-file-bytes`, and `--max-total-bytes`.

## Files and state

Repository-owned files live under `.struktly/`:

- `config.json` adds context include/exclude rules and check commands.
- `direction.md`, `constraints.md`, and `decisions.md` are optional project
  guidance written by people.
- `project-context.md`, `context-packets/` and `agent-instructions/` are
  generated output, and are Git-ignored by default.
- `tasks/` contains optional portable task handoffs; finished ones live under
  `tasks/archive/`.

Runtime and product state is deliberately absent from this CLI.

## System boundary

This repository owns deterministic repository selection and packaging: scanning,
selection, context packets, task declarations, provenance, and validation.
Request interpretation, privacy policy, model routing, local-model management,
dispatch, approvals, sessions, and durable history belong to Struktly Platform.
Intelligence experiments are integrated there only when a product capability
needs them; they are not dependencies of this CLI.

The desktop app—not this CLI—owns chats, executions, provider sessions, working
copies, checks, evidence, memory, and review history.

## Driving the desktop platform headlessly

If the Struktly desktop app is installed, `struktly intel` runs its headless
entrypoint:

```sh
struktly intel plan "add request timeout middleware"
struktly intel status --json
```

This is a bridge and nothing else. The CLI imports no platform code, opens no
connection to a platform process, and still calls no model. It finds the `intel`
binary the app ships beside `struktly-server`, hands the process over to it with
every argument and the environment unchanged, and returns its exit code. What
`intel` accepts and prints is the platform's contract, documented by the
platform. This page does not list its subcommands, because a copy of them here
would be wrong the first time the platform grew one — run `struktly intel` with
no arguments and the installed app answers for itself.

What that path needs on the machine is the platform's to state, not this
page's, and it is stated by `struktly intel` itself. Two things are this
repository's business and are true here: the desktop application does not have
to be open, and no window appears. Whether a given provider can be reached from
a terminal alone depends on where its credentials live, which the platform
documents.

The binary is resolved as `$STRUKTLY_INTEL`, then `intel` beside this executable,
then the desktop app's install location (on macOS,
`/Applications/Struktly.app/Contents/MacOS/`), so a `struktly` installed on its
own still finds the app, and only then `intel` on `PATH`. A file at the install
location is the platform by construction; a file named `intel` on `PATH` is not,
and it would be handed your arguments and environment. Without the desktop app, the command prints one sentence on stderr and
exits 127, so a script can tell "not installed" apart from any answer the
platform itself gives.

## Machine interfaces

Machine-readable formats are defined in [`schemas/`](schemas/). The command,
stream, error, and exit-code contract is documented in
[`docs/integration-contract.md`](docs/integration-contract.md). Compatibility
rules are in [`docs/compatibility.md`](docs/compatibility.md). Current scope and
planned context-quality work are in [`docs/roadmap.md`](docs/roadmap.md).

Start the MCP server with:

```sh
struktly mcp --root .
```

See [`docs/agent-hooks.md`](docs/agent-hooks.md) for Claude Code and Codex
examples.

## Development

```sh
make lint
make test
make build
```

See [`CONTRIBUTING.md`](CONTRIBUTING.md) and [`SECURITY.md`](SECURITY.md).

## License

[MIT](LICENSE)
