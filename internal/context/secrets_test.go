package context

import (
	stdcontext "context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// leakMarker stands in for secret material everywhere in this file. Tests pair
// it with a pattern prefix so the scanner fires, and assert on the marker so a
// failure never prints anything credential-shaped.
const leakMarker = "private-material-must-not-leak"

// guidanceWithLateSecret returns a guidance document larger than the selection
// path's per-file read window, with the secret past that window.
func guidanceWithLateSecret(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("# Direction\n\n")
	for b.Len() < maxPacketFileBytes+8*1024 {
		b.WriteString("padding line that is ordinary repository guidance\n")
	}
	b.WriteString("\n-----BEGIN PRIVATE KEY-----\n" + leakMarker + "\n")
	return b.String()
}

// B1: the guidance files are read whole and copied into the packet's Direction,
// Constraints and Decisions fields, while the secret scan on the selection path
// only ever sees the first maxPacketFileBytes of a file. A secret past that
// offset shipped in full — and the packet's own truncation record claimed only
// the scanned prefix had been included.
func TestGuidanceFieldsNeverCarryUnscannedBytes(t *testing.T) {
	root := initSelectionRepo(t)
	writeFile(t, root, ".struktly/direction.md", guidanceWithLateSecret(t))

	result, err := Brief(BriefOptions{Root: root, Task: "review direction", NoWrite: true})
	if err != nil {
		t.Fatalf("Brief returned error: %v", err)
	}
	if strings.Contains(result.Packet.Direction, leakMarker) {
		t.Fatal("packet.direction carried bytes the secret scanner never saw")
	}

	// The packet must not claim less than it carries. Whatever survives in the
	// legacy field has to be no larger than the bytes actually included.
	for _, item := range result.Packet.Items {
		if item.Path != ".struktly/direction.md" {
			continue
		}
		if len(result.Packet.Direction) > item.IncludedBytes {
			t.Fatalf("packet.direction is %d bytes but the item records %d of %d included",
				len(result.Packet.Direction), item.IncludedBytes, item.OriginalBytes)
		}
	}
}

// B1, second half: a secret inside the scanned window must exclude the file and
// clear the legacy field, which is the behaviour sanitizeLegacyFields already
// guaranteed for constraints.md. Direction and decisions get the same guarantee.
func TestGuidanceFieldsClearedWhenExcluded(t *testing.T) {
	for _, guidance := range []struct {
		rel   string
		field func(Packet) string
	}{
		{rel: ".struktly/direction.md", field: func(p Packet) string { return p.Direction }},
		{rel: ".struktly/decisions.md", field: func(p Packet) string { return p.Decisions }},
	} {
		t.Run(guidance.rel, func(t *testing.T) {
			root := initSelectionRepo(t)
			writeFile(t, root, guidance.rel, "-----BEGIN PRIVATE KEY-----\n"+leakMarker+"\n")

			result, err := Brief(BriefOptions{Root: root, Task: "review guidance", NoWrite: true})
			if err != nil {
				t.Fatalf("Brief returned error: %v", err)
			}
			if strings.Contains(guidance.field(result.Packet), leakMarker) {
				t.Fatalf("%s leaked into its legacy packet field", guidance.rel)
			}
			assertDecision(t, result.Packet.Exclusions, guidance.rel, "secret_detected")
		})
	}
}

// B2: suggest-instructions excerpts the same guidance files into draft
// instruction files on disk, on a path that never consulted the secret scanner.
func TestSuggestInstructionsDoesNotExcerptSecrets(t *testing.T) {
	root := initSelectionRepo(t)
	writeFile(t, root, ".struktly/project-context.md", "# Repository context\n")
	writeFile(t, root, ".struktly/constraints.md", "-----BEGIN PRIVATE KEY-----\n"+leakMarker+"\n")
	writeFile(t, root, ".struktly/direction.md",
		"# Direction\n\n## Non-goals\n\n- -----BEGIN PRIVATE KEY----- "+leakMarker+"\n")

	result, err := SuggestInstructions(SuggestInstructionsOptions{Root: root})
	if err != nil {
		t.Fatalf("SuggestInstructions returned error: %v", err)
	}
	for _, path := range result.OutputPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), leakMarker) {
			t.Fatalf("%s excerpted secret-bearing guidance", filepath.Base(path))
		}
	}
}

// B3: filepath.Match's `*` does not cross `/`, so `*secret*` could not match
// `secrets/db.txt`. The non-Git walk skipped such directories and the Git-backed
// selection did not, so the two paths disagreed about the same tree.
func TestSensitiveDirectoryConventionsAreExcluded(t *testing.T) {
	for _, rel := range []string{
		"secrets/db.txt",
		"config/credentials/aws.txt",
		"deploy/tokens/ci.txt",
	} {
		t.Run(rel, func(t *testing.T) {
			root := initSelectionRepo(t)
			writeFile(t, root, rel, "value\n")

			explanation, err := ExplainSelection(stdcontext.Background(), root, rel, "inspect secrets credentials tokens db aws ci", "")
			if err != nil {
				t.Fatalf("ExplainSelection returned error: %v", err)
			}
			if explanation.Decision != "excluded" || explanation.Reason != "sensitive_path" {
				t.Fatalf("%s: got %s (%s), want excluded (sensitive_path)", rel, explanation.Decision, explanation.Reason)
			}

			selection, err := selectPacketContextWithLimits(stdcontext.Background(), root,
				"inspect secrets credentials tokens db aws ci", nil, DefaultPacketLimits())
			if err != nil {
				t.Fatalf("selectPacketContext returned error: %v", err)
			}
			assertItemAbsent(t, selection.items, rel)
		})
	}
}

