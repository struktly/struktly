# Agent hooks

Copy-pasteable recipes for wiring struktly into AI coding agents. Struktly never edits agent configuration itself — every change below is one you make and review.

## MCP (preferred)

The struktly binary is also an MCP stdio server, which is the cleanest integration for agents that speak MCP. Register it with Claude Code:

```sh
claude mcp add struktly -- struktly mcp --root .
```

It exposes three tools:

- `context_scan` — refreshes `.struktly/project-context.md` from the filesystem.
- `context_brief` — generates a task-scoped context packet, returning Markdown
  inline for compatibility and the complete `struktly/packet/v1` value as MCP
  `structuredContent`.
- `evidence_record` — appends a structured evidence entry to `.struktly/evidence.md` after the work.

The agent drives the whole loop through tool calls — no shell hooks, no copy-pasting packet paths. The recipes below are the alternative for setups without MCP.

## Claude Code hooks (alternative)

Claude Code hooks run shell commands on session events, configured in `.claude/settings.json`. Anything a `SessionStart` hook prints to stdout is added to the model's context before the first prompt.

Refresh `.struktly/project-context.md` at the start of every new session:

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup",
        "hooks": [
          { "type": "command", "command": "struktly scan" }
        ]
      }
    ]
  }
}
```

Add entries with the `resume`, `clear`, or `compact` matchers to also rescan on those events. SessionStart hooks run on every session, so keep them fast — `struktly scan` is deterministic and makes no network calls.

Because SessionStart stdout is injected as context, a hook that prints markdown puts it straight in front of the model. `struktly brief --stdout "<task>"` prints a task-scoped context packet to stdout (the "wrote" note goes to stderr), so it composes with this mechanism. Since the task is usually not known at session start, the common pattern is to pass the packet when you launch the session:

```sh
claude "$(struktly brief --stdout 'add request timeout middleware')

Do the task described in the packet above."
```

## Cursor

Cursor has no equivalent hook system that struktly assumes. Instead, promote the generated instruction draft and let the instructions make the agent run struktly itself:

```sh
struktly suggest-instructions
# review .struktly/agent-instructions/CURSOR.suggested.md, then copy it
# into .cursor/rules/ (or your active Cursor rules)
```

The draft's session protocol tells the agent to run `struktly scan` before important sessions, `struktly brief "<task>"` for a task-scoped packet, and `struktly evidence` after the work.

## Codex

Same approach: promote `.struktly/agent-instructions/AGENTS.suggested.md` into repo-root `AGENTS.md`, which Codex reads. The promoted instructions tell the agent to run the struktly commands itself.

In both cases struktly only writes the `.suggested.md` drafts — promotion into active instruction files is always a manual, reviewed step.
