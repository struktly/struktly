package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSuggestInstructionsWritesSuggestedFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "Makefile", "test:\n\tgo test ./...\n")
	writeFile(t, root, ".struktly/direction.md", `# Current Direction

## Non-goals

- A new coding agent or chat interface
- A SaaS control plane

## Out of Scope

- Graph-first product story
`)
	writeFile(t, root, ".struktly/constraints.md", "---\ntype: constraints\ntitle: \"Constraints\"\n---\n\n# Constraints\n\n- Keep changes reviewable and evidence-backed.\n")
	writeFile(t, root, ".struktly/decisions.md", "# Decisions\n\n## 2026-07-04 — Keep local-first\n\n**Decision:** Keep .struktly/ as the product surface.\n\n**Status:** accepted\n\n---\n\n## 2026-07-04 — Open idea\n\n**Decision:** Maybe add hosted sync later.\n\n**Status:** proposed\n")
	writeFile(t, root, "AGENTS.md", "# Active instructions\n\nDo not overwrite.\n")
	writeFile(t, root, "CLAUDE.md", "# Active Claude instructions\n\nDo not overwrite.\n")

	if _, err := Scan(ScanOptions{Root: root}); err != nil {
		t.Fatalf("scan fixture: %v", err)
	}

	result, err := SuggestInstructions(SuggestInstructionsOptions{
		Root: root,
		Now:  time.Date(2026, 7, 5, 14, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("SuggestInstructions returned error: %v", err)
	}
	if len(result.OutputPaths) != 3 {
		t.Fatalf("expected 3 output paths, got %d: %v", len(result.OutputPaths), result.OutputPaths)
	}

	for _, name := range []string{"AGENTS.suggested.md", "CLAUDE.suggested.md", "CURSOR.suggested.md"} {
		path := filepath.Join(root, agentInstructionsDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		content := string(data)
		if !strings.HasPrefix(content, "---\ntype: agent-instructions\n") {
			t.Fatalf("draft %s should start with OKF frontmatter:\n%s", name, content)
		}
		assertContains(t, content, "# Struktly Agent Instructions (Suggested)")
		assertContains(t, content, "2026-07-05T14:00:00Z")
		assertContains(t, content, "## Constraints")
		assertContains(t, content, "Keep changes reviewable and evidence-backed.")
		if strings.Contains(content, "type: constraints") {
			t.Fatalf("constraints frontmatter leaked into %s:\n%s", name, content)
		}
		assertContains(t, content, "## Product Non-goals")
		assertContains(t, content, "do not build or assume any of these")
		assertContains(t, content, "A new coding agent or chat interface")
		assertContains(t, content, "Graph-first product story")
		assertContains(t, content, "## Active Decisions")
		assertContains(t, content, "Keep local-first")
		assertContains(t, content, "Keep .struktly/ as the product surface.")
		if strings.Contains(content, "Maybe add hosted sync later") {
			t.Fatalf("proposed decision should not appear in %s", name)
		}
		assertContains(t, content, "## Verification Commands")
		assertContains(t, content, "make test")
		assertContains(t, content, "never overwrites active agent instruction files")
	}

	agents, err := os.ReadFile(filepath.Join(root, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read active AGENTS.md: %v", err)
	}
	if !strings.Contains(string(agents), "Do not overwrite.") {
		t.Fatalf("active AGENTS.md was modified")
	}
	claude, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("read active CLAUDE.md: %v", err)
	}
	if !strings.Contains(string(claude), "Do not overwrite.") {
		t.Fatalf("active CLAUDE.md was modified")
	}

	claudeSuggested, err := os.ReadFile(filepath.Join(root, agentInstructionsDir, "CLAUDE.suggested.md"))
	if err != nil {
		t.Fatalf("read CLAUDE.suggested.md: %v", err)
	}
	assertContains(t, string(claudeSuggested), "repo-root `CLAUDE.md`")

	cursorSuggested, err := os.ReadFile(filepath.Join(root, agentInstructionsDir, "CURSOR.suggested.md"))
	if err != nil {
		t.Fatalf("read CURSOR.suggested.md: %v", err)
	}
	assertContains(t, string(cursorSuggested), "`.cursor/rules/`")
}

func TestSuggestInstructionsRequiresScan(t *testing.T) {
	root := t.TempDir()
	if _, err := SuggestInstructions(SuggestInstructionsOptions{Root: root}); err == nil {
		t.Fatalf("expected scan prerequisite error")
	}
}

func TestSuggestInstructionsColdRepoOmitsStruktlyInternalContent(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "README.md", "# Cold Repo\n")
	writeFile(t, root, "Makefile", "test:\n\tgo test ./...\n")

	if _, err := Scan(ScanOptions{Root: root}); err != nil {
		t.Fatalf("scan fixture: %v", err)
	}
	result, err := SuggestInstructions(SuggestInstructionsOptions{
		Root: root,
		Now:  time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("SuggestInstructions returned error: %v", err)
	}

	for _, path := range result.OutputPaths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		content := string(data)
		for _, leaked := range []string{
			"architecture-to-code",
			"MVP slice",
			"graph-first UX",
			"docs/product/current-direction.md",
		} {
			if strings.Contains(content, leaked) {
				t.Fatalf("cold-repo draft %s leaked Struktly-internal content %q:\n\n%s", path, leaked, content)
			}
		}
		assertContains(t, content, "No `.struktly/constraints.md` file was found")
	}
}

func TestExtractActiveDecisions(t *testing.T) {
	decisions := extractActiveDecisions(`# Ledger

## 2026-07-04 — Accepted one

**Decision:** Keep it local-first.

**Status:** accepted

## 2026-07-04 — Proposed one

**Decision:** Add SaaS later.

**Status:** proposed
`)
	if len(decisions) != 1 {
		t.Fatalf("expected 1 active decision, got %d: %v", len(decisions), decisions)
	}
	if !strings.Contains(decisions[0], "Accepted one") {
		t.Fatalf("unexpected decision summary: %q", decisions[0])
	}
	if !strings.Contains(decisions[0], "Keep it local-first.") {
		t.Fatalf("expected decision body in summary: %q", decisions[0])
	}
}

func TestExtractNonGoals(t *testing.T) {
	nonGoals := extractNonGoals(`# Direction

## Non-goals

- A chat UI
- A SaaS product

## What This Is Not

- Graph-first story

## Out of Scope

- Marketplace
- No hosted sync
`)
	if len(nonGoals) != 5 {
		t.Fatalf("expected 5 non-goals, got %d: %v", len(nonGoals), nonGoals)
	}
}
