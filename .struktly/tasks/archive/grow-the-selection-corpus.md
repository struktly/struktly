---
type: task
schema: struktly/task/v1
id: grow-the-selection-corpus
title: "Grow the labelled selection corpus past the shapes it already covers"
status: done
priority: medium
created: 2026-09-01
updated: 2026-09-01
---

# Grow the labelled selection corpus past the shapes it already covers

## Objective

The corpus measures selection on repository shapes it does not measure today,
so the next context-quality change has something to be wrong against.

## Background

`docs/roadmap.md` says the recorded context-quality slices are done and that
further work starts here, because every slice so far changed shape after being
measured against the corpus. The corpus is currently six cases over three
fixtures — `go-service`, `flat-package`, `noisy-legacy` — all of them Go, all
of them small, none of them with a repository whose guidance contradicts
itself.

That is enough to have caught real regressions and not enough to justify the
next selector change. A slice measured only against shapes that already pass is
a slice measured against nothing.

## Required outcomes

- [ ] Cases exist for shapes the corpus does not reach today. At minimum: a
      repository whose only guidance is `AGENTS.md`, a repository with a deep
      nested module layout, a repository where the request names a symbol that
      appears in several packages, and a repository large enough that the
      packet limits truncate.
- [ ] At least one non-Go fixture, so `mustSurface` recall does not depend on
      declaration rendering and symbol matching being available.
- [ ] Every new case carries the same labelling discipline as the existing
      ones: `mustSelect` for content that must be carried, `mustSurface` for a
      file that must at least be named, `mustExclude` for irrelevance and
      disclosure, and a comment saying why each label is what it is.
- [ ] `report.json` is regenerated and the growth in selected items is visible
      per case rather than aggregated.

## Non-goals

- Do not add a precision metric or an accuracy score. The corpus records
  selected-item counts deliberately, for the reason written at the top of
  `corpus_test.go`.
- Do not change the selector in this task. A case that fails is a finding, and
  it should be recorded as one rather than fixed under cover of adding it.
- Do not label a file relevant because the current selector picks it.

## Definition of done

New cases are labelled, the corpus report is regenerated, any case that fails
today is recorded as a finding rather than silently accommodated, and both
checks below pass.

- `go test ./internal/context`
- `make test`
