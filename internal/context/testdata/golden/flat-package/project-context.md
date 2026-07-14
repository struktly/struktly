---
type: project-context
schema: struktly/project-context/v1
title: "Repository context: flat-package"
description: "Local summary of repository files, commands, and guidance."
timestamp: $TIMESTAMP
---

# Repository context

Generated locally from repository files and Git metadata.

## Repository

- Repository name: flat-package
- Repository root: `.`

## Top-level directories

- No top-level directories detected.

## Languages and frameworks

- Go

## Build and test commands

- `go test ./...`
- `go build ./...`
- `go vet ./...`

## Documentation

- `README.md`

## Files excluded from context

- Git-ignored files and `.git` internals.
- Dependencies, build output, caches, and generated runtime state.
- Common credential files and secret-looking filenames.
- Symlinks, non-regular files, binaries, and invalid UTF-8 text.
- File discovery: Outside Git, root-level exact, directory, and glob .gitignore patterns are applied; negation and full Git semantics require a Git repository.

## Sources

- `README.md`
- `go.mod`
