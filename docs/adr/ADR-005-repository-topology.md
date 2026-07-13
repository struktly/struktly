# ADR-005: Repository topology (split revision)

Status: accepted (2026-07-12, decision by Sebastian). Supersedes the
single-monorepo decision recorded here on 2026-07-10.

## Context

The public CLI needs a clear compatibility boundary that does not require access
to a separate private product or marketing repository.

## Decision

- This repository owns the independently installable open-source context CLI and
  stable `.struktly` formats.
- Separate private products and marketing sites are outside this repository and
  do not define its contributor workflow or public roadmap.
- Other products consume the CLI through its executable and stable file formats,
  never by importing its Go internals or duplicating scanner behavior.

## Consequences

This repository keeps module path `github.com/struktly/struktly` and its public
compatibility obligations. Cross-product changes may require coordinated releases,
but implementations must not be copied across the boundary.
