---
type: task
schema: struktly/task/v1
id: context-engine-contract
title: "Make the context engine a stable platform dependency"
status: done
priority: high
created: 2026-07-15
updated: 2026-07-15
agent: codex
---

# Make the context engine a stable platform dependency

## Pick up this task

Read `docs/integration-contract.md`, then preserve `struktly/packet/v1` while completing the unchecked outcomes below.

## Objective

Make Struktly's open-source CLI the deterministic, inspectable context engine used by the desktop app without turning the CLI into a second chat or execution product.

## Constraints

- Keep existing `brief` invocations and `struktly/packet/v1` compatible.
- Keep context generation local and deterministic.
- Do not move Chat, Session, working-copy, or review ownership into this repository.
- Do not require a separately installed CLI for the packaged desktop app.

## Required outcomes

- [x] Support side-effect-free JSON context and scan generation.
- [x] Fail with a stable error when the approved Git revision changes.
- [x] Publish a machine-readable capabilities contract.
- [x] Present `context` as the primary request-scoped command while retaining `brief` compatibility.
- [x] Explain the boundary between `scan`, `context`, and `explain` in public docs.
- [x] Record the next context-quality slices without claiming they already exist.

## Execution plan

1. Extend the existing command and packet boundary additively.
2. Update the versioned schemas and integration contract.
3. Verify focused command, context, and schema behavior.

## Definition of done

The desktop app can negotiate capabilities, request packet v1 at an approved revision without repository writes, and receive structured cancellation or revision errors; existing `brief` integrations continue to work.

## Evidence

- The Go test suite and lint target pass.
- `struktly validate --json` accepts this portable task.
- Live capability and side-effect-free packet invocations return their versioned schemas.
