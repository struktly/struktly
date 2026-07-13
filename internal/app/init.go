package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	repoctx "github.com/struktly/struktly/internal/context"
	"github.com/struktly/struktly/internal/files"
)

type InitOptions struct {
	Root string
}

type InitResult struct {
	CreatedPaths   []string
	SkippedPaths   []string
	ScanOutputPath string
}

// Init creates repository configuration and runs an initial scan.
func Init(opts InitOptions) (InitResult, error) {
	root, err := files.CleanRoot(opts.Root)
	if err != nil {
		return InitResult{}, err
	}
	if err := os.MkdirAll(filepath.Join(root, ".struktly"), 0o755); err != nil {
		return InitResult{}, fmt.Errorf("create .struktly dir: %w", err)
	}
	config, err := json.MarshalIndent(repoctx.DefaultConfig(), "", "  ")
	if err != nil {
		return InitResult{}, fmt.Errorf("marshal config.json: %w", err)
	}

	result := InitResult{}
	configPath := filepath.Join(root, ".struktly", "config.json")
	if _, err := os.Stat(configPath); err == nil {
		result.SkippedPaths = append(result.SkippedPaths, configPath)
	} else if err := os.WriteFile(configPath, append(config, '\n'), 0o644); err != nil {
		return InitResult{}, fmt.Errorf("write config.json: %w", err)
	} else {
		result.CreatedPaths = append(result.CreatedPaths, configPath)
	}

	scanResult, err := repoctx.Scan(repoctx.ScanOptions{Root: root})
	if err != nil {
		return InitResult{}, err
	}
	result.ScanOutputPath = scanResult.OutputPath

	return result, nil
}
