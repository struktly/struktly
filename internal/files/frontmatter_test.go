package files

import (
	"testing"
	"time"
)

func TestOKFFrontmatter(t *testing.T) {
	got := OKFFrontmatter("context-packet", "Context Packet: add auth", "Task-scoped context.", time.Date(2026, 7, 10, 10, 0, 0, 0, time.UTC))
	want := "---\n" +
		"type: context-packet\n" +
		"schema: struktly/packet/v1\n" +
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