// B5: the non-Git walk reported symlinked files, and inspectFile then read
// through them with os.ReadFile — while the scan's own output claims symlinks
// are excluded. A symlink named direction.md excerpted whatever it pointed at.
func TestNonGitScanDoesNotFollowSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "private-notes.md")
	if err := os.WriteFile(outside, []byte("# Outside\n\n"+leakMarker+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".struktly"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".struktly", "direction.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	writeFile(t, root, "README.md", "# Repository\n")

	result, err := Scan(ScanOptions{Root: root, NoWrite: true})
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	if result.Snapshot.Direction != nil && strings.Contains(result.Snapshot.Direction.Excerpt, leakMarker) {
		t.Fatal("scan followed a symlink out of the repository and excerpted it")
	}
}

// B6: a file cut short because the packet's total budget ran out was recorded
// as `content_limit`, which humanReason renders as "per-file size limit
// reached". The per-file limit had nothing to do with it. The audit trail is
// the product's claim about itself, so a wrong reason there is worse than none.
func TestTotalBudgetTruncationIsNotLabelledContentLimit(t *testing.T) {
	root := initSelectionRepo(t)
	writeFile(t, root, "AGENTS.md", "# Agents\n"+strings.Repeat("a", 5000))
	writeFile(t, root, "CLAUDE.md", "# Claude\n"+strings.Repeat("b", 5000))

	limits := DefaultPacketLimits()
	limits.MaxTotalBytes = 6000
	selection, err := selectPacketContextWithLimits(stdcontext.Background(), root, "instructions", nil, limits)
	if err != nil {
		t.Fatalf("selectPacketContext returned error: %v", err)
	}
	if len(selection.truncations) == 0 {
		t.Fatal("expected a truncation from the total budget")
	}
	for _, truncation := range selection.truncations {
		if truncation.Reason == "content_limit" {
			t.Fatalf("%s was cut by the total budget (%d bytes) but is recorded as %q, which means the per-file limit of %d bytes",
				truncation.Path, limits.MaxTotalBytes, truncation.Reason, limits.MaxFileBytes)
		}
		if truncation.Reason != "total_limit" {
			t.Fatalf("unexpected truncation reason %q", truncation.Reason)
		}
	}
}

// B6, second half: candidates counted against the item limit included files
// that could never have been included anyway, because the limit was applied
// before the file was inspected. "40 more matched but did not fit" is a
// different claim from "40 more matched, and some were secrets".
func TestItemLimitOmissionsExcludeUnincludableFiles(t *testing.T) {
	root := initSelectionRepo(t)
	writeFile(t, root, "instructions-one.md", "# One\n")
	writeFile(t, root, "instructions-two.md", "# Two\n")
	writeFile(t, root, "instructions-three.md", "-----BEGIN PRIVATE KEY-----\n"+leakMarker+"\n")

	limits := DefaultPacketLimits()
	limits.MaxItems = 1
	selection, err := selectPacketContextWithLimits(stdcontext.Background(), root, "instructions one two three", nil, limits)
	if err != nil {
		t.Fatalf("selectPacketContext returned error: %v", err)
	}
	for _, exclusion := range selection.exclusions {
		if exclusion.Reason != "item_limit" {
			continue
		}
		if strings.Contains(exclusion.Detail, "instructions-three.md") {
			t.Fatalf("item_limit omission names a file excluded for another reason: %q", exclusion.Detail)
		}
	}
	assertDecision(t, selection.exclusions, "instructions-three.md", "secret_detected")
}

// B7: these token shapes were not recognised at all.
func TestSecretPatternsCoverCommonProviderTokens(t *testing.T) {
	for name, sample := range map[string]string{
		"github fine-grained": "github_pat_" + strings.Repeat("A", 22) + "_" + strings.Repeat("b", 59),
		"github classic":      "ghp_" + strings.Repeat("A", 36),
		"slack bot":           "xoxb-123456789012-1234567890123-" + strings.Repeat("a", 24),
		"stripe live":         "sk_live_" + strings.Repeat("a", 24),
		"google api":          "AIza" + strings.Repeat("a", 35),
		"aws access key":      "AKIA" + strings.Repeat("A", 16),
	} {
		if !containsSecret("token = " + sample + "\n") {
			t.Errorf("%s token shape is not detected", name)
		}
	}
	// Ordinary prose must stay includable.
	for _, benign := range []string{
		"See docs/tokens.md for the tokenizer design.\n",
		"const maxTokens = 4096\n",
	} {
		if containsSecret(benign) {
			t.Errorf("benign content classified as a secret: %q", benign)
		}
	}
}

// Truncation used byte indexes, which land inside a multi-byte rune for any
// non-ASCII text and emit invalid UTF-8 into the packet.
func TestExcerptsTruncateOnRuneBoundaries(t *testing.T) {
	for name, got := range map[string]string{
		"excerptMarkdown": excerptMarkdown(strings.Repeat("é", 50), 11),
		"sectionExcerpt":  sectionExcerptFor(strings.Repeat("é", 50), 11),
	} {
		if !utf8.ValidString(got) {
			t.Errorf("%s produced invalid UTF-8: %q", name, got)
		}
	}
}

func sectionExcerptFor(body string, maxChars int) string {
	var b strings.Builder
	writeSectionExcerpt(&b, "## Repository\n\n"+body+"\n", []string{"## Repository"}, maxChars)
	return b.String()
}
