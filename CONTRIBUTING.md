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

Keep changes small and include tests for behavior changes. Do not add a runtime
dependency when the standard library or an existing dependency is sufficient.

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

There is no build artifact. Struktly is installed with
`go install github.com/struktly/struktly/cmd/struktly@vX.Y.Z`, so the tag is the
release: the module proxy serves it and `struktly version` reports it from the
build information Go records at install time.

The current version lives in
[`.release-please-manifest.json`](.release-please-manifest.json) rather than
being derived from tags, because `v0.2.1` was published from a commit that is
not an ancestor of `main` and tag-derived versioning would read the history
wrongly.

Everything a release depends on runs on every push to `main`, not on the release
pull request: race tests, the gitleaks history scan, and an install smoke test
against a clean repository. release-please opens its pull request with
`GITHUB_TOKEN`, and GitHub does not start workflows for those, so a check that
only ran on pull requests would never see the commit being tagged.

The changelog for a release is generated from commit subjects. Write them for a
reader of the release notes, and put the reasoning in the commit body, where it
stays attached to the change. A release pull request can be edited before it is
merged if a release deserves a fuller note than its subjects give it.

## Reports

For a selection problem, provide a minimal repository layout, the command, and
the expected and actual result. Do not include private source or credentials.

Report vulnerabilities through the private route in [`SECURITY.md`](SECURITY.md).
Project participation is governed by [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md).
