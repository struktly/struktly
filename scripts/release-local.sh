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
#             github-release. Produces the version bump commit, tag, and draft.
#   assets    build all three deterministic binaries for an existing draft,
#             upload them, verify the set, and publish the release.
#   all       check, then prepare, then assets. What you want for "just cut the
#             release" while hosted Actions are unavailable.
#
# Requires: gh (authenticated, with write access to struktly/struktly), git,
# jq, bun, and go.
root=$(unset CDPATH; cd -- "$(dirname -- "$0")/.." && pwd)
cd "$root"

owner=struktly
repo=struktly
repo_url="https://github.com/$owner/$repo"
# The module path kept zricethezav after the project moved to the gitleaks
# organization, so the import path and the repository URL disagree.
gitleaks_module=github.com/zricethezav/gitleaks/v8@v8.30.1
release_please=release-please@17.6.0
prepared_tag=

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
  require_tools git gh jq npx go
  require_clean_main
  log_step "syncing main"
  git fetch origin main
  git reset --hard origin/main

  token=$(gh_token)

  log_step "creating/updating the release-please PR"
  # The same config the workflow uses, so the version this computes locally is
  # the version CI would have computed. A local run that used different rules
  # would be a second release process wearing the first one's name.
  npx --yes "$release_please" release-pr \
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
    *) log_info "left #$pr_number open; merge it yourself, then re-run 'prepare' to create the tag and draft"; return 0 ;;
  esac

  log_step "merging #$pr_number"
  # Squash, to match the workflow and keep main's linear-history rule.
  merge_admin=
  case "${STRUKTLY_RELEASE_ADMIN_MERGE:-}" in 1|true|TRUE|yes|YES) merge_admin=--admin ;; esac
  gh pr merge "$pr_number" -R "$owner/$repo" --squash --delete-branch ${merge_admin:+"$merge_admin"}

  log_step "tagging and creating the draft GitHub release"
  # Reads the merged manifest, so the tag is whatever was actually merged
  # rather than whatever this shell computed a minute ago.
  npx --yes "$release_please" github-release \
    --repo-url="$repo_url" \
    --token="$token" \
    --config-file=release-please-config.json \
    --manifest-file=.release-please-manifest.json \
    --target-branch=main

  git fetch origin main --tags
  git reset --hard origin/main
  tag=$(git describe --tags --abbrev=0)
  prepared_tag=$tag
  log_ok "prepared $tag as a draft"
}

cmd_assets() {
  tag=${1:?usage: scripts/release-local.sh assets TAG}
  require_tools git gh go
  require_clean_main

  log_step "verifying the draft release"
  git fetch origin "refs/tags/$tag:refs/tags/$tag"
  revision=$(git rev-list -n 1 "$tag")
  [ -n "$revision" ] || fail "tag does not exist: $tag"
  [ "$(gh release view "$tag" -R "$owner/$repo" --json isDraft --jq .isDraft)" = true ] ||
    fail "$tag is not a draft release"
  [ "$(gh release view "$tag" -R "$owner/$repo" --json isPrerelease --jq .isPrerelease)" = false ] ||
    fail "$tag is unexpectedly a prerelease"
  [ "$(gh release view "$tag" -R "$owner/$repo" --json targetCommitish --jq .targetCommitish)" = "$revision" ] ||
    fail "$tag release target does not match its tag"

  assets_root=$(mktemp -d)
  assets_checkout="$assets_root/repository"
  assets_dist="$assets_root/dist"
  cleanup_assets() {
    git -C "$root" worktree remove "$assets_checkout" >/dev/null 2>&1 || true
    rm -rf "$assets_root"
  }
  trap cleanup_assets EXIT INT TERM
  git worktree add --detach "$assets_checkout" "$tag" >/dev/null
  mkdir "$assets_dist"

  log_step "building deterministic release binaries"
  for target in aarch64-apple-darwin x86_64-unknown-linux-gnu aarch64-unknown-linux-gnu; do
    "$assets_checkout/scripts/package-release-binary.sh" "$tag" "$target" "$assets_dist"
  done

  log_step "uploading and publishing $tag"
  gh release upload "$tag" -R "$owner/$repo" --clobber "$assets_dist"/*
  expected=$(find "$assets_dist" -maxdepth 1 -type f | wc -l | tr -d ' ')
  actual=$(gh release view "$tag" -R "$owner/$repo" --json assets --jq '.assets | length')
  [ "$expected" = 6 ] || fail "built $expected assets, expected 6"
  [ "$actual" = "$expected" ] || fail "uploaded $actual assets, expected $expected"
  gh release edit "$tag" -R "$owner/$repo" --draft=false --prerelease=false --latest
  [ "$(gh release view "$tag" -R "$owner/$repo" --json isDraft --jq .isDraft)" = false ] ||
    fail "$tag remained a draft after publication"

  cleanup_assets
  trap - EXIT INT TERM
  log_ok "released $tag with $actual verified assets"
  log_info "install it with: go install github.com/$owner/$repo/cmd/struktly@$tag"
}

cmd_all() {
  require_clean_main
  log_step "syncing main before the release gate"
  git fetch origin main
  git reset --hard origin/main
  cmd_check
  cmd_prepare
  [ -z "$prepared_tag" ] || cmd_assets "$prepared_tag"
}

usage() {
  cat <<'USAGE'
usage: scripts/release-local.sh <command>

  check     run every gate CI runs, and change nothing
  prepare   open/update the release PR, merge it, tag and leave a draft
  assets    build, upload, verify and publish an existing draft tag
  all       check, prepare, build assets and publish

Runs the same stages as .github/workflows/, with your own gh auth. Set
STRUKTLY_RELEASE_ADMIN_MERGE=1 only when hosted checks cannot start and the
complete local check has passed in this run.
USAGE
}

case "${1:-}" in
  check)   shift; cmd_check "$@" ;;
  prepare) shift; cmd_prepare "$@" ;;
  assets)  shift; cmd_assets "$@" ;;
  all)     shift; cmd_all "$@" ;;
  ""|-h|--help|help) usage ;;
  *) usage; exit 1 ;;
esac
