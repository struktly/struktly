package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	repoctx "github.com/struktly/struktly/internal/context"
)

func TestInitCreatesConfigAndScans(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "README.md", "# Demo Repo\n")

	result, err := Init(InitOptions{Root: root})
	if err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	if len(result.CreatedPaths) != 1 {
		t.Fatalf("expected 1 created path, got %v", result.CreatedPaths)
	}
	if len(result.SkippedPaths) != 0 {
		t.Fatalf("expected no skipped paths, got %v", result.SkippedPaths)
	}
	if result.ScanOutputPath != filepath.Join(root, ".struktly", "project-context.md") {
		t.Fatalf("unexpected scan output path: %s", result.ScanOutputPath)
	}
	if _, err := os.Stat(result.ScanOutputPath); err != nil {
		t.Fatalf("expected project-context.md after Init: %v", err)
	}
	config, err := os.ReadFile(filepath.Join(root, ".struktly", "config.json"))
	if err != nil {
		t.Fatalf("read config.json: %v", err)
	}
	var gotConfig repoctx.Config
	if err := json.Unmarshal(config, &gotConfig); err != nil {
		t.Fatalf("parse config.json: %v", err)
	}
	if gotConfig.Schema != repoctx.ConfigSchema {
		t.Fatalf("expected config schema %q, got %q", repoctx.ConfigSchema, gotConfig.Schema)
	}
	if want := repoctx.DefaultConfig(); !reflect.DeepEqual(gotConfig, want) {
		t.Fatalf("expected default config %#v, got %#v", want, gotConfig)
	}
}

func TestInitNeverOverwritesExistingConfig(t *testing.T) {
	root := t.TempDir()

	if _, err := Init(InitOptions{Root: root}); err != nil {
		t.Fatalf("first Init returned error: %v", err)
	}
	configPath := filepath.Join(root, ".struktly", "config.json")
	configBefore := []byte("{\n  \"schema\": \"struktly/config/v1\",\n  \"context\": {\"include\": [\"docs/*.md\"]},\n  \"checks\": {}\n}\n")
	if err := os.WriteFile(configPath, configBefore, 0o644); err != nil {
		t.Fatalf("replace config.json: %v", err)
	}

	result, err := Init(InitOptions{Root: root})
	if err != nil {
		t.Fatalf("second Init returned error: %v", err)
	}
	if len(result.CreatedPaths) != 0 {
		t.Fatalf("expected no created paths on rerun, got %v", result.CreatedPaths)
	}
	if len(result.SkippedPaths) != 1 {
		t.Fatalf("expected 1 skipped path on rerun, got %v", result.SkippedPaths)
	}
	configAfter, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("reread config.json: %v", err)
	}
	if string(configAfter) != string(configBefore) {
		t.Fatalf("config.json changed on rerun:\nbefore:\n%s\nafter:\n%s", configBefore, configAfter)
	}
}
