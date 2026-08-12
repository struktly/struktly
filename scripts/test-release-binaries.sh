#!/bin/sh
set -eu

root=$(unset CDPATH; cd -- "$(dirname -- "$0")/.." && pwd)
tmp=$(mktemp -d)
temporary_tag=false
cleanup() {
	if [ "$temporary_tag" = true ]; then git -C "$root" tag -d "$tag" >/dev/null; fi
	rm -rf "$tmp"
}
trap cleanup EXIT INT TERM

tag=$(git -C "$root" tag --points-at HEAD | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+$' | head -1 || true)
if [ -z "$tag" ]; then
	tag=v0.0.0
	git -C "$root" rev-parse --verify "refs/tags/$tag" >/dev/null 2>&1 && {
		printf 'test-release-binaries: temporary tag already exists: %s\n' "$tag" >&2
		exit 1
	}
	git -C "$root" tag "$tag" HEAD
	temporary_tag=true
fi
revision=$(git -C "$root" rev-parse HEAD)

for target in aarch64-apple-darwin x86_64-unknown-linux-gnu aarch64-unknown-linux-gnu; do
	out="$tmp/$target"
	"$root/scripts/package-release-binary.sh" "$tag" "$target" "$out"
	binary="$out/struktly-$target"
	manifest="$binary.sha256"
	expected=$(awk 'NR == 1 { print $1 }' "$manifest")
	actual=$(shasum -a 256 "$binary" | awk '{print $1}')
	[ "$expected" = "$actual" ]
	grep -Fx "version=$tag" "$manifest" >/dev/null
	grep -Fx "revision=$revision" "$manifest" >/dev/null
	grep -Fx "target=$target" "$manifest" >/dev/null
done

repeat="$tmp/repeat"
"$root/scripts/package-release-binary.sh" "$tag" aarch64-apple-darwin "$repeat"
cmp "$tmp/aarch64-apple-darwin/struktly-aarch64-apple-darwin" \
	"$repeat/struktly-aarch64-apple-darwin" >/dev/null

host_target=''
case "$(uname -s)-$(uname -m)" in
	Darwin-arm64) host_target=aarch64-apple-darwin ;;
	Linux-x86_64) host_target=x86_64-unknown-linux-gnu ;;
	Linux-aarch64) host_target=aarch64-unknown-linux-gnu ;;
esac
if [ -n "$host_target" ]; then
	"$tmp/$host_target/struktly-$host_target" version --json |
		grep -F "\"version\": \"$tag\"" >/dev/null
	"$tmp/$host_target/struktly-$host_target" version --json |
		grep -F "\"revision\": \"$revision\"" >/dev/null
fi

if "$root/scripts/package-release-binary.sh" "$tag" unsupported "$tmp/unsupported" >/dev/null 2>&1; then
	printf 'test-release-binaries: unsupported target was accepted\n' >&2
	exit 1
fi

printf 'test-release-binaries: passed\n'
