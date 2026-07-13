---
type: task
schema: struktly/task/v1
id: open-source-readiness
title: "Prepare the Struktly CLI repository for public open source"
status: in-progress
priority: high
created: 2026-07-13
updated: 2026-07-13
agent: codex
agent_model: "GPT-5.6 Sol"
reasoning_effort: high
agent_session: 019f5ccd-9e2c-7c02-9eda-f738110b09ee
resume_command: "codex resume 019f5ccd-9e2c-7c02-9eda-f738110b09ee"
---

# Prepare the Struktly CLI repository for public open source

Status: In progress

Priority: Release blocker

Created: 2026-07-13

Task file: `.struktly/tasks/open-source-readiness.md`

## Pick up this task

From the repository checkout, resume the implementation session:

```sh
codex resume 019f5ccd-9e2c-7c02-9eda-f738110b09ee
```

This is an implementation task, not another audit. Continue through the highest
priority coherent slices, verify each slice, and leave the repository ready for a
deliberate visibility change. Do not change repository visibility, rewrite shared
history, publish a release, or alter GitHub organization settings without explicit
approval at the relevant step.

## Objective

Make `struktly/struktly` safe, coherent, and presentable as the independently
useful MIT-licensed CLI described in the README:

> The local, Git-native control plane for coding-agent work.

The public repository must be self-contained for users and contributors. The
private platform may consume the CLI and its stable formats, but public contributors
must not need private repositories, private product plans, or machine-local context
to understand or evolve the CLI.

## Constraints

- Treat this as implementation work, not another audit.
- Keep the public CLI independently useful without the private platform.
- Keep portable declarations and task handoffs under `.struktly/`; keep sessions,
  credentials, caches, logs, and other runtime state outside the repository.
- Obtain explicit approval immediately before rewriting shared history, publishing
  a release, changing GitHub settings, or changing repository visibility.

## Required outcomes

- [ ] Current files and reachable Git history are approved for public disclosure.
- [ ] Public direction, constraints, ADRs, contributor guidance, and generated
      dogfood artifacts describe one coherent CLI product boundary.
- [ ] No tracked file contains private checkout paths or requires unavailable
      private documentation.
- [ ] `go install ...@latest` installs the behavior documented by the README.
- [ ] Security reporting, community health files, CI hardening, and branch
      protections meet the launch baseline below.
- [ ] A clean-environment installation and end-to-end CLI smoke test pass from the
      release candidate tag.
- [ ] The final dedicated secret/history scan reports no unreviewed findings.

## Starting assessment

Do not make the repository public yet.

The CLI implementation is credible, independently useful, and suitable for open
source. The repository needs a focused release-readiness pass before its visibility
changes. The main blockers are historical disclosure, contradictory canonical
documentation, an outdated public release, and incomplete security/community
infrastructure.

| Area | Status |
|---|---|
| CLI implementation | Ready |
| Public product narrative | Ready in the current worktree |
| Git history and privacy | Blocked |
| Release mechanics | Blocked |
| Community and security hygiene | Repository files ready; GitHub settings pending |

## What is ready

- MIT-licensed, standalone, local-first CLI with no required account, server,
  model provider, or network call.
- Deterministic, versioned JSON schemas and stable packet identity.
- Git-authoritative ignore handling, explicit security exclusions, provenance,
  content hashes, truncation reporting, and structured errors.
- Low dependency count and a clear standard-library-first policy.
- Full tests, race tests, module verification, lint, build, golden determinism,
  schema parsing, and diff checks pass.
- Current-tree and history regex checks found no apparent credential material
  outside the intentional secret-detection test fixture.

The appropriate positioning is:

> An independent MIT-licensed Git-native CLI and context format, consumed by—but
> not subordinate to—a separate private platform.

## Release blockers

### 1. Decide what history may become public

The 34-commit history contains removed platform implementation, including:

- `cmd/struktly-server/`
- `internal/api/`
- `internal/repositories/`
- `internal/storage/sqlite/`
- the removed React/Vite `web/` application

Changing repository visibility exposes the complete reachable history, not only
the current tree. Either explicitly approve that implementation for open source or
create a clean public root history from the current CLI tree. Preserve the original
private history elsewhere. Retarget or remove old tags so they do not keep the old
history reachable.

After rewriting, run a dedicated history scanner such as `gitleaks` or
`trufflehog`. Also confirm that the commit author email is intended to be public.

GitHub documents that public repository contents become visible and forkable:
<https://docs.github.com/en/repositories/managing-your-repositorys-settings-and-features/managing-repository-settings/setting-repository-visibility>.

