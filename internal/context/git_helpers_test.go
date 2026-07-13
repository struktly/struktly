package context

import (
	"os"
	"path/filepath"
	"testing"
)

func initGitRepo(t *testing.T, root string) {
	t.Helper()
	runGit(t, root, "init", "-q", "-b", "main")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "commit.gpgsign", "false")
	exclude := ".struktly/project-context.md\n.struktly/scans/\n.struktly/context-packets/\n"
	if err := os.WriteFile(filepath.Join(root, ".git", "info", "exclude"), []byte(exclude), 0o644); err != nil {
		t.Fatalf("write Git exclude: %v", err)
	}
	runGit(t, root, "add", "-A")
	runGit(t, root, "commit", "-qm", "fixture")
}
