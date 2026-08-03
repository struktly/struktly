---
type: task
schema: struktly/task/v1
id: consistent-command-surface
title: "Make output flags and command names consistent"
status: ready
priority: medium
created: 2026-08-03
---

# Make output flags and command names consistent

## Mission

The command surface grew a command at a time and reads that way. Two specific
inconsistencies, and a third that is a question rather than a defect.

**Output flags differ per command.** `--stdout` exists on some commands and not
others, `--json` exists on most, and at least one path rejects the pair with
"use either --stdout or --json". A reader cannot predict from one command how
the next one behaves. Anything that produces output a person or a program might
want to pipe should offer the same two ways to get it.

**`brief` is a command that is not a command.** It survives as a compatibility
alias for `context`. Whether it still earns that is a decision worth making
rather than inheriting — an alias nobody has needed in months is a name that
still has to be explained.

**Command names are uneven.** `context`, `scan`, `tasks`, `status`, `explain`,
`validate`, `doctor` are verbs or nouns depending on the command;
`suggest-instructions` is a sentence. Whether that matters is the open question:
consistency is worth something, and renaming a published CLI's commands is worth
less than it sounds.

## Requirements

- Every command that emits output supports the same flags for getting it. Decide
  the rule — most likely human-readable by default, `--json` for the machine
  contract, `--stdout` where a file would otherwise be written — and apply it
  everywhere rather than per command.
- Where two output flags are genuinely exclusive, the error says which
  combination is valid rather than restating that they conflict.
- The flag rule is stated once in `--help` output or the README, so a reader
  learns it from any one command.
- Decide `brief` explicitly: keep it, or remove it and say so in the changelog.
  Do not leave it undocumented and alive.
- If command names change, the old names keep working for at least one release
  and the change is a breaking-change note. If they do not change, record why so
  the question stops being reopened.

## Non-goals

- Changing what any command actually produces. Output formats and schemas are
  fixed by this task; only how you ask for them is in scope.
- Adding commands.
- Renaming the binary.
- Restructuring the `internal/` packages behind the commands.

## Done when

- A person who learns the output convention from one command predicts it
  correctly for every other, verified by reading `--help` for each.
- No command writes a file where a sibling would print, without a stated reason.
- `brief` is either documented or gone.
- The naming question has an answer in the repository, whichever way it went.
- `make lint`, `make test` and `make build` pass, and the capabilities document
  still reports every command that exists.

## Notes

The capabilities document is the contract a consumer negotiates against, so any
command or flag change has to keep `capabilities --json` truthful in the same
commit. A consumer that pins behaviour on a capability which quietly changed
shape is the failure this contract exists to prevent.
