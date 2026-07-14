---
type: project-context
schema: struktly/project-context/v1
title: "Repository context: noisy-legacy"
description: "Local summary of repository files, commands, and guidance."
timestamp: $TIMESTAMP
---

# Repository context

Generated locally from repository files and Git metadata.

## Repository

- Repository name: noisy-legacy
- Repository root: `.`

## Top-level directories

- `_legacy`
- `archive`
- `docs`
- `legacy`
- `testdata`

## Languages and frameworks

- Go

## Build and test commands

- `go test ./...`
- `make test`
- `go build ./...`
- `go vet ./...`

## Documentation

- `README.md`
- `docs/current.md`

## Files excluded from context

- Git-ignored files and `.git` internals.
- Dependencies, build output, caches, and generated runtime state.
- Common credential files and secret-looking filenames.
- Symlinks, non-regular files, binaries, and invalid UTF-8 text.
- File discovery: Outside Git, root-level exact, directory, and glob .gitignore patterns are applied; negation and full Git semantics require a Git repository.

## Deprioritized paths

These directories look archived or fixture-only, so their docs and commands were not treated as current repository guidance:

- `_legacy`
- `archive`
- `legacy`
- `testdata`

## Sources

- `Makefile`
- `README.md`
- `go.mod`
- `docs/current.md`
