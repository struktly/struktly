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
runtime state. Repository writes belong under `.struktly/`; run state and
unapproved memory remain outside the repository.

## Reports

For a selection problem, provide a minimal repository layout, the command, and
the expected and actual result. Do not include private source or credentials.

Report vulnerabilities through the private route in [`SECURITY.md`](SECURITY.md).
Project participation is governed by [`CODE_OF_CONDUCT.md`](CODE_OF_CONDUCT.md).
