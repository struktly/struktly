package state

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryDirUsesOverrideAndStableCheckoutKey(t *testing.T) {
	base := t.TempDir()
	t.Setenv(EnvDir, base)
	root := t.TempDir()
	first, err := RepositoryDir(root)
	if err != nil {
		t.Fatal(err)
	}
	second, err := RepositoryDir(filepath.Join(root, "."))
	if err != nil {
		t.Fatal(err)
	}
	if first != second || !strings.HasPrefix(first, filepath.Join(base, "repositories")+string(filepath.Separator)) {
		t.Fatalf("unexpected repository state paths: %q %q", first, second)
	}
}
