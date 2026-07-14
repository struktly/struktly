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
- Packet hash: `sha256:bd0a1b90ad07ddb8aa1da480323258e91762f674795fde44d2d823d22f4979b2`
- Repository: `git:0e709b14ea2e846e54e1d38161502ee8f4a49a40963cbbe8abb58b938ca7a1d7`
- Branch: `main`
- HEAD revision: `983c7b8424a771fe26dbd36f266a305acf674d48`

## Repository

- Repository name: go-service
- Repository root: `.`

## Top-level directories

- `.struktly`
- `docs`
- `middleware`

## Languages and frameworks

- Go

## Direction

From `.struktly/direction.md`:

# Direction

Ship a reliable payments API with strict timeouts on every outbound call.

## Non-goals

- No new public endpoints until the middleware chain is hardened.

## Constraints

From `.struktly/constraints.md`:

# Constraints

- Keep handler latency under 200ms.
- No new third-party dependencies without a decision record.

## Required checks

- No required checks are configured.

## Suggested checks

- `go build ./...`
- `go test ./...`
- `go vet ./...`
- `make build`
- `make test`

## Relevant documentation

- `README.md`
- `docs/adr/0001-record.md`
- `docs/architecture.md`

## Files to inspect

- `README.md`
- `docs/adr/0001-record.md`
- `middleware/`
- `middleware/timeout.go`

## Included files

### `.struktly/constraints.md`

- Type: Declaration
- Why it was included: matched a repository context rule
- Content hash: `sha256:81564faaa40d080d602077a0bfc444e5599f68f165e1c603c89ee505e5292485`
- Bytes: `112/112`

```text
# Constraints

- Keep handler latency under 200ms.
- No new third-party dependencies without a decision record.
```

### `.struktly/direction.md`

- Type: Declaration
- Why it was included: matched a repository context rule
- Content hash: `sha256:1fff95fc45cbaa0c04fe78855816cc3d507df5e85b3b1c097ec98a5e12af0057`
- Bytes: `308/308`

```text
---
type: direction
title: "Repository Direction"
description: "Direction for the go-service fixture."
timestamp: $TIMESTAMP
---

# Direction

Ship a reliable payments API with strict timeouts on every outbound call.

## Non-goals

- No new public endpoints until the middleware chain is hardened.
```

### `Makefile`

- Type: Manifest
- Why it was included: matched a repository context rule
- Content hash: `sha256:e450912e118fd02bee73756f442a14547c1f17ce479dc9ab319b985468514bb9`
- Bytes: `45/45`

```text
test:
	go test ./...

build:
	go build ./...
```

### `README.md`

- Type: Documentation
- Why it was included: matched a repository context rule
- Content hash: `sha256:3358d5eb78ed93bd2419ed728bc1049ce2a689cb87ca55325ae0ceb4f48c962b`
- Bytes: `125/125`

```text
# Go Service

A small payments API used as a Struktly scan fixture.

## Development

Run `make test` before sending changes.
```

### `go.mod`

- Type: Manifest
- Why it was included: matched a repository context rule
- Content hash: `sha256:f6c2e852e2767de6925f0cdffc43aceb1ccd9192f70473b0f0407448a4886ce1`
- Bytes: `41/41`

```text
module example.com/go-service

go 1.24.0
```

### `middleware/timeout.go`

- Type: Source
- Why it was included: its filename matched the task
- Content hash: `sha256:1ddd4a28247cbd327889ea8e97584abab79c8074864dbc715869bcee82602f7d`
- Bytes: `283/283`

```text
// Package middleware provides HTTP middleware for the service.
package middleware

import (
	"net/http"
	"time"
)

// Timeout wraps h with a fixed request timeout.
func Timeout(h http.Handler, d time.Duration) http.Handler {
	return http.TimeoutHandler(h, d, "request timed out")
}
```

## Sources

- `.struktly/constraints.md`
- `.struktly/direction.md`
- `Makefile`
- `README.md`
- `docs/adr/0001-record.md`
- `docs/architecture.md`
- `go.mod`
