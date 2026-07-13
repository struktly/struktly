---
type: direction
title: "Struktly Direction"
description: "Repo-owned product direction consumed by struktly briefs and instruction drafts."
timestamp: 2026-07-10T18:30:00Z
---

# Struktly Direction

Struktly is an MIT-licensed, local-first CLI for building inspectable repository
context for coding-agent work. It works from local files and Git metadata only:
no account, server, GUI, model provider, or network call is required.

## Product Boundary

- This repository owns the `struktly` executable and documented stable
  `.struktly/` formats.
- The CLI scans repositories, produces task-scoped context packets, records
  reviewable evidence, and supports human-approved portable memory.
- JSON schemas and the documented command/stream contract are the public
  integration boundary. Markdown is human-readable presentation.
- Other products may consume the executable and these formats, but this
  repository is self-governing and contributors need no other checkout or plan.

## Near-Term Direction

Improve the reliability, clarity, and portability of the existing CLI contract:
deterministic output, safe selection, clear diagnostics, cross-platform behavior,
and concise workflows for contributors and coding agents. See `docs/roadmap.md`.

## Non-goals

- A new coding agent, chat interface, or foundation model
- An unbounded autonomous software engineer, Devin clone, or OpenHands fork
- Automatic promotion of agent output into trusted memory
- Any core capability that requires a hosted account, SaaS dependency, or model provider to function
- A hosted control plane, GUI, worktree manager, terminal, or provider adapter
- A proprietary replacement for Git, Markdown, JSON, or MCP
