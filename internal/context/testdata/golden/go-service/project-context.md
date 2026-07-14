---
type: project-context
schema: struktly/project-context/v1
title: "Repository context: go-service"
description: "Local summary of repository files, commands, and guidance."
timestamp: $TIMESTAMP
---

# Repository context

Generated locally from repository files and Git metadata.

## Repository

- Repository name: go-service
- Repository root: `.`

## Top-level directories

- `.struktly`
- `docs`
- `middleware`

## Languages and frameworks

- Go

## Build and test commands

- `go test ./...`
- `make test`
- `go build ./...`
- `make build`
- `go vet ./...`

## Documentation

- `README.md`
- `docs/architecture.md`
- `docs/adr/0001-record.md`

## Decision records

- `docs/adr/0001-record.md`

## Files excluded from context

- Git-ignored files and `.git` internals.
- Dependencies, build output, caches, and generated runtime state.
- Common credential files and secret-looking filenames.
- Symlinks, non-regular files, binaries, and invalid UTF-8 text.
- File discovery: Outside Git, root-level exact, directory, and glob .gitignore patterns are applied; negation and full Git semantics require a Git repository.

## Repository direction

Source: `.struktly/direction.md`

# Direction

Ship a reliable payments API with strict timeouts on every outbound call.

## Non-goals

- No new public endpoints until the middleware chain is hardened.

## Sources

- `Makefile`
- `README.md`
- `go.mod`
- `docs/architecture.md`
- `docs/adr/0001-record.md`
