# Decisions

## The capabilities document is derived from the binary, not declared

Decided 2026-09-01.

`capabilities --json` is what a consumer negotiates against, and `--require`
answers with an exit code read from it. A hand-written list there is a claim
compared against a claim: three invented commands could be advertised, satisfy
a consumer, and pass the suite.

So each list is held to the thing it describes. Advertised commands must
resolve in the command tree, and the tree must be advertised or named as
outside the contract. Advertised schemas must be published files or declared
Markdown-only, and published files must be advertised or declared input-only.
Features are held the same way once each has a registered proof
(`tasks/bind-features-to-proof.md`); until then that list is the one exception,
and it is named as one wherever the rule is stated.

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
