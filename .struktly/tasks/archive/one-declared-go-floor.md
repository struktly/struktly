---
type: task
schema: struktly/task/v1
id: one-declared-go-floor
title: "State the supported Go floor once, and check that every place agrees"
status: done
priority: low
created: 2026-09-01
updated: 2026-09-01
---

# State the supported Go floor once, and check that every place agrees

## Objective

The minimum Go this module supports is decided deliberately, recorded where a
decision belongs, and checked — so the five places that state it cannot drift
apart.

## Background

The floor is currently written in five places: `go.mod`, `mise.toml`, the
README badge, the README prose, and `CONTRIBUTING.md`. Nothing compares them.
CI resolves `go.mod` for lint and builds the test matrix on `stable`, so a
raised `go` directive would be noticed, but a stale badge or a stale sentence
would not be, and the badge is the first thing a prospective installer reads.

The floor also has not been chosen against the constraint that makes it matter:
this is the module people install with `go install`, so its `go` directive is a
compatibility surface for toolchains nobody here controls. The sibling
components in this ecosystem hold a different floor for their own reasons, and
copying either direction without deciding is how a published minimum moves by
accident.

## Required outcomes

- [ ] The floor is chosen explicitly, with the reason recorded in
      `.struktly/decisions.md` — which this repository declares support for and
      does not yet have.
- [ ] Every statement of the floor agrees with `go.mod`, including the README
      badge.
- [ ] A check fails when they disagree, and it runs where a pull request sees
      it.
- [ ] `AGENTS.md` and `CONTRIBUTING.md` say which file is authoritative, so the
      next raise is a one-line edit and a regenerated set rather than a search.

## Non-goals

- Do not raise the floor as part of this task. Deciding and enforcing are the
  work; a raise is a separate change with its own reason and its own release
  note.
- Do not add a toolchain directive to `go.mod` to pin consumers to a version.
- Do not make CI install the toolchain through mise. mise is the local pin and
  CI pins separately; that split is deliberate.

## Definition of done

The decision file records the floor and why; the badge, both prose statements,
the mise pin and the go directive agree; a check enforces the agreement and
fails when one of them is edited alone; and both checks below pass.

- `make lint`
- `make test`
