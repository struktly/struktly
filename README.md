<p align="center">
  <img src="https://avatars.githubusercontent.com/u/301897861?s=192&v=4" width="96" height="96" alt="Struktly icon">
</p>

<h1 align="center">Struktly CLI</h1>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white" alt="Go 1.25 or newer">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-2ea44f" alt="MIT license"></a>
</p>

`struktly` builds useful repository context for a coding request. It reads a
local Git repository and returns Markdown for people or versioned JSON for
tools.

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
# Create optional repository configuration and a general repository summary.
struktly init

# Print context selected for one coding request.
struktly context --stdout "add request timeout middleware"
```

`context` writes a Markdown file and a `struktly/packet/v1` JSON file under
`.struktly/context-packets/`. Use `--json` when another program needs the
structured packet. Use `--json --no-write` when an integration needs only the
packet and must not modify the repository. `brief` remains a compatible alias.

You can pass the Markdown packet directly to an installed coding agent:

```sh
struktly context --stdout "add request timeout middleware" | claude -p
struktly context --stdout "add request timeout middleware" | codex exec -
```

Struktly supplies context only. Claude Code, Codex, or another caller still owns
its permissions and execution behavior.

## Core commands

| Command | What it does |
|---|---|
| `init` | Create repository configuration and run the first scan. |
| `context <request>` | Build a request-specific packet from live repository state. |
| `scan` | Write a general repository summary. It is optional and not a prerequisite for `context`. |
| `explain <path>` | Diagnose why one path would be included or excluded. |
| `validate` | Validate configuration and portable task files. |
| `doctor` | Check the repository and local CLI setup. |
| `capabilities` | Report supported schemas and machine-interface features. |
| `suggest-instructions` | Draft agent instruction files for human review. |
| `mcp` | Expose `scan`, `brief`, and `evidence` over MCP stdio. |

The CLI retains experimental `evidence`, `run`, and `memory` commands for
compatibility. Their record formats are not stable machine interfaces. The
desktop app—not this CLI—owns chats, provider sessions, working copies, checks,
and review history.

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
configuration and request-word matching. It skips sensitive filenames, detected
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
Current scope and planned context-quality work are in
[`docs/roadmap.md`](docs/roadmap.md).

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
