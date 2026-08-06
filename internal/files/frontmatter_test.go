package files

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestOKFFrontmatter(t *testing.T) {
	got := OKFFrontmatter("context-packet", "Context Packet: add auth", "Task-scoped context.", time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC))
	want := "---\n" +
		"type: context-packet\n" +
		"schema: struktly/packet/v2\n" +
		"title: \"Context Packet: add auth\"\n" +
		"description: \"Task-scoped context.\"\n" +
		"timestamp: 2026-07-10T10:00:00Z\n" +
		"---\n\n"
	if got != want {
		t.Fatalf("unexpected frontmatter:\n%q\nwant:\n%q", got, want)
	}
}

func TestStripFrontmatter(t *testing.T) {
	in := "---\ntype: constraints\ntimestamp: 2026-07-10T10:00:00Z\n---\n\n# Constraints\n\n- Keep it small.\n"
	if got := StripFrontmatter(in); got != "# Constraints\n\n- Keep it small.\n" {
		t.Fatalf("unexpected stripped content: %q", got)
	}
	for _, passthrough := range []string{
		"# No frontmatter\n",
		"--- not frontmatter\n",
		"---\nunterminated frontmatter\n",
	} {
		if got := StripFrontmatter(passthrough); got != passthrough {
			t.Fatalf("expected passthrough for %q, got %q", passthrough, got)
		}
	}
}

// C4: a leading slash anchors a .gitignore pattern to the repository root. The
// slash was kept in the stored pattern, which can never match a
// repository-relative path, so `/generated` was silently dead while the scan
// reported that root-level patterns were applied.
func TestIgnoreMatcherAppliesRootAnchoredPatterns(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("/generated\n/build.log\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := NewIgnoreMatcher(root)

	for _, skipped := range []struct {
		rel   string
		isDir bool
	}{
		{rel: "generated", isDir: true},
		{rel: "generated/out.txt"},
		{rel: "build.log"},
	} {
		if !m.ShouldSkip(skipped.rel, skipped.isDir) {
			t.Errorf("root-anchored pattern did not skip %q", skipped.rel)
		}
	}
	// Anchored means anchored: the same name deeper in the tree is not covered.
	if m.ShouldSkip("src/generated/out.txt", false) {
		t.Error("root-anchored pattern matched a nested path")
	}
}
