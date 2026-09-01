# Working in this repository

`struktly` is the part of Struktly that can be checked from outside. It decides
what a coding agent is given — a context packet recording what was selected,
what was excluded, and why — and it verifies that an exported Record is the one
that was sealed. The repository is public so both can be audited by anyone.

## Read before changing anything

- [`README.md`](README.md) — what the tool is, and what a packet looks like.
- [`CONTRIBUTING.md`](CONTRIBUTING.md) — build, lint, test, release.
- [`docs/compatibility.md`](docs/compatibility.md) — schema policy, and the
  contract a consumer negotiates.
- [`docs/integration-contract.md`](docs/integration-contract.md) — commands,
  streams, exit codes, security.
- [`docs/roadmap.md`](docs/roadmap.md) — the scope boundary, and what is
  deliberately not here.

This repository also declares its own direction and constraints under
[`.struktly/`](.struktly), in the same files the CLI reads from any other
repository. They are short on purpose and they are the current answer; the
documents above are the reasoning behind it.

## What must stay true

A test holds each of these; the one held only by review says so.

- **No network.** The CLI reads a repository and writes a file. It does not call
  a model, upload source, or fetch anything.
- **One dependency.** cobra, and the pflag and mousetrap it brings. Prefer the
  standard library, and prefer doing without.
- **Deterministic output.** The same request on the same revision produces the
  same `packet_hash`. Nothing may depend on wall-clock time, map iteration
  order, or the order a filesystem happens to return entries in.
- **Exclusions are load-bearing.** Detected secrets, sensitive names, binaries,
  symlinks, Git-ignored files, dependencies, build output, and oversized content
  stay out. Loosening one is a security change, not a tuning change.
- **Opinions come from the repository, not from Go.** Direction and constraints
  are read from `.struktly/direction.md`, `constraints.md`, and `decisions.md`.
  Never compile one repository's opinions into a string literal that every other
  repository would then inherit. Held by review.
- **Machine surfaces are versioned, and `capabilities` describes this binary.**
  An advertised command must resolve in the command tree, an advertised schema
  must be a published file, and removing anything a consumer negotiates is a
  breaking change; each fails a test. The feature list is not yet held this
  way — [`.struktly/tasks/bind-features-to-proof.md`](.struktly/tasks/bind-features-to-proof.md)
  is that work.
- **Writes stay under `.struktly/`,** and `--no-write` means nothing is written
  at all. Product state — chats, executions, approvals, evidence, memory — is
  the desktop product's, not this repository's.

## Verifying

```sh
make lint
make test
make build
```

`make test` takes a few minutes; the selection corpus dominates it. For a narrow
change, `go test ./internal/context` or `go test ./cmd/struktly` is the useful
subset, but run the whole suite before opening a pull request. A change to what
gets selected should be measured against the labelled corpus in
`internal/context/testdata/corpus`, not argued about.

`mise.toml` pins the Go toolchain; `mise install` matches what CI builds with.
The `Makefile` is the stable entry point either way.

The supported Go floor is the `go` directive in `go.mod` and nothing else. The
mise pin, the README badge, the README prose and `CONTRIBUTING.md` follow it,
`toolchain_test.go` fails when they disagree, and
[`.struktly/decisions.md`](.struktly/decisions.md) says why the floor is where
it is. Raising it is a deliberate change, not a consequence of your local
toolchain.
