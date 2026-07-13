# Behavior Contracts

A behavior contract is a human-approved markdown file that pins down what a service *must keep doing* while agents refactor around it. Struktly's brownfield workflow assumes one contract per service, stored under `.struktly/behavior-contracts/` and reviewed like code.

The CLI does not generate or validate contracts today — this is a convention document. Contracts pay off because briefs and instruction drafts can point agents at them, and evidence entries can cite them as the spec a change was verified against.

## Template

```markdown
# Behavior Contract: <service-name>

- Status: draft | approved
- Approved by: <name or pending>
- Source repo: `<path or url>`
- Linked tests:
  - `<test file that enforces a rule below>`

## Goal

One paragraph: what the service does and for whom.

## Triggers

Numbered list of entry points — HTTP routes, queue consumers, cron jobs, CLI commands.

## Rules

Numbered, testable statements of required behavior. Each rule should be
enforceable by at least one linked test. Prefer "must" phrasing with concrete
inputs and outputs (status codes, payload fields, ordering guarantees).

## Exclusion Rules

What the service deliberately does NOT do. These stop agents from "helpfully"
adding endpoints, retries, or fields that were excluded on purpose.

## Idempotency / Dry-Run

How repeated or speculative invocations behave, if applicable.

## Acceptance Examples

| Scenario | Expected |
|----------|----------|
| <concrete input> | <concrete observable outcome> |

## Remains Uncertain

Open questions the contract does not settle yet.
```

## Working with contracts

- Keep rules atomic: one observable behavior per numbered rule, so a failing test maps to exactly one rule.
- Update the contract in the same PR that changes the behavior — a stale contract is worse than none.
- When briefing an agent for a refactor, include the contract path in the task, e.g. `struktly brief "refactor order storage; behavior contract: .struktly/behavior-contracts/orders.md"`.
