# Working in this repository

This service drains a durable queue and retries with backoff. The retry policy
is the part people get wrong: a retry must be idempotent, and the backoff is
capped rather than unbounded.

Run `go test ./...` before proposing a change.
