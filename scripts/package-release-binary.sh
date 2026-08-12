#!/bin/sh
set -eu

root=$(unset CDPATH; cd -- "$(dirname -- "$0")/.." && pwd)
tag=${1:?usage: package-release-binary.sh TAG TARGET OUTPUT_DIR}
target=${2:?usage: package-release-binary.sh TAG TARGET OUTPUT_DIR}
output_dir=${3:?usage: package-release-binary.sh TAG TARGET OUTPUT_DIR}

fail() {
	printf 'package-release-binary: %s\n' "$1" >&2
	exit 1
}

hash_file() {
	if command -v sha256sum >/dev/null 2>&1; then
		sha256sum "$1" | awk '{print $1}'
	else
		shasum -a 256 "$1" | awk '{print $1}'
	fi
}

printf '%s\n' "$tag" | grep -Eq '^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' ||
	fail "tag must be a stable semantic version"

case "$target" in
	aarch64-apple-darwin) goos=darwin goarch=arm64 ;;
	x86_64-unknown-linux-gnu) goos=linux goarch=amd64 ;;
	aarch64-unknown-linux-gnu) goos=linux goarch=arm64 ;;
	*) fail "unsupported target: $target" ;;
esac

revision=$(git -C "$root" rev-list -n 1 "$tag" 2>/dev/null) || fail "tag does not exist: $tag"
[ "$(git -C "$root" rev-parse HEAD)" = "$revision" ] || fail "HEAD does not match $tag"
date=$(git -C "$root" show -s --format=%cI "$revision")
[ -n "$date" ] || fail "could not resolve release date"

binary="struktly-$target"
manifest="$binary.sha256"
mkdir -p "$output_dir"
[ ! -e "$output_dir/$binary" ] || fail "asset already exists: $output_dir/$binary"
[ ! -e "$output_dir/$manifest" ] || fail "asset already exists: $output_dir/$manifest"

pkg=github.com/struktly/struktly/internal/buildinfo
ldflags="-s -w -buildid= -X $pkg.Version=$tag -X $pkg.Revision=$revision -X $pkg.Date=$date"
(
	cd "$root"
	CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
		go build -trimpath -buildvcs=false -ldflags "$ldflags" \
		-o "$output_dir/$binary" ./cmd/struktly
)
chmod 0755 "$output_dir/$binary"
{
	printf '%s  %s\n' "$(hash_file "$output_dir/$binary")" "$binary"
	printf 'version=%s\n' "$tag"
	printf 'revision=%s\n' "$revision"
	printf 'target=%s\n' "$target"
} > "$output_dir/$manifest"
