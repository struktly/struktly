package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/struktly/struktly/internal/files"
)

func TestBriefWritesContextPacket(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "README.md", "# Demo Repo\n")
	writeFile(t, root, "Makefile", "test:\n\tgo test ./...\n")
	writeFile(t, root, ".struktly/direction.md", "# Current Direction\n\nLocal-first context and evidence layer.\n")
	writeFile(t, root, ".struktly/constraints.md", "# Constraints\n\n- No model calls in CLI commands.\n\n## Non-goals\n\n- No hosted control plane.\n")
	writeFile(t, root, ".struktly/decisions.md", "# Decisions\n\n- Keep it local-first.\n")
	writeFile(t, root, ".struktly/evidence.md", "# Evidence\n\n- User narrowed direction.\n")
	if _, err := Scan(ScanOptions{Root: root}); err != nil {
		t.Fatalf("scan fixture: %v", err)
	}
	initGitRepo(t, root)

	result, err := Brief(BriefOptions{
		Root: root,
		Task: "Refocus Struktly around the local-first context and evidence MVP",
		Now:  time.Date(2026, 7, 4, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Brief returned error: %v", err)
	}

	wantDir := filepath.Join(root, ".struktly", "context-packets")
	wantDir, err = filepath.EvalSymlinks(filepath.Dir(wantDir))
	if err != nil {
		t.Fatalf("resolve packet parent: %v", err)
	}
	wantDir = filepath.Join(wantDir, "context-packets")
	if filepath.Dir(result.OutputPath) != wantDir {
		t.Fatalf("unexpected context packet dir: %s", result.OutputPath)
	}
	if !strings.HasSuffix(result.OutputPath, "20260704-120000-refocus-struktly-around-the-local-first-context-and-evidence-mvp.md") {
		t.Fatalf("unexpected context packet name: %s", result.OutputPath)
	}

	data, err := os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatalf("read context packet: %v", err)
	}
	content := string(data)

	if !strings.HasPrefix(content, "---\ntype: context-packet\n") {
		t.Fatalf("packet should start with OKF frontmatter:\n%s", content)
	}
	assertContains(t, content, "timestamp: 2026-07-04T12:00:00Z")
	assertContains(t, content, "# Struktly Context Packet")
	assertContains(t, content, "## Task")
	assertContains(t, content, "Refocus Struktly around the local-first context and evidence MVP")
	assertContains(t, content, "## Repository")
	assertContains(t, content, "## Product Direction")
	assertContains(t, content, "Local-first context and evidence layer.")
	assertContains(t, content, "## Constraints")
	assertContains(t, content, "No model calls in CLI commands.")
	assertContains(t, content, "No hosted control plane.")
	assertContains(t, content, "## Verification Commands")
	assertContains(t, content, "make test")
	assertContains(t, content, "## Suggested Files To Inspect")
	assertContains(t, content, "## Source References")
	assertContains(t, content, "README.md")
	assertContains(t, content, ".struktly/decisions.md")
	assertContains(t, content, ".struktly/evidence.md")

	for _, retired := range []string{
		"## Repo Summary",
		"## Relevant Commands",
		"## Suggested Tests / Checks To Run",
		"## Known Risks",
	} {
		if strings.Contains(content, retired) {
			t.Fatalf("packet should not contain retired section %q:\n%s", retired, content)
		}
	}

	suggested := sectionContent(content, "## Suggested Files To Inspect")
	if strings.Contains(suggested, ".struktly/project-context.md") {
		t.Fatalf("suggested files should not self-reference project context:\n%s", suggested)
	}

	// Direction and constraints exist, so setup only mentions approved memory.
	setup := sectionContent(content, "## Struktly Setup")
	assertContains(t, setup, "approved memory")
	if strings.Contains(setup, "direction.md") || strings.Contains(setup, "constraints.md") {
		t.Fatalf("setup section should only list missing items:\n%s", setup)
	}
}

func TestBriefBuildsLiveContextWithoutPriorScan(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "README.md", "# Live context\n")
	writeFile(t, root, "Makefile", "test:\n\tgo test ./...\n")
	initGitRepo(t, root)

	result, err := Brief(BriefOptions{Root: root, Task: "test live context"})
	if err != nil {
		t.Fatalf("Brief returned error: %v", err)
	}
	if len(result.Packet.Items) == 0 || !containsString(result.Packet.VerificationCommands, "make test") {
		t.Fatalf("brief did not collect live context: %+v", result.Packet)
	}
}

