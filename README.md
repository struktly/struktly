<p align="center">
  <img src="https://avatars.githubusercontent.com/u/301897861?s=192&v=4" width="96" height="96" alt="Struktly icon">
</p>

<h1 align="center">Struktly CLI</h1>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white" alt="Go 1.25 or newer">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-2ea44f" alt="MIT license"></a>
</p>

`struktly` selects useful repository context for a specific coding task. It
reads a local Git repository and writes the result as Markdown for people and
versioned JSON for tools.

The CLI runs locally. It does not call a model, upload source code, or start a
coding agent.

The [Struktly desktop app](https://struktly.io/) uses this CLI as its context
layer. The app is a separate product for running and reviewing agent work; this
repository contains only the open-source CLI.

## Install

Struktly requires Go 1.25 or newer and Git.

```sh
go install github.com/struktly/struktly/cmd/struktly@latest
struktly version
```

## Try it

Run these commands from a Git repository:

```sh
# Create .struktly/config.json and scan the repository.
struktly init

# Print context selected for one task.
struktly brief --stdout "add request timeout middleware"
```

`brief` also writes a Markdown file and a `struktly/packet/v1` JSON file under
`.struktly/context-packets/`. Use `--json` when another program needs the
structured packet.

You can pass the Markdown packet directly to an installed coding agent:

```sh
struktly brief --stdout "add request timeout middleware" | claude -p
struktly brief --stdout "add request timeout middleware" | codex exec -
```

Struktly supplies context only. Claude Code, Codex, or another caller still owns
its permissions and execution behavior.

## Core commands

| Command | What it does |
|---|---|
| `init` | Create repository configuration and run the first scan. |
| `scan` | Summarize repository structure, commands, and guidance. |
| `brief <task>` | Select repository files and guidance for one task. |
| `explain <path>` | Explain why a path would be included or excluded. |
| `validate` | Validate configuration and portable task files. |
| `doctor` | Check the repository and local CLI setup. |
| `suggest-instructions` | Draft agent instruction files for human review. |
| `mcp` | Expose `scan`, `brief`, and `evidence` over MCP stdio. |

The CLI also includes experimental `evidence`, `run`, and `memory` commands.
Their record formats are not stable machine interfaces yet. They are kept
separate from the versioned context schemas.

Run `struktly <command> --help` for flags.

## Files and state

Repository-owned files live under `.struktly/`:

- `config.json` adds context include/exclude rules and check commands.
- `direction.md`, `constraints.md`, and `decisions.md` are optional project
  guidance written by people.
- `project-context.md` and `context-packets/` are generated output.
- `tasks/` contains optional portable task handoffs.
- `evidence.md` and `memory/approved/` contain optional reviewed records.

Runtime work records, event logs, and unapproved memory candidates live in the
user configuration directory rather than the repository. Set
`STRUKTLY_STATE_DIR` to choose another location.

## What enters a packet

Struktly asks Git for tracked and non-ignored files, then applies repository
configuration and task-word matching. It skips sensitive filenames, detected
secrets, binaries, symlinks, dependencies, build output, and local runtime
state. Every packet explains exclusions and truncation.

The current limits are 40 files, 64 KiB per file, and 512 KiB total. Selected
files include their path, Git revision, byte counts, and full-content SHA-256
hash.

## Integrations

Machine-readable formats are defined in [`schemas/`](schemas/). The command,
stream, error, and exit-code contract is documented in
[`docs/integration-contract.md`](docs/integration-contract.md). Compatibility
rules are in [`docs/compatibility.md`](docs/compatibility.md).

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
