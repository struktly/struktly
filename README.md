<p align="center">
  <img src="https://avatars.githubusercontent.com/u/301897861?s=192&v=4" width="96" height="96" alt="Struktly icon">
</p>

<h1 align="center">Struktly</h1>

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white" alt="Go 1.25 or newer">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-MIT-2ea44f" alt="MIT license"></a>
</p>

`struktly` is a local CLI that scans a Git repository and selects context for a
specific task. It writes Markdown for people and versioned JSON for tools. It
does not call a model or hosted service.

## Install

Struktly requires Go 1.25 or newer and Git.

```sh
go install github.com/struktly/struktly/cmd/struktly@latest
struktly version
```

## Quick start

Run these commands from a Git repository:

```sh
# Create .struktly/config.json and scan the repository.
struktly init

# Print a task-specific context packet.
struktly brief --stdout "add request timeout middleware"

# Record checks after the change.
struktly evidence \
  --task "add request timeout middleware" \
  --checks "go test ./..." \
  --run-checks
```

`brief` writes a Markdown packet and a `struktly/packet/v1` JSON file under
`.struktly/context-packets/`. Use `--json` to write the JSON packet to stdout.

## Commands

The main commands are:

| Command | Purpose |
|---|---|
| `init` | Create `.struktly/config.json` and scan the repository. |
| `scan` | Write a repository summary to `.struktly/project-context.md`. |
| `brief <task>` | Select context for one task and write a context packet. |
| `explain <path>` | Explain why a path is included or excluded. |
| `validate` | Validate repository configuration and task files. |
| `doctor` | Check the repository and local installation. |
| `evidence` | Append a record of work and verification. |
| `run` | Create and inspect local work records. |
| `memory` | Review and approve repository notes. |
| `mcp` | Serve `scan`, `brief`, and `evidence` over MCP stdio. |

Run `struktly <command> --help` for flags and examples.

## Files and state

Repository files live under `.struktly/`:

- `config.json` controls additional includes, excludes, and checks.
- Optional `direction.md`, `constraints.md`, and `decisions.md` add repository-owned context.
- `project-context.md` and `context-packets/` contain generated output.
- `evidence.md` and `memory/approved/` contain reviewable records.
- `tasks/` contains optional `struktly/task/v1` task files.

Runs, event logs, and unapproved memory candidates are stored in the user's
configuration directory, not in the repository. Set `STRUKTLY_STATE_DIR` to
override that location.

## Selection and safety

Struktly asks Git for tracked and non-ignored files. It excludes sensitive
filenames and detected secrets, binaries, symlinks, dependencies, build output,
and local runtime state. Context packets report exclusions and truncation.

The current packet limits are 40 files, 64 KiB per file, and 512 KiB total.
Selected files include their source path, Git revision, byte counts, and SHA-256
content hash.

## JSON and MCP

Machine-readable formats are defined in [`schemas/`](schemas/). The command,
stream, error, and exit-code contract is documented in
[`docs/integration-contract.md`](docs/integration-contract.md). Compatibility
rules are in [`docs/compatibility.md`](docs/compatibility.md).

Start the MCP server with:

```sh
struktly mcp --root .
```

Agent-specific setup examples are in [`docs/agent-hooks.md`](docs/agent-hooks.md).

## Development

```sh
make lint
make test
make build
```

See [`CONTRIBUTING.md`](CONTRIBUTING.md) and [`SECURITY.md`](SECURITY.md).

## License

[MIT](LICENSE)
