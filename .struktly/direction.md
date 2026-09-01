# Direction

`struktly` is the part of Struktly that can be checked from outside. Before work
it decides what a coding agent is given: it reads a local Git repository and
returns the files and repository guidance selected for one coding request, with
the reason recorded per file and every exclusion and truncation stated. After
work, `verify` checks that an exported Record is the one that was sealed.
`intel` is a handover to the desktop product and nothing more.

The repository is public so that decision is auditable. The value is not that
the files are better chosen than the ones a person would have pasted, but that
the choice is written down and re-checkable: the same request on the same
revision produces the same `packet_hash`, so two runs that disagree disagree
about the repository rather than about the tool.

Current work is context quality, measured against the labelled selection corpus
instead of argued about. Every slice so far changed shape after being measured
against it, so new work starts by adding cases to the corpus.

## Non-goals

- No model call, network request, or upload from this binary.
- No product state. Chats, executions, approvals, evidence, memory, and review
  history belong to the desktop product.
- No request classification, routing, or capability selection. That is a
  separate closed component, and `struktly intel` is a process handover to the
  installed desktop product rather than a reimplementation of it.
- No Go-level coupling to the desktop product, and no dependency on a module
  that is not published.
- No dependency the standard library or cobra already covers.
- No opinionated content compiled into Go literals. Opinions come from a
  repository's own `.struktly/` declarations.
