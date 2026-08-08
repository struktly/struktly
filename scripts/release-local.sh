#!/usr/bin/env sh
set -eu

# Runs struktly/struktly's release from a local machine instead of GitHub
# Actions, with the same stages and the same gates.
#
# Two things make this more than a convenience. The organization forbids
# GitHub Actions from creating pull requests, so `release.yml` gets as far as
# pushing the release branch and then cannot open the pull request — a release
# PR raised from here carries your own token instead, and opens. And a pull
# request opened by GITHUB_TOKEN does not start workflows at all, so one raised
# from here is also the only version of it that gets checked.
#
# Subcommands mirror the pipeline's stages, in order, and are each safe to
# re-run alone if an earlier one already succeeded:
#
#   check     everything CI runs before a release is allowed to exist: lint,
#             tests, the race detector, the secret history scan, govulncheck,
#             the CLI against this repository's own declarations, and an
#             install smoke test in a clean repository. Same as ci.yml's
#             `test` and `release-readiness` jobs. Changes nothing.
#   prepare   release-please release-pr -> you review -> merge -> release-please
#             github-release. Produces the version bump commit, the tag, and
#             the published GitHub release. Same as release.yml.
#   all       check, then prepare. What you want for "just cut the release".
#
# There is nothing to build and attach. Struktly is installed with
# `go install github.com/struktly/struktly/cmd/struktly@vX.Y.Z`, so the tag is
# the release: the module proxy serves it and `struktly version` reports it
# from the build information Go records at install time.
#
# Requires: gh (authenticated, with write access to struktly/struktly), git,
# jq, bun, and go.
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

owner=struktly
repo=struktly
repo_url="https://github.com/$owner/$repo"
# The module path kept zricethezav after the project moved to the gitleaks
# organization, so the import path and the repository URL disagree.
gitleaks_module=github.com/zricethezav/gitleaks/v8@v8.30.1

if [ -t 1 ]; then
  C_RESET='\033[0m'; C_DIM='\033[2m'; C_BOLD='\033[1m'
  C_BLUE='\033[34m'; C_GREEN='\033[32m'; C_RED='\033[31m'
else
  C_RESET=''; C_DIM=''; C_BOLD=''; C_BLUE=''; C_GREEN=''; C_RED=''
fi
log_step() { printf '%b%s%b %s\n' "${C_BLUE}${C_BOLD}" "==>" "${C_RESET}" "$1"; }
log_ok()   { printf '%b%s%b %s\n' "${C_GREEN}" "ok " "${C_RESET}" "$1"; }
log_info() { printf '%b%s%b %s\n' "${C_DIM}" "  ." "${C_RESET}" "$1"; }
fail()     { printf '%b%s%b release-local: %s\n' "${C_RED}" "!! " "${C_RESET}" "$1" >&2; exit 1; }

require_tools() {
  for tool in "$@"; do
    command -v "$tool" >/dev/null 2>&1 || fail "required tool not found: $tool"
  done
}

require_clean_main() {
  [ -z "$(git status --porcelain)" ] || fail "working tree is dirty; commit, stash, or discard changes first"
  branch=$(git rev-parse --abbrev-ref HEAD)
  [ "$branch" = "main" ] || fail "checked out branch is '$branch', expected 'main'"
}

gh_token() { gh auth token; }

# Where `go install` actually puts things. GOBIN wins over GOPATH/bin when it is
# set, and assuming GOPATH/bin is how a script works on the machine it was
# written on and nowhere else.
go_bin() {
  bin=$(go env GOBIN)
  [ -n "$bin" ] || bin="$(go env GOPATH)/bin"
  printf '%s' "$bin"
}

# --- check: the gates a release has to survive (ci.yml) ---

