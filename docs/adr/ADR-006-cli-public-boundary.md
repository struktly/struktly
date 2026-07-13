# ADR-006: Public CLI boundary

Status: accepted (2026-07-13)

## Context

The repository was split from a broader product implementation. Earlier ADRs
described platform storage, events, and future surfaces that are not part of the
current CLI and would make this repository depend on private planning.

## Decision

This repository is authoritative only for the independently useful Struktly CLI,
its stable `.struktly/` formats, and its documented MCP interface. It has no
runtime dependency on another Struktly repository, account, service, or provider.

Portable declarations and approved knowledge live under `.struktly/`. Sessions,
credentials, caches, logs, run state, and pending memory are per-user runtime
state outside the repository.

This supersedes prior platform ADRs, which are not part of the public CLI record.
ADR-005 remains the topology record for this public boundary.

## Consequences

Public documentation and contributor guidance describe implemented CLI behavior
only. Changes to the executable, schemas, command streams, or MCP tool names
follow `docs/compatibility.md`.
