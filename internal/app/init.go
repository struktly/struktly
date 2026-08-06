package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	repoctx "github.com/struktly/struktly/internal/context"
)

type InitOptions struct {
	Root string
}

type InitResult struct {
	// Root is the anchored repository root, which is not necessarily the
	// requested one. Callers report paths relative to it.
	Root           string
	CreatedPaths   []string
	SkippedPaths   []string
	ScanOutputPath string
}

// Init creates repository configuration and runs an initial scan.
func Init(opts InitOptions) (InitResult, error) {
	// Anchored at the Git top level so the config init writes is the config
	// every other command reads. See repoctx.AnchorRoot.
	root, err := repoctx.AnchorRoot(context.Background(), opts.Root)
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

	result := InitResult{Root: root}
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
