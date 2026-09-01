---
type: task
schema: struktly/task/v1
id: selection-precision-gaps
title: "Close the three selection gaps the widened corpus found"
status: ready
priority: high
created: 2026-09-01
---

# Close the three selection gaps the widened corpus found

## Objective

The three cases in the corpus that currently carry a `knownGap` assert normally,
because the selector no longer has the gap they record.

## Background

Widening the corpus past the shapes it already covered turned up three failures
on the first run. None of them is a labelling argument: each is a request a
person would ask, answered with a packet a person would call wrong.

They are recorded as `knownGap` on their cases in `internal/context/corpus_test.go`
rather than as prose, so the labels stay honest, the suite stays green, and
closing one fails the corpus until the field is deleted.

### A request word that matches a directory selects every file in it

`nested-modules / record telemetry from the api checkout handler` also selects
`services/api/internal/handler/health.go`, as `task_match`. The request names
the checkout handler; `handler` is a path segment, and matching it appears to
admit the whole directory. In a deep tree that is the difference between a
packet about one handler and a packet about a package.

### A symbol in several packages is not narrowed by the rest of the request

`ambiguous-symbol / close the store client cleanly` selects `store/client.go`
and also `http/client.go` and `worker/client.go`. `Client` is declared three
times; the request says which one. Symbol matching reaches all the declarations
of a name and nothing weighs the other words against them.

Note the sibling case, `rename the Client type everywhere it is declared`,
which wants all three and passes. The two together are the point: the same
repository, the same symbol, and the correct answer differs.

### Guidance fills a tight item budget before any code is selected

`go-service / add request timeout middleware +items=4` returns four items, all
of them repository guidance, and no code at all. Guidance is worth carrying;
it is not worth the whole budget. A packet that describes the repository and
omits the file the request is about has spent its room on the part a reader
could have guessed.

The byte-budget case beside it behaves correctly — the answer is carried and
truncated — so this is about how the item budget is spent, not about limits in
general.

## Required outcomes

- [ ] A request word that matches a directory no longer admits files in it that
      nothing else in the request reaches.
- [ ] A symbol declared in several packages is narrowed by the rest of the
      request, without breaking the case that wants every declaration.
- [ ] Under a tight item budget, the files that answer the request are selected
      before guidance fills it.
- [ ] The three `knownGap` fields are deleted and their cases assert normally.
- [ ] `report.json` is regenerated and the movement in every other case is
      read before it is accepted.

## Non-goals

- Do not weaken a corpus label to close a gap. If a label is wrong, argue that
  separately; the whole value of the corpus is that a label is not adjustable
  by whoever is trying to get a green run.
- Do not add a relevance score, a ranking model, or anything that stops being
  explainable by `struktly explain`.
- Do not fix these one at a time without rerunning the whole corpus. Every
  slice so far moved cases nobody expected it to.

## Definition of done

The three cases assert normally with their labels unchanged, no case regressed,
the corpus report is regenerated and its movement accounted for, and both
checks below pass.

- `go test ./internal/context`
- `make test`