cmd_check() {
  require_tools go git
  log_step "lint"
  make lint
  log_step "tests"
  go test ./...
  log_step "build"
  go build ./...
  log_step "the CLI against this repository's own declarations"
  go run ./cmd/struktly validate
  go run ./cmd/struktly doctor >/dev/null
  log_step "race detector"
  go test -race ./...
  log_step "vulnerability check"
  go run golang.org/x/vuln/cmd/govulncheck@v1.6.0 ./...

  # The gitleaks CLI is MIT and needs no key; only its GitHub Action requires a
  # licence for organization-owned repositories. Installed by version so the Go
  # checksum database verifies it. History, not the working tree: a secret
  # removed from HEAD is still readable in the commit that added it.
  log_step "secret history scan"
  # `go run`, exactly as ci.yml does it: no install step, nothing left on the
  # PATH afterwards, and the checksum database still verifies what it fetched.
  go run "$gitleaks_module" git --config .gitleaks.toml --redact --verbose .

  log_step "install smoke test in a clean repository"
  go install ./cmd/struktly
  smoke_root=$(mktemp -d)
  # Removed on the way out whether or not the smoke test passed; a failure
  # leaves the log, not a directory nobody remembers creating.
  trap 'rm -rf "$smoke_root"' EXIT
  git -C "$smoke_root" init --quiet
  git -C "$smoke_root" config user.name "Struktly release-local"
  git -C "$smoke_root" config user.email "release-local@struktly.invalid"
  printf '# smoke repository\n' > "$smoke_root/README.md"
  git -C "$smoke_root" add README.md
  git -C "$smoke_root" commit --quiet -m initial
  struktly_bin="$(go_bin)/struktly"
  "$struktly_bin" init --root "$smoke_root"
  "$struktly_bin" context --root "$smoke_root" --json --no-write "smoke test" > /dev/null
  "$struktly_bin" validate --root "$smoke_root"
  "$struktly_bin" doctor --root "$smoke_root" > /dev/null
  rm -rf "$smoke_root"
  trap - EXIT

  log_ok "every gate CI runs passed here"
}

# --- prepare: version bump, tag, GitHub release (release.yml) ---

cmd_prepare() {
  require_tools git gh jq bun go
  require_clean_main
  log_step "syncing main"
  git fetch origin main
  git reset --hard origin/main

  token=$(gh_token)

  log_step "creating/updating the release-please PR"
  # The same config the workflow uses, so the version this computes locally is
  # the version CI would have computed. A local run that used different rules
  # would be a second release process wearing the first one's name.
  bunx release-please release-pr \
    --repo-url="$repo_url" \
    --token="$token" \
    --config-file=release-please-config.json \
    --manifest-file=.release-please-manifest.json \
    --target-branch=main

  pr_json=$(gh pr list -R "$owner/$repo" --state open --json number,headRefName,url,title \
    --jq '[.[] | select(.headRefName | startswith("release-please--"))][0]')
  [ "$pr_json" != "null" ] && [ -n "$pr_json" ] || fail "release-please did not open a release PR (no pending release-worthy commits on main?)"
  pr_number=$(printf '%s' "$pr_json" | jq -r '.number')
  pr_url=$(printf '%s' "$pr_json" | jq -r '.url')
  pr_title=$(printf '%s' "$pr_json" | jq -r '.title')

  log_ok "release PR ready: $pr_url"
  log_info "$pr_title"
  printf '%bReview the version bump and changelog, then confirm.%b\n' "$C_BOLD" "$C_RESET"
  printf 'Merge release PR #%s now? [y/N] ' "$pr_number"
  read -r ans
  case "$ans" in
    y|Y|yes|YES) ;;
    *) log_info "left #$pr_number open; merge it yourself, then re-run 'prepare' to tag and publish"; return 0 ;;
  esac

  log_step "merging #$pr_number"
  # Squash, to match the workflow and keep main's linear-history rule.
  gh pr merge "$pr_number" -R "$owner/$repo" --squash --delete-branch

  log_step "tagging and publishing the GitHub release"
  # Reads the merged manifest, so the tag is whatever was actually merged
  # rather than whatever this shell computed a minute ago.
  bunx release-please github-release \
    --repo-url="$repo_url" \
    --token="$token" \
    --config-file=release-please-config.json \
    --manifest-file=.release-please-manifest.json \
    --target-branch=main

  git fetch origin main --tags
  git reset --hard origin/main
  tag=$(git describe --tags --abbrev=0)
  log_ok "released $tag"
  log_info "install it with: go install github.com/$owner/$repo/cmd/struktly@$tag"
}

cmd_all() {
  cmd_check
  cmd_prepare
}

usage() {
  cat <<'USAGE'
usage: scripts/release-local.sh <command>

  check     run every gate CI runs, and change nothing
  prepare   open/update the release PR, merge it, tag and publish
  all       check, then prepare

Runs the same stages as .github/workflows/, with your own gh auth.
USAGE
}

case "${1:-}" in
  check)   shift; cmd_check "$@" ;;
  prepare) shift; cmd_prepare "$@" ;;
  all)     shift; cmd_all "$@" ;;
  ""|-h|--help|help) usage ;;
  *) usage; exit 1 ;;
esac
