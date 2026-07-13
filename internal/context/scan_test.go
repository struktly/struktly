package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScanWritesProjectContext(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "README.md", "# Demo Repo\n\nA test repository.\n")
	writeFile(t, root, "go.mod", "module example.com/demo\n\ngo 1.24.0\n")
	writeFile(t, root, "Makefile", "test:\n\tgo test ./...\n\nbuild:\n\tgo build ./...\n")
	writeFile(t, root, "package.json", `{"scripts":{"test":"vitest run","build":"vite build"}}`)
	writeFile(t, root, ".struktly/direction.md", "---\ntype: direction\n---\n\n# Current Direction\n\nPreserve stable command output.\n")
	writeFile(t, root, "docs/notes/current.md", "# Current notes\n")
	writeFile(t, root, "docs/adr/0001-record-decisions.md", "# ADR 0001\n")
	writeFile(t, root, "AGENTS.md", "# Agent instructions\n")
	writeFile(t, root, ".gitignore", "node_modules/\nsecrets/\n.env.local\n")
	mkdir(t, root, "cmd")
	mkdir(t, root, "internal")
	mkdir(t, root, "node_modules")
	writeFile(t, root, "node_modules/noise/README.md", "# ignored\n")
	writeFile(t, root, "secrets/api-key.txt", "do-not-read\n")
	writeFile(t, root, ".env.local", "TOKEN=secret\n")
	writeFile(t, root, "legacy/docs/archive/old-plan.md", "# Old plan\n")
	writeFile(t, root, "pkg/testdata/fixture/Makefile", "test:\n\tgo test ./...\n")

	result, err := Scan(ScanOptions{Root: root, Now: time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}

	if result.OutputPath != filepath.Join(root, ".struktly", "project-context.md") {
		t.Fatalf("unexpected output path: %s", result.OutputPath)
	}

	data, err := os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatalf("read project context: %v", err)
	}
	content := string(data)

	if !strings.HasPrefix(content, "---\ntype: project-context\n") {
		t.Fatalf("project context should start with OKF frontmatter:\n%s", content)
	}
	assertContains(t, content, "title: \"Project Context: "+filepath.Base(root)+"\"")
	assertContains(t, content, "timestamp: 2026-07-05T12:00:00Z")
	assertContains(t, content, "# Struktly Project Context")
	assertContains(t, content, "- Repository name: "+filepath.Base(root))
	assertContains(t, content, "- Repository root: `.`")
	if strings.Contains(content, filepath.ToSlash(root)) {
		t.Fatalf("project context leaked absolute repository root:\n%s", content)
	}
	assertContains(t, content, "- `cmd`")
	assertContains(t, content, "- `internal`")
	assertContains(t, content, "- Go")
	assertContains(t, content, "- JavaScript/TypeScript")
	assertContains(t, content, "- `make test`")
	assertContains(t, content, "- `make build`")
	assertContains(t, content, "- `npm run test`")
	assertContains(t, content, "- `docs/notes/current.md`")
	assertContains(t, content, "- `docs/adr/0001-record-decisions.md`")
	assertContains(t, content, "- `AGENTS.md`")
	assertContains(t, content, "Source: `.struktly/direction.md`")
	if strings.Contains(content, "type: direction") {
		t.Fatalf("direction frontmatter leaked into project context:\n%s", content)
	}
	assertContains(t, content, "Preserve stable command output.")
	assertContains(t, content, "- `.env` / `.env.*`")
	assertContains(t, content, "Outside Git, root-level exact, directory, and glob .gitignore patterns are applied")

	if strings.Contains(content, "node_modules/noise") {
		t.Fatalf("project context should not include ignored node_modules content:\n%s", content)
	}
	if strings.Contains(content, "do-not-read") {
		t.Fatalf("project context should not include secret file contents:\n%s", content)
	}

	assertContains(t, content, "## Deprioritized Paths")
	assertContains(t, content, "- `legacy`")
	assertContains(t, content, "- `pkg/testdata`")
	if strings.Contains(content, "old-plan.md") {
		t.Fatalf("stale legacy docs should not be listed:\n%s", content)
	}
	if strings.Contains(content, "testdata/fixture") {
		t.Fatalf("testdata commands should not be collected:\n%s", content)
	}
}

func TestRankPathsByDepth(t *testing.T) {
	ranked := rankPathsByDepth([]string{
		"docs/deep/nested/plan.md",
		"README.md",
		"docs/guide.md",
	})
	want := []string{"README.md", "docs/guide.md", "docs/deep/nested/plan.md"}
	for i, path := range want {
		if ranked[i] != path {
			t.Fatalf("expected %v, got %v", want, ranked)
		}
	}
}

func TestScanUsesGitAuthoritativeNestedIgnores(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "README.md", "# Repo\n")
	writeFile(t, root, "nested/.gitignore", "/Makefile\n")
	writeFile(t, root, "nested/Makefile", "test:\n\tprint-secret-output\n")
	initGitRepo(t, root)

	result, err := Scan(ScanOptions{Root: root})
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	data, err := os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "make -C nested test") || strings.Contains(string(data), "print-secret-output") {
		t.Fatalf("scan surfaced nested ignored file:\n%s", data)
	}
}

func TestScanOmitsDeletedTrackedPathsAndCheckoutRoot(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "README.md", "# Repo\n")
	writeFile(t, root, "deleted.md", "remove me\n")
	initGitRepo(t, root)
	if err := os.Remove(filepath.Join(root, "deleted.md")); err != nil {
		t.Fatalf("remove tracked file: %v", err)
	}

	result, err := Scan(ScanOptions{Root: root})
	if err != nil {
		t.Fatalf("Scan returned error: %v", err)
	}
	data, err := os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatalf("read project context: %v", err)
	}
	if strings.Contains(string(data), filepath.ToSlash(root)) || strings.Contains(string(data), "deleted.md") {
		t.Fatalf("scan leaked checkout state:\n%s", data)
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

func mkdir(t *testing.T, root, rel string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(rel)), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
}

func assertContains(t *testing.T, content, want string) {
	t.Helper()
	if !strings.Contains(content, want) {
		t.Fatalf("expected content to contain %q\n\n%s", want, content)
	}
}
