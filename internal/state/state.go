package state

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

const EnvDir = "STRUKTLY_STATE_DIR"

// RepositoryDir returns the per-user runtime-state directory for one local
// checkout. Portable declarations remain under the repository's .struktly/.
func RepositoryDir(root string) (string, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve repository state root: %w", err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(abs); resolveErr == nil {
		abs = resolved
	}
	base := os.Getenv(EnvDir)
	if base == "" {
		configDir, err := os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("resolve user state directory: %w", err)
		}
		base = filepath.Join(configDir, "struktly", "state")
	}
	digest := sha256.Sum256([]byte(filepath.Clean(abs)))
	return filepath.Join(base, "repositories", hex.EncodeToString(digest[:16])), nil
}

func Path(root string, elements ...string) (string, error) {
	dir, err := RepositoryDir(root)
	if err != nil {
		return "", err
	}
	return filepath.Join(append([]string{dir}, elements...)...), nil
}
