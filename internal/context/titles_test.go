package context

import (
	stdcontext "context"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocumentTitleReadsTheFirstHeading(t *testing.T) {
	root := t.TempDir()
	for name, test := range map[string]struct {
		content string
		want    string
	}{
		"plain heading":     {"# Architecture\n\nBody.\n", "Architecture"},
		"after frontmatter": {"---\ntype: doc\ntitle: \"Metadata\"\n---\n\n# Real Title\n", "Real Title"},
		"after prose":       {"Some preamble.\n\n# Later Heading\n", "Later Heading"},
		"ignores h2":        {"## Not This\n\n# This One\n", "This One"},
		"no heading":        {"Just prose.\n", ""},
		"empty heading":     {"#\n\n# Actual\n", "Actual"},
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(root, "doc.md")
			writeFile(t, root, "doc.md", test.content)
			title, ok := documentTitle(path)
			if test.want == "" {
				if ok {
					t.Fatalf("expected no title, got %q", title)
				}
				return
			}
			if !ok || title != test.want {
				t.Fatalf("title = %q (%v), want %q", title, ok, test.want)
			}
		})
	}
}

// The gap the corpus found: a decision record whose filename is a serial number
// and whose title says exactly what it is about.
func TestTitleMatchReachesADocumentItsFilenameHides(t *testing.T) {
	root := initSelectionRepo(t)
	writeFile(t, root, "docs/adr/0001-record.md",
		"# ADR 0001: Record architecture decisions\n\nStatus: accepted\n")
	writeFile(t, root, "docs/adr/0002-unrelated.md",
		"# ADR 0002: Choose a colour scheme\n\nStatus: accepted\n")

	const task = "document the architecture decisions"
	selection, err := selectPacketContext(stdcontext.Background(), selectionRequest{
		root: root, task: task, limits: DefaultPacketLimits(),
	})
	if err != nil {
		t.Fatalf("selectPacketContext returned error: %v", err)
	}

	var record *PacketItem
	for i := range selection.items {
		switch selection.items[i].Path {
		case "docs/adr/0001-record.md":
			record = &selection.items[i]
		case "docs/adr/0002-unrelated.md":
			t.Error("a decision record about something else was selected")
		}
	}
	if record == nil {
		t.Fatalf("the decision record was not selected: %v", selectedPaths(selection.items))
	}
	if record.Reason != "title_match" {
		t.Fatalf("reason = %q, want title_match", record.Reason)
	}
	if !strings.Contains(record.Provenance.Location, "Record architecture decisions") {
		t.Fatalf("provenance does not quote the title: %q", record.Provenance.Location)
	}

	explanation, err := ExplainSelection(stdcontext.Background(), root, "docs/adr/0001-record.md", task, "")
	if err != nil {
		t.Fatalf("ExplainSelection returned error: %v", err)
	}
	if explanation.Reason != "title_match" || !strings.Contains(explanation.Detail, "architecture decisions") {
		t.Fatalf("explain = %s / %q", explanation.Reason, explanation.Detail)
	}
}

// One request word in a title is a word the document happens to contain. The
// threshold is two, and unlike identifiers it is a count rather than a ratio:
// a prose title is long enough that "half its tokens" would reject titles that
// plainly are about the request.
func TestTitleMatchNeedsMoreThanOneWord(t *testing.T) {
	root := t.TempDir()
	words := map[string]struct{}{"architecture": {}, "decisions": {}}

	writeFile(t, root, "one.md", "# Architecture\n")
	if match, _ := fileTitleMatch(root, "one.md", words); match.score != 0 {
		t.Errorf("a single matched word counted as evidence: %#v", match)
	}
	writeFile(t, root, "two.md", "# ADR 0001: Record architecture decisions\n")
	if match, _ := fileTitleMatch(root, "two.md", words); match.score != 2 {
		t.Errorf("a title naming both words was not matched: %#v", match)
	}
}

// Title matching must only add, like symbol matching before it.
func TestTitleMatchingOnlyAddsCandidates(t *testing.T) {
	root := initSelectionRepo(t)
	writeFile(t, root, "docs/untitled.md", "No heading in this document at all.\n")

	selection, err := selectPacketContext(stdcontext.Background(), selectionRequest{
		root: root, task: "heading document at all", limits: DefaultPacketLimits(),
	})
	if err != nil {
		t.Fatalf("selectPacketContext returned error: %v", err)
	}
	for _, item := range selection.items {
		if item.Path == "docs/untitled.md" && item.Reason == "title_match" {
			t.Fatal("a document with no title produced a title match")
		}
	}
}
