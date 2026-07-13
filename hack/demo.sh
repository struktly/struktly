#!/usr/bin/env sh
# Demo: run the full struktly continuity loop against any repo.
#
#   ./hack/demo.sh https://github.com/go-chi/chi ["task for the first brief"]
#
# Clones the repo cold, then walks the loop: scan -> brief #1 -> evidence ->
# approved memory -> brief #2. Brief #1 starts cold; brief #2 carries an
# "## Approved Memory" section. Deterministic; no model calls.
set -eu

REPO_URL="${1:?usage: $0 <git-url> [task]}"
TASK="${2:-add request timeout middleware}"
TASK2="add structured request logging"
RULE="All middleware must have a matching _test.go file."

WORKDIR="$(mktemp -d)"
trap 'rm -rf "$WORKDIR"' EXIT
export STRUKTLY_STATE_DIR="$WORKDIR/state"

# print_section <file> <"## Header">: print the section up to the next "## ".
print_section() {
	awk -v h="$2" '
		$0 == h { hit = 1 }
		hit && $0 != h && substr($0, 1, 3) == "## " { exit }
		hit { print }
	' "$1"
}

echo "==> Cloning $REPO_URL (shallow)"
git clone --depth 1 --quiet "$REPO_URL" "$WORKDIR/repo"
[ -d "$WORKDIR/repo/.struktly" ] || echo "    cold repo: no .struktly/ yet"

echo "==> Building struktly"
BIN="$WORKDIR/struktly"
go build -o "$BIN" ./cmd/struktly

echo "==> struktly scan"
"$BIN" scan --root "$WORKDIR/repo"

echo "==> struktly brief \"$TASK\"  (BRIEF #1 — cold session)"
PACKET1="$("$BIN" brief --root "$WORKDIR/repo" "$TASK" | sed -n 's/^wrote //p')"
echo "--------------------------------------------------------------"
grep '^# ' "$PACKET1" | head -1
print_section "$PACKET1" "## Task"
print_section "$PACKET1" "## Suggested Files To Inspect"
echo "--------------------------------------------------------------"
echo "    (full packet: $PACKET1 — no memory section yet)"

echo "==> Agent session (simulated): the agent does the work here, guided by BRIEF #1."

echo "==> struktly evidence  (record what was verified)"
"$BIN" evidence --root "$WORKDIR/repo" \
	--task "$TASK" \
	--agent demo \
	--outcome "Task completed; checks pass." \
	--checks "go test ./..." --result pass

echo "==> struktly memory candidate + approve  (the human gate)"
CANDIDATE_ID="$("$BIN" memory candidate --root "$WORKDIR/repo" \
	--content "$RULE" --scope repository |
	sed -n 's/^[[:space:]]*"id": *"\([^"]*\)".*/\1/p' | head -1)"
echo "    candidate: $CANDIDATE_ID"
"$BIN" memory approve --root "$WORKDIR/repo" "$CANDIDATE_ID" >/dev/null
echo "    approved: \"$RULE\""

echo "==> struktly brief \"$TASK2\"  (BRIEF #2 — next session)"
PACKET2="$("$BIN" brief --root "$WORKDIR/repo" "$TASK2" | sed -n 's/^wrote //p')"

echo
echo "==> THE MONEY SHOT: BRIEF #2 starts warm"
echo "--------------------------------------------------------------"
print_section "$PACKET2" "## Approved Memory"
echo "--------------------------------------------------------------"
echo "BRIEF #1 had no memory — any agent can cold-read a repo."
echo "BRIEF #2 carries the approved learning automatically: evidence was"
echo "recorded, a human approved one rule, and the next session gets it."
echo "Portable declarations, evidence, and approved memory remain reviewable under .struktly/."
echo "Run state and pending memory stayed in the temporary runtime-state directory."
