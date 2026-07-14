---
type: context-packet
schema: struktly/packet/v1
title: "Context: add request timeout middleware"
description: "Repository files and guidance selected for this task."
timestamp: $TIMESTAMP
---

# Context packet

Generated locally from repository files and Git metadata.

## Task

add request timeout middleware

## Packet details

- Schema: `struktly/packet/v1`
- Packet hash: `sha256:ebae81f1047eb8ccd667ab92a3a0d683bc131c7f0cf34e5f76ebf207e9bf9038`
- Repository: `git:1168f74c9e90c15390b4e6f53975a8471636ca941879d877eb982e4945447bd1`
- Branch: `main`
- HEAD revision: `51632cd3c819af13d20ca87974e07f214f2797e1`

## Repository

- Repository name: noisy-legacy
- Repository root: `.`

## Top-level directories

- `.struktly`
- `_legacy`
- `archive`
- `docs`
- `legacy`
- `testdata`

## Languages and frameworks

- Go

## Required checks

- No required checks are configured.

## Suggested checks

- `go build ./...`
- `go test ./...`
- `go vet ./...`
- `make test`

## Relevant documentation

- `README.md`
- `docs/current.md`

## Files to inspect

- `README.md`

## Included files

### `Makefile`

- Type: Manifest
- Why it was included: matched a repository context rule
- Content hash: `sha256:57d3973f34968810f6110671fdcdea344c0cccdd329118e67184e5dbaa5125ce`
- Bytes: `21/21`

```text
test:
	go test ./...
```

### `README.md`

- Type: Documentation
- Why it was included: matched a repository context rule
- Content hash: `sha256:101b7771d85d2fe6d76bcc745259b5eb719a5c384760b12fcc929de6228a769d`
- Bytes: `82/82`

```text
# Noisy Legacy

A service with stale directories used as a Struktly scan fixture.
```

### `go.mod`

- Type: Manifest
- Why it was included: matched a repository context rule
- Content hash: `sha256:72f802eedaabfd858e4c10b5c12ab57b028689f265973a7bc22fbdc97ba2fe09`
- Bytes: `43/43`

```text
module example.com/noisy-legacy

go 1.24.0
```

## Sources

- `Makefile`
- `README.md`
- `docs/current.md`
- `go.mod`
