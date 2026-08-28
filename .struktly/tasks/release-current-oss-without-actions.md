---
type: task
schema: struktly/task/v1
id: release-current-oss-without-actions
title: "Publish current OSS release assets without hosted Actions"
status: in-progress
lane: release
priority: critical
created: 2026-08-28
updated: 2026-08-28
agent: "13 OSS CLI / Integrations"
---

# Publish current OSS release assets without hosted Actions

## Objective

Make the local release runner publish the exact deterministic OSS asset set that
Platform consumes when hosted GitHub Actions cannot start.

## Assignment

- Task: `release-current-oss-without-actions`
- Owner: lane 13 OSS CLI / Integrations
- Base: OSS `origin/main` at `1c8fd378`
- Branch/worktree: `fix/local-release-assets` in
  `/Users/sdf/code/.struktly-worktrees/oss-local-release-assets`
- Dependencies: accepted portable Record producer/consumer tranche; working
  maintainer GitHub authentication
- Merge order: merge this release-flow correction after Platform's independent
  artifact-selection repair; cut the OSS release before updating Platform's
  pinned CLI identity

## Problem

The local release runner can create the release-please pull request, tag and
draft, but it does not reproduce the release-assets workflow. With hosted
Actions unavailable, the new tag would have no deterministic binaries for
Platform to pin.

## Done when

- A local `assets TAG` stage verifies the exact tag, draft state and release
  target before building anything.
- It builds the macOS ARM and two Linux release binaries in an isolated exact-tag
  checkout, with checksum manifests and embedded version/revision identity.
- It uploads exactly six assets, verifies that exact count, and publishes only
  after upload succeeds.
- The operation is safe to retry while the release is a draft.
- `all` runs check, prepare, assets and publication in that order.
- An explicit administrator merge switch is available only for the account-level
  hosted-check outage; the normal merge path remains unchanged.
- Release binary contracts, shell syntax, ShellCheck and the full Go gate pass
  in a clean toolchain environment.

## Verification

- `env -u GOROOT GOCACHE="$(mktemp -d)" ./scripts/test-release-binaries.sh`
- `sh -n scripts/release-local.sh`
- `shellcheck scripts/release-local.sh scripts/package-release-binary.sh scripts/test-release-binaries.sh`
- `git diff --check`
