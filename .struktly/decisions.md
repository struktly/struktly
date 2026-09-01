# Decisions

## The supported Go floor is the `go` directive in `go.mod`

Decided 2026-09-01.

This module is installed with `go install`, so its `go` directive is a
compatibility surface for toolchains nobody here controls. Raising it to match
the newest release, or to match a sibling component that has different
consumers, would drop installers and buy this repository nothing.

So `go.mod` is authoritative, and every other statement of the floor — the mise
pin, the README badge, the README prose, and `CONTRIBUTING.md` — follows it.
`toolchain_test.go` fails when they disagree, because five statements of one
number is four opportunities to be wrong and the badge is the one a prospective
installer reads first.

Raising the floor is a deliberate change with its own reason and its own
release note. It is not a side effect of developing on a newer toolchain.
