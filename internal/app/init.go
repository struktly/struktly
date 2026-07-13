package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	repoctx "github.com/struktly/struktly/internal/context"
	"github.com/struktly/struktly/internal/files"
)

type InitOptions struct {
	Root string
	Now  time.Time
}

type InitResult struct {
	CreatedPaths   []string
	SkippedPaths   []string
	ScanOutputPath string
}

// Init scaffolds the repo-owned .struktly seed files and runs an initial
// scan. Existing seed files are never overwritten.
func Init(opts InitOptions) (InitResult, error) {
	root, err := files.CleanRoot(opts.Root)
	if err != nil {
		return InitResult{}, err
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}

	if err := os.MkdirAll(filepath.Join(root, ".struktly"), 0o755); err != nil {
		return InitResult{}, fmt.Errorf("create .struktly dir: %w", err)
	}
	config, err := json.MarshalIndent(repoctx.DefaultConfig(), "", "  ")
	if err != nil {
		return InitResult{}, fmt.Errorf("marshal config.json: %w", err)
	}

	seeds := []struct {
		name    string
		content string
	}{
		{"config.json", string(config) + "\n"},
		{"direction.md", directionTemplate(now)},
		{"constraints.md", constraintsTemplate(now)},
	}

	result := InitResult{}
	for _, seed := range seeds {
		path := filepath.Join(root, ".struktly", seed.name)
		if _, err := os.Stat(path); err == nil {
			result.SkippedPaths = append(result.SkippedPaths, path)
			continue
		}
		if err := os.WriteFile(path, []byte(seed.content), 0o644); err != nil {
			return InitResult{}, fmt.Errorf("write %s: %w", seed.name, err)
		}
		result.CreatedPaths = append(result.CreatedPaths, path)
	}

	scanResult, err := repoctx.Scan(repoctx.ScanOptions{Root: root})
	if err != nil {
		return InitResult{}, err
	}
	result.ScanOutputPath = scanResult.OutputPath

	return result, nil
}

func directionTemplate(now time.Time) string {
	return files.OKFFrontmatter("direction", "Product Direction", "Repo-owned product direction consumed by struktly briefs and instruction drafts.", now) +
		"# Product Direction\n\nDescribe what this product is and where it is heading.\n\n## Non-goals\n\n- List what this product deliberately does not do.\n"
}

func constraintsTemplate(now time.Time) string {
	return files.OKFFrontmatter("constraints", "Constraints", "Repo-owned constraints excerpted into every struktly context packet.", now) +
		"# Constraints\n\n- List hard constraints agents must respect (for example: keep changes reviewable).\n\n## Non-goals\n\n- List out-of-scope work agents must not start.\n"
}
