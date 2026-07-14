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
struktly brief --stdout "add request timeout middleware" | claude -p
struktly brief --stdout "add request timeout middleware" | codex exec -
```

The packet contains the task and selected repository files. The agent still uses
its own configuration, permissions, and session storage.

To inspect the packet before passing it on:

```sh
struktly brief --stdout "add request timeout middleware"
```

## Instruction files

Generate draft instruction files with:

```sh
struktly scan
struktly suggest-instructions
```

Drafts are written under `.struktly/agent-instructions/`. Review one before
copying it to `AGENTS.md`, `CLAUDE.md`, or a Cursor rules directory.
