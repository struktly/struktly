---
type: task
schema: struktly/task/v1
id: contract-a-consumer-can-check
title: "Let a consumer check a build against the negotiated contract without restating it"
status: done
priority: high
created: 2026-09-01
updated: 2026-09-01
---

# Let a consumer check a build against the negotiated contract without restating it

## Objective

A consumer that gates on this CLI can assert a candidate build satisfies the
contract by asking the binary, not by carrying its own copy of the list.

## Background

`capabilities --json` reports what a build supports, and
`TestCapabilitiesCommandReportsContextContract` now holds what a consumer is
entitled to find. But a consumer gating a version bump has no supported way to
express "these are the entries I need", so the known integration today
hand-copies the commands, schemas and features into its own pin workflow and
compares them in a script. Two lists, one of them with no test behind it, and
the copy is the one that decides whether a bump ships.

The asymmetry is the problem: this repository can prove it did not drop an
entry, and a consumer still cannot prove it without repeating the entry.

## Required outcomes

- [ ] A consumer can assert a required set against a build in one invocation
      and get a non-zero exit and a structured error naming every missing
      entry, not just the first.
- [ ] The mechanism is advertised in `capabilities` like any other machine
      surface, so a consumer can detect whether the build it has supports being
      asked.
- [ ] `docs/integration-contract.md` documents it, and
      `docs/compatibility.md` says how it relates to the negotiated set the
      test holds.
- [ ] The negotiated set stays defined in exactly one place in this
      repository.

## Non-goals

- Do not name a consumer, encode one consumer's list, or ship a default
  "required" set. What a caller needs is the caller's statement, not this
  repository's guess.
- Do not add a network fetch, a version-comparison rule, or any notion of a
  supported version range. `capabilities` answers about the binary in hand.
- Do not change what is currently advertised in order to make the mechanism
  tidier.

## Design notes

Two shapes are worth comparing before writing code, and the choice belongs in
the pull request rather than here:

1. A flag — `capabilities --require <file>` or repeated `--require` values —
   which keeps the answer an exit code and needs no new schema.
2. A schema field, where `capabilities` reports the negotiated set itself and a
   consumer diffs it, which is more data and less policy.

Prefer whichever leaves the consumer's gate shorter, since the length of that
gate is what caused the duplication.

## Definition of done

The mechanism ships with tests covering the satisfied case, the missing-entry
case, and a malformed requirement; capabilities advertises it; the documents
above describe it; and both checks below pass.

- `make lint`
- `make test`