### 2. Make public repository direction authoritative for its scope

The public direction, constraints, contributor guidance, ADRs, roadmap, and
tracked generated drafts must describe the implemented CLI contract only. Remove
legacy product planning from this repository, regenerate dogfood artifacts, and
keep contributors independent of any private checkout or unavailable plan.

### 3. Publish a release matching the README

The README installs:

```sh
go install github.com/struktly/struktly/cmd/struktly@latest
```

The latest tag is still `v0.1.0`, while the current schemas, packet behavior,
inspection commands, structured errors, and state boundary are listed under
`Unreleased`. A new user therefore does not install the product described by the
README.

Finalize the changelog and cut a matching release, likely `v0.4.0` based on the
compatibility policy. Add a `struktly version` command with build metadata before
supporting public bug reports.

### 4. Remove local and private context from tracked files

Generated portable artifacts must not contain an absolute checkout path. Use a
repository-relative root or omit the absolute path. Avoid committing volatile
machine-local exports unless they are intentional fixtures.

The README also links to private repositories that return no useful public page.
Either explain the commercial/private boundary without inaccessible links or add
public landing pages for those repositories.

### 5. Confirm legal ownership

The MIT license is suitable, but confirm that `Struktly` is the correct legal
copyright holder and that all code extracted from `froppa/struktly.io` is owned by
the releasing person or entity. Review any platform code retained in public history
as part of that decision.

## Security and community launch requirements

Before changing visibility:

- Add `SECURITY.md` with supported versions and a private reporting route.
- Enable GitHub private vulnerability reporting, secret scanning, push protection,
  Dependabot alerts, and security updates.
- Add a Code of Conduct and link it from the README or contributing guide.
- Add issue forms for bugs and context-quality reports plus a pull-request template.
- Correct `CONTRIBUTING.md`; it still promises no writes outside `.struktly/`,
  which conflicts with the external runtime-state boundary.
- Protect `main`, require CI, require conversation resolution, and disable force
  pushes.

The Open Source Guide's baseline includes a license, README, contributing guide,
and Code of Conduct:
<https://opensource.guide/starting-a-project/>.

GitHub recommends a repository security policy and private vulnerability reporting:

- <https://docs.github.com/en/code-security/getting-started/quickstart-for-securing-your-repository>
- <https://docs.github.com/en/code-security/how-tos/report-and-fix-vulnerabilities/configure-vulnerability-reporting/configure-for-a-repository>

## CI and supply-chain hardening

- Give the workflow an explicit `permissions: contents: read` boundary.
- Pin third-party actions to reviewed full commit SHAs.
- Update `actions/setup-go` and test patched `1.25.x` plus current stable Go.
- Add macOS and Windows coverage; filesystem, path, Git, and user-config behavior
  are platform-sensitive.
- Add `govulncheck ./...` and a dedicated secret-history scan to the release gate.
- Configure Dependabot for Go modules and GitHub Actions.
- If binary releases are added, publish checksums, an SBOM, provenance attestations,
  and signed release artifacts.

GitHub recommends least-privilege workflow tokens and full-SHA action pinning:

- <https://docs.github.com/en/actions/how-tos/security-for-github-actions/security-guides/use-github_token-in-workflows>
- <https://docs.github.com/en/actions/reference/security/secure-use>

## Execution plan

1. Preserve the private repository and create an approved clean public history.
2. Rewrite public direction, constraints, contributor guidance, and roadmap.
3. Remove private/local paths and regenerate intentional dogfood artifacts.
4. Add security, conduct, issue, and pull-request policies.
5. Harden CI and configure repository security and branch rules.
6. Run tests, race tests, `govulncheck`, and full history secret scanning.
7. Finalize the changelog and tag the release matching the README.
8. Verify installation from the tag in a clean environment.
9. Change visibility and immediately inspect the public community profile,
   dependency graph, release page, package documentation, and rendered README.

Steps 1, 7, and 9 change shared history or public external state. Obtain explicit
approval immediately before executing each one.

## Definition of done

The task is complete when:

1. Every required outcome above is checked with recorded evidence.
2. Repository-local format, lint, test, race, build, schema, vulnerability, and
   secret-history checks pass.
3. The README, command help, installed release, generated context, and public
   repository metadata agree on what Struktly does today.
4. No public contributor workflow depends on another private checkout.
5. `main` is protected and the approved release commit is reproducibly installable.
6. A maintainer has explicitly approved the history rewrite (if chosen), release,
   and visibility change.

Once these items are complete, the repository is representable as a serious public
alpha and aligned with current open-source expectations.
