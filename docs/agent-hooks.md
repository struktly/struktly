# Coding-agent integrations

Struktly prepares repository context. It does not change agent permissions or
edit agent configuration. The examples below are optional.

## MCP

The `mcp` command exposes `context_scan`, `context_brief`, and `evidence_record`
over stdio. For Claude Code:

```sh
claude mcp add struktly -- struktly mcp --root .
```

`context_brief` returns Markdown and a `struktly/packet/v1` value as structured
content.

## Claude Code and Codex

Pipe a Markdown context packet into either installed CLI:

```sh
struktly context --stdout "add request timeout middleware" | claude -p
struktly context --stdout "add request timeout middleware" | codex exec -
```

The packet contains the task and selected repository files. The agent still uses
its own configuration, permissions, and session storage.

To inspect the packet before passing it on:

```sh
struktly context --stdout "add request timeout middleware"
```

`brief` remains an alias for existing scripts. Integrations that consume JSON
without repository-local exports should use `context --json --no-write` and pin
the approved `HEAD` with `--expect-base-revision`.

## Instruction files

Generate draft instruction files with:

```sh
struktly scan
struktly suggest-instructions
```

Drafts are written under `.struktly/agent-instructions/`. Review one before
copying it to `AGENTS.md`, `CLAUDE.md`, or a Cursor rules directory.
