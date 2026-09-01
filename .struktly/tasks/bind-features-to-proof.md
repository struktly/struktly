---
type: task
schema: struktly/task/v1
id: bind-features-to-proof
title: "Bind every advertised feature to executable conformance proof"
status: ready
priority: high
created: 2026-09-01
---

# Bind every advertised feature to executable conformance proof

## Objective

`capabilities --json` cannot advertise a feature identifier that no test
proves, and cannot drop a proof while the identifier stays advertised.

## Background

The commands and schemas in the capabilities document are now derived from the
binary: `TestAdvertisedCommandsExistInCobraTree` holds the command list to the
cobra tree and `TestAdvertisedSchemasExistAndAreEnforceable` holds the schema
list to `schemas/`. The feature list is the remaining hand-written inventory.

Most features already have a proof somewhere in the suite — `context.scope`,
`context.seeds`, `context.no_write`, `scan.no_write`, `context.limits`,
`context.symbol_matching` and the rest each have tests that would fail if the
behaviour broke. What is missing is the link: nothing connects an advertised
identifier to the test that establishes it, so an identifier with no proof
(`context.telepathy`) advertises as readily as one with several, and
`--require` answers for both.

The proof for `context.no_write` is also weaker than the claim. It asserts that
`.struktly/` was not created in one temporary repository; the advertised
invariant is that nothing was written anywhere.

## Design

A bidirectional proof registry in `cmd/struktly`, as test code:

- One proof function per advertised feature, registered against its
  identifier. A proof is a test that exercises the behaviour and fails if it
  is absent — not a check that the identifier is present in a list.
- A test that asserts every advertised feature has a registered proof, and
  every registered proof names an advertised feature. Adding an identifier
  without a proof fails; deleting a proof while the identifier stays fails.
- Proofs that already exist elsewhere in the suite move or are called from
  the registry rather than duplicated.

Do not bind features to test names as strings and confirm them with
`go test -list`. That proves a name exists, which is the failure mode this
task closes.

## Required outcomes

- [ ] Every identifier in `currentCapabilities().Features` has a proof
      function in the registry, and a fabricated identifier fails the suite.
- [ ] Every proof in the registry maps to an advertised identifier, and an
      orphaned proof fails the suite.
- [ ] `context.no_write` and `scan.no_write` are proven by hashing the whole
      repository tree before and after and asserting it is unchanged, not by
      inspecting `.struktly/`.
- [ ] `docs/compatibility.md` states that the feature list is held by proof,
      in the same terms it uses for the negotiated set.

## Non-goals

- Do not add a feature identifier for behaviour that exists only to give it a
  proof. The registry describes the contract as it is.
- Do not weaken a proof to make registration easier. A feature whose proof
  cannot be written honestly should be removed from the advertisement, and
  that removal is a compatibility change to be argued as one.
- Do not touch the command or schema bindings; they are done.

## Definition of done

The registry exists with both directions asserted, a fabricated feature and an
orphaned proof each fail the suite when tried, the two `no_write` proofs
snapshot the tree, and the checks below pass.

- `go test ./cmd/struktly`
- `make test`
