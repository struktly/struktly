package context

import (
	stdcontext "context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLoadConfigRejectsMalformedAndUnsafeDeclarations(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, configPath, `{"schema":"struktly/config/v1","unexpected":true}`)
	if _, _, err := LoadConfig(root); err == nil {
		t.Fatal("expected unknown config field to fail")
	}

	writeFile(t, root, configPath, `{"schema":"struktly/config/v1","context":{"include":["../outside"]},"checks":{}}`)
	if _, _, err := LoadConfig(root); err == nil {
		t.Fatal("expected traversal pattern to fail")
	}
}

func TestLoadConfigAddsPortableRulesToDefaults(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, configPath, `{
  "schema": "struktly/config/v1",
  "context": {"include": ["docs/*.md"], "exclude": ["docs/private.md"]},
  "checks": {"required": ["go test ./..."], "suggested": ["go vet ./..."]}
}`)
	cfg, declared, err := LoadConfig(root)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if !declared {
		t.Fatal("expected config to be declared")
	}
	if !containsString(cfg.Context.Include, "README.md") || !containsString(cfg.Context.Include, "docs/*.md") {
		t.Fatalf("expected default and declared include rules, got %v", cfg.Context.Include)
	}
}

func TestResolveRepositoryRequiresGitAndFindsCanonicalRoot(t *testing.T) {
	if _, err := ResolveRepository(stdcontext.Background(), t.TempDir()); !errors.Is(err, ErrNotGitRepository) {
		t.Fatalf("expected ErrNotGitRepository, got %v", err)
	}

	root := t.TempDir()
	runGit(t, root, "init", "-q", "-b", "main")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "commit.gpgsign", "false")
	writeFile(t, root, "README.md", "# Repo\n")
	runGit(t, root, "add", "README.md")
	runGit(t, root, "commit", "-qm", "initial")
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}

	repo, err := ResolveRepository(stdcontext.Background(), filepath.Join(root, "nested"))
	if err != nil {
		t.Fatalf("ResolveRepository returned error: %v", err)
	}
	wantRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	if repo.Root != "." || repo.HeadRevision == "" || repo.Identity == "" || repo.absoluteRoot != wantRoot {
		t.Fatalf("unexpected repository: %+v", repo)
	}
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_DATE=2026-07-11T00:00:00Z", "GIT_COMMITTER_DATE=2026-07-11T00:00:00Z")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
