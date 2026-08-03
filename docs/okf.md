# Open Knowledge Format conformance

Struktly's repository task declarations are an **OKF v0.2 bundle**. This document
records what that commits us to, which parts of the spec are normative for this
repository, and where the specification lives.

## Source

Open Knowledge Format, Google Cloud, vendor-neutral markdown specification for
giving AI agents curated context.

- Specification: <https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md>
- Announcement: <https://cloud.google.com/blog/products/data-analytics/how-the-open-knowledge-format-can-improve-data-sharing>
- Version targeted here: **v0.2**

Section numbers below refer to that document. It is not vendored: a copy would
drift, and the point of adopting an external format is that someone else owns
it.

## Why

A task is curated context for an agent, which is the thing OKF exists to carry.
Emitting a conformant bundle means a repository's declared work is readable by
any OKF consumer without installing Struktly, and the CLI stops being a private
format with a public binary.

## What conformance requires (§11)

A bundle conforms when:

1. every non-reserved `.md` file in the tree contains a parseable YAML
   frontmatter block;
2. every frontmatter block contains a non-empty `type` field;
3. every reserved filename — `index.md` (§8), `log.md` (§9) — follows its
   defined structure when present.

`.struktly/tasks/` satisfies all three. Every task carries `type: task`;
`index.md` is the directory listing and is skipped by discovery rather than
parsed as a concept.

## What conformance forbids, and what we changed

§4.1 states that producers may add frontmatter keys, that consumers **MUST NOT**
reject documents with unrecognized fields, and that they **SHOULD** preserve
those keys when round-tripping. §6 states that consumers **MUST** tolerate
broken links.

Task validation used to reject any frontmatter key outside a fixed allowlist.
That is a direct violation, and its practical cost was worse than the principle:
a repository could not annotate its own tasks without the file disappearing from
the task list. Unknown keys are now carried in `Task.Extensions` and emitted in
`struktly/tasks/v1`.

## The strict profile, and why it is still conformant

Struktly validates its own `type: task` concepts strictly: `status` and
`priority` have closed vocabularies, `id` must match the filename, and a body
must carry the contract headings.

That is a **domain profile over a permissive base**, which is what §4.1's
"minimally opinionated" design is for. Two things keep it inside the spec:

- Discovery reports rather than rejects. `DiscoverTasks` returns valid tasks and
  invalid files side by side, so one malformed task never hides its siblings and
  the bundle is never refused as a whole.
- The strictness applies to concepts claiming `schema: struktly/task/v1`, not to
  the bundle. An unknown `type` is not ours to judge and is not rejected.

`LoadTasks` remains strict and fails on the first invalid task. It is the
internal, single-purpose path; `DiscoverTasks` is what the machine contract and
the desktop application use.

## Recommended fields we do not yet emit

§4.1 recommends `title`, `description`, `resource` and `tags`. Tasks carry
`title`. The rest are open questions rather than omissions — `description` in
particular would duplicate the mission paragraph unless it is derived.

## Not yet decided

Whether context packets should also be OKF bundles. A packet is curated context
for an agent, so the fit is obvious and the consequences are not: packets are
byte-pinned and hashed, and a format someone else versions is a different
compatibility surface from one this repository controls. Recorded here so the
question is visible rather than answered by drift.
