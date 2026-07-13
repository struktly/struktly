# Agent integrations

Struktly does not edit agent configuration. The examples below are optional.

## MCP

The `mcp` command exposes `context_scan`, `context_brief`, and `evidence_record`
over stdio. For Claude Code:

```sh
claude mcp add struktly -- struktly mcp --root .
```

`context_brief` returns Markdown and a `struktly/packet/v1` value as structured
content.

## Shell

Print a packet and pass it to any command that accepts a prompt:

```sh
struktly brief --stdout "add request timeout middleware"
```

Claude Code can receive the packet directly:

```sh
claude "$(struktly brief --stdout 'add request timeout middleware')"
```

## Instruction files

Generate draft instruction files with:

```sh
struktly scan
struktly suggest-instructions
```

The drafts are written under `.struktly/agent-instructions/`. Review a draft
before copying it to `AGENTS.md`, `CLAUDE.md`, or a Cursor rules directory.
