# Contributing

Bug reports and focused pull requests are welcome. Open an issue before changing
a command, schema, compatibility rule, or the documented scope of the CLI.

## Development

Struktly requires Go 1.25 or newer and Git.

```sh
make lint
make test
make build
```

`go.mod` is the authoritative statement of that floor. `mise.toml` pins it, so
`mise install` gives you the toolchain this repository is developed against;
mise is optional and the `Makefile` is the entry point with or without it. The
badge, the sentence above and the mise pin all follow `go.mod`, and a test fails
when one of them is edited alone.

`make lint` builds the golangci-lint pinned in `tools/go.mod` into `.bin/` the
first time it is needed, so there is nothing to install beforehand and CI runs
that same binary against that same configuration. Formatting is gofumpt and is
reported by the same run; `make fmt` applies it.

Keep changes small and include tests for behavior changes. Do not add a runtime
dependency when the standard library or an existing dependency is sufficient;
`boundary_test.go` holds the installed binary to cobra and what cobra brings,
so a new one fails until it is named there with its reason.

## Compatibility

JSON is the machine-readable interface. Changes within a schema version must be
additive; breaking changes require a new schema version. See
[`docs/compatibility.md`](docs/compatibility.md).

Generated context must remain deterministic. It must respect Git ignores and
must not include secrets, binaries, symlinks, dependencies, build output, or
runtime state. Repository writes belong under `.struktly/`; product state belongs
to Struktly Platform rather than this CLI.

## Releasing

Releases are automated by
[release-please](https://github.com/googleapis/release-please). Every push to
`main` updates a release pull request; merging it tags the commit and publishes
the GitHub release. Nobody picks a version by hand — it comes from the
conventional-commit subjects since the last release, and while the project is
pre-1.0 a `!` breaking change bumps the minor rather than the major.

Struktly remains installable with
`go install github.com/struktly/struktly/cmd/struktly@vX.Y.Z`. Each stable
release also carries checksum-manifested binaries for the targets consumed by
Struktly Platform. They are built from the release tag with the exact version
and revision embedded, and are never overwritten. The normal path uses the
public Ubuntu runner. When hosted Actions cannot start, an authenticated
maintainer runs `scripts/release-local.sh all`; it executes the same gates,
creates the release-please tag and draft, builds all three deterministic
targets locally, verifies the six-asset set, and only then publishes it.
The laptop path pins `release-please@17.6.0`, matching the pinned v5 Action,
rather than resolving a different release algorithm at execution time.

The current version lives in
[`.release-please-manifest.json`](.release-please-manifest.json) rather than
being derived from tags, because `v0.2.1` was published from a commit that is
not an ancestor of `main` and tag-derived versioning would read the history
wrongly.

Everything a release depends on runs on every push to `main`, not on the release
pull request: race tests, a govulncheck scan, the gitleaks history scan, and an
install smoke test against a clean repository. release-please opens its pull
request with `GITHUB_TOKEN`, and GitHub does not start workflows for those, so a
check that only ran on pull requests would never see the commit being tagged.
Those checks run on pull requests as well, because a secret that reaches the
history of `main` fails the scan on every run afterwards.

The changelog for a release is generated from commit subjects. Write them for a
reader of the release notes, and put the reasoning in the commit body, where it
stays attached to the change. A release pull request can be edited before it is
merged if a release deserves a fuller note than its subjects give it.

## Reports

For a selection problem, provide a minimal repository layout, the command, and
the expected and actual result. Do not include private source or credentials.

Report vulnerabilities through the private route in [`SECURITY.md`](SECURITY.md).
Project participation is governed by [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md).
