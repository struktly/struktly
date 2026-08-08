---
type: context-packet
schema: struktly/packet/v2
title: "Context: add request timeout middleware"
description: "Repository files and guidance selected for this task."
timestamp: $TIMESTAMP
---

# Context packet

Generated locally from repository files and Git metadata.

## Task

add request timeout middleware

## Packet details

- Schema: `struktly/packet/v2`
- Packet hash: `sha256:7d15b90f6e98d6e24df04708c31b31cb48f61dfd31226a9f9149d0f7f8c6ec8d`
- Repository: `git:9e32ee6a0f32d5c9fb24e98545a74619da682cba8248d369648c707ad8112c40`
- Branch: `main`
- HEAD revision: `bbf50c1ba03f95b993b74ece34f282d51deb63dd`
- Scope: `whole repository`

## Repository

- Repository name: go-service
- Repository root: `.`

## Top-level directories

- `.struktly`
- `docs`
- `internal`
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

### `internal/clock/clock.go`

- Type: Source
- Why it was included: a selected file imports it
- Content hash: `sha256:7f86d528f399fab5677a123f8fcee25b6ed6d7b4311a3c1df073682c183555a9`
- Bytes: `272/272`

```text
// Package clock supplies the wall-clock source middleware depends on.
package clock

import "time"

// Grace is the deadline applied when none is configured.
const Grace = 30 * time.Second

// Wall returns the current instant.
func Wall() time.Time { return time.Now() }
```

### `middleware/logger.go`

- Type: Source
- Why it was included: its filename matched the task
- Content hash: `sha256:79ab9481be7e91f1130665b0c50407af6c808a804d625e2d625837b3d517f21a`
- Bytes: `281/281`

```text
package middleware

import (
	"log"
	"net/http"
)

// Logger logs each request method and path.
func Logger(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		h.ServeHTTP(w, r)
	})
}
```

### `middleware/timeout.go`

- Type: Source
- Why it was included: its filename matched the task
- Content hash: `sha256:e59afd85a547cb059a77ab04fcfe46c2e2cc1b0697ae07cc12ac5a7c7f16c1bd`
- Bytes: `310/310`

```text
// Package middleware provides HTTP middleware for the service.
package middleware

import (
	"net/http"

	"example.com/go-service/internal/clock"
)

// Timeout wraps h with a fixed request timeout.
func Timeout(h http.Handler) http.Handler {
	return http.TimeoutHandler(h, clock.Grace, "request timed out")
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
