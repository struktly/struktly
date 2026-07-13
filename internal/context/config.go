package context

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const ConfigSchema = "struktly/config/v1"

const configPath = ".struktly/config.json"

var defaultIncludePatterns = []string{
	"README.md",
	"AGENTS.md",
	"CLAUDE.md",
	"go.mod",
	"go.work",
	"package.json",
	"pyproject.toml",
	"Cargo.toml",
	"Makefile",
	".github/copilot-instructions.md",
	".cursor/rules/*.md",
	".struktly/config.json",
	".struktly/direction.md",
	".struktly/constraints.md",
	".struktly/decisions.md",
	".struktly/evidence.md",
	".struktly/memory/approved/*.json",
}

type ContextConfig struct {
	Include []string `json:"include,omitempty"`
	Exclude []string `json:"exclude,omitempty"`
}

type CheckConfig struct {
	Required  []string `json:"required,omitempty"`
	Suggested []string `json:"suggested,omitempty"`
}

type Config struct {
	Schema  string        `json:"schema"`
	Context ContextConfig `json:"context"`
	Checks  CheckConfig   `json:"checks"`
}

func DefaultConfig() Config {
	return Config{
		Schema: ConfigSchema,
		Context: ContextConfig{
			Include: append([]string(nil), defaultIncludePatterns...),
		},
	}
}

// LoadConfig reads the optional portable repository declaration. When the
// file does not exist, the built-in conservative selection rules apply.
func LoadConfig(root string) (Config, bool, error) {
	path := filepath.Join(root, filepath.FromSlash(configPath))
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultConfig(), false, nil
	}
	if err != nil {
		return Config{}, false, fmt.Errorf("read %s: %w", configPath, err)
	}

	var declared Config
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&declared); err != nil {
		return Config{}, true, fmt.Errorf("parse %s: %w", configPath, err)
	}
	if err := ensureJSONEOF(dec); err != nil {
		return Config{}, true, fmt.Errorf("parse %s: %w", configPath, err)
	}
	if err := ValidateConfig(declared); err != nil {
		return Config{}, true, fmt.Errorf("validate %s: %w", configPath, err)
	}

	defaults := DefaultConfig()
	declared.Context.Include = append(defaults.Context.Include, declared.Context.Include...)
	declared.Context.Include = uniqueSorted(declared.Context.Include)
	declared.Context.Exclude = uniqueSorted(declared.Context.Exclude)
	declared.Checks.Required = uniqueSorted(declared.Checks.Required)
	declared.Checks.Suggested = uniqueSorted(declared.Checks.Suggested)
	return declared, true, nil
}

func ValidateConfig(cfg Config) error {
	if cfg.Schema != ConfigSchema {
		return fmt.Errorf("schema must be %q", ConfigSchema)
	}
	for _, group := range []struct {
		label    string
		patterns []string
	}{
		{label: "context.include", patterns: cfg.Context.Include},
		{label: "context.exclude", patterns: cfg.Context.Exclude},
	} {
		for _, pattern := range group.patterns {
			if err := validatePortablePattern(pattern); err != nil {
				return fmt.Errorf("%s pattern %q: %w", group.label, pattern, err)
			}
		}
	}
	for _, group := range []struct {
		label  string
		checks []string
	}{
		{label: "checks.required", checks: cfg.Checks.Required},
		{label: "checks.suggested", checks: cfg.Checks.Suggested},
	} {
		for _, check := range group.checks {
			if strings.TrimSpace(check) == "" {
				return fmt.Errorf("%s entries must not be empty", group.label)
			}
			if strings.ContainsRune(check, '\x00') || strings.ContainsAny(check, "\r\n") {
				return fmt.Errorf("%s entries must be single-line commands", group.label)
			}
		}
	}
	return nil
}

func validatePortablePattern(pattern string) error {
	if pattern == "" || strings.TrimSpace(pattern) != pattern {
		return errors.New("must not be empty or have surrounding whitespace")
	}
	if strings.ContainsRune(pattern, '\x00') || strings.Contains(pattern, "\\") {
		return errors.New("must use portable forward-slash paths")
	}
	if strings.HasPrefix(pattern, "/") {
		return errors.New("must be repository-relative")
	}
	for _, part := range strings.Split(pattern, "/") {
		if part == ".." {
			return errors.New("must not traverse outside the repository")
		}
	}
	if _, err := filepath.Match(pattern, "probe"); err != nil {
		return fmt.Errorf("invalid glob: %w", err)
	}
	return nil
}

func ensureJSONEOF(dec *json.Decoder) error {
	var extra any
	if err := dec.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("multiple JSON values")
}

func uniqueSorted(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
