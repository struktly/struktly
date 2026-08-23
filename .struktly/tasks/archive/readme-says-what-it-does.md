---
type: task
schema: struktly/task/v1
id: readme-says-what-it-does
title: "Make the README explain what this tool is for"
status: done
priority: high
created: 2026-08-03
updated: 2026-08-23
---

# Make the README explain what this tool is for

## Mission

Someone who runs this CLI against a real repository gets good output and still
cannot say what the tool is for or when to reach for it. The README describes
mechanics accurately and never lands the point: what problem this solves, what
you do with the result, and why you would run it rather than letting an agent
browse the repository itself.

The example is the sharpest instance. `struktly context --stdout "add request
auth middleware"` shows the invocation and not the value — a reader cannot tell
from it what comes back, how it differs from what they would get by pasting file
paths themselves, or what they are meant to do with it next.

## Requirements

- Every capability claim is verified against the current binary before it is
  written. A claim that was true of an earlier version is worse than no claim,
  because a reader who checks and finds it false stops trusting the rest.
- The opening states the job, not the mechanism: what a reader gets, and when it
  is the right tool. "Builds repository context for a coding request" describes
  the implementation of something the reader has to infer.
- At least one example shows real output, not just the command. Abridged is
  fine; invented is not. A reader should be able to see the shape of a packet
  and understand why it is worth having.
- The examples use a request a reader recognises, against a repository shape
  they can picture. The current one is plausible but abstract.
- Say plainly what the tool does not do. It does not call a model, upload
  source, or run an agent. Those are trust-relevant facts and they read as
  reassurance, not as limitations.
- The relationship to the desktop application is stated once, accurately, and
  without implying the CLI is a teaser for it.

## Non-goals

- Rewriting the command reference or the schema documentation.
- Marketing voice, feature lists, badges, or comparison tables.
- Documenting behaviour that does not exist yet, including anything that only
  the desktop application does.
- Changing any command, flag, or output format. This is documentation of what is
  there.

## Done when

- Every claim in the README is reproducible against the current binary, checked
  by running it rather than by reading the source.
- A reader unfamiliar with the project can state, after the first screen, what
  the tool produces and when they would use it.
- At least one example shows real output.
- `make lint` and `make test` pass unchanged; this task touches no code.