func TestBriefColdRepoOmitsStruktlyInternalContent(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "README.md", "# Cold Repo\n")
	writeFile(t, root, "Makefile", "test:\n\tgo test ./...\n")
	if _, err := Scan(ScanOptions{Root: root}); err != nil {
		t.Fatalf("scan fixture: %v", err)
	}
	initGitRepo(t, root)
	result, err := Brief(BriefOptions{
		Root: root,
		Task: "add rate limiting to the orders endpoint",
		Now:  time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Brief returned error: %v", err)
	}

	data, err := os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatalf("read context packet: %v", err)
	}
	content := string(data)

	for _, leaked := range []string{
		"architecture-to-code",
		"struktly remember",
		"product/cmd/struktly",
		"website/src",
		"mvp-technical-plan",
		"docs/product/current-direction.md",
		"Graph/UI",
	} {
		if strings.Contains(content, leaked) {
			t.Fatalf("cold-repo brief leaked Struktly-internal content %q:\n\n%s", leaked, content)
		}
	}

	assertContains(t, content, "## Struktly Setup")
	assertContains(t, content, ".struktly/direction.md")
	assertContains(t, content, ".struktly/constraints.md")
	assertContains(t, content, ".struktly/memory/approved/")
	assertContains(t, content, "Run `struktly init` to scaffold these files.")
	if strings.Contains(content, "file was found") {
		t.Fatalf("cold-repo packet should not contain missing-file apologies:\n%s", content)
	}
}

func TestBriefSurfacesTaskMatchedFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "README.md", "# Router\n")
	writeFile(t, root, "Makefile", "test:\n\tgo test ./...\n")
	writeFile(t, root, "handler.go", "package router\n")
	writeFile(t, root, "middleware/timeout.go", "package middleware\n")
	writeFile(t, root, "middleware/logger.go", "package middleware\n")
	writeFile(t, root, "legacy/timeout.go", "package old\n")
	if _, err := Scan(ScanOptions{Root: root}); err != nil {
		t.Fatalf("scan fixture: %v", err)
	}
	initGitRepo(t, root)
	result, err := Brief(BriefOptions{
		Root: root,
		Task: "add request timeout middleware",
		Now:  time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Brief returned error: %v", err)
	}

	data, err := os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatalf("read context packet: %v", err)
	}
	suggested := sectionContent(string(data), "## Suggested Files To Inspect")

	assertContains(t, suggested, "middleware/timeout.go")
	assertContains(t, suggested, "middleware/")
	if strings.Contains(suggested, "logger.go") {
		t.Fatalf("unmatched file should not be suggested:\n%s", suggested)
	}
	if strings.Contains(suggested, "legacy/timeout.go") {
		t.Fatalf("stale-directory file should not be suggested:\n%s", suggested)
	}
}

func TestBriefStripsFrontmatterFromInputs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "README.md", "# Repo\n")
	writeFile(t, root, ".struktly/direction.md", "---\ntype: direction\ntitle: \"Product Direction\"\n---\n\n# Direction\n\nShip the local-first wedge.\n")
	writeFile(t, root, ".struktly/constraints.md", "---\ntype: constraints\n---\n\n# Constraints\n\n- Keep changes reviewable.\n")
	if _, err := Scan(ScanOptions{Root: root}); err != nil {
		t.Fatalf("scan fixture: %v", err)
	}
	initGitRepo(t, root)
	result, err := Brief(BriefOptions{
		Root: root,
		Task: "tighten constraint handling",
		Now:  time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("Brief returned error: %v", err)
	}

	data, err := os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatalf("read context packet: %v", err)
	}
	body := files.StripFrontmatter(string(data))

	assertContains(t, body, "Ship the local-first wedge.")
	assertContains(t, body, "Keep changes reviewable.")
	legacySections := sectionContent(body, "## Product Direction") + sectionContent(body, "## Constraints")
	for _, leaked := range []string{"type: direction", "type: constraints"} {
		if strings.Contains(legacySections, leaked) {
			t.Fatalf("input frontmatter leaked into rendered excerpts %q:\n%s", leaked, legacySections)
		}
	}
}

func TestRankByTaskOverlap(t *testing.T) {
	ranked := rankByTaskOverlap("improve orders idempotency handling", []string{
		"docs/payments.md",
		"docs/orders/idempotency.md",
		"docs/orders/overview.md",
	})
	if len(ranked) != 2 {
		t.Fatalf("expected 2 ranked paths, got %v", ranked)
	}
	if ranked[0] != "docs/orders/idempotency.md" {
		t.Fatalf("expected the two-word match first, got %v", ranked)
	}
}
