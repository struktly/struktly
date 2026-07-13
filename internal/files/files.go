package files

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
)

var DefaultIgnoredDirs = []string{
	".git",
	"node_modules",
	"vendor",
	".venv",
	"venv",
	"dist",
	"build",
	".next",
	".turbo",
	".cache",
	"__pycache__",
	".pytest_cache",
	".yarn",
	"coverage",
	"site",
	"tmp",
	"logs",
	".terraform",
	".direnv",
}

var DefaultIgnoredPaths = []string{
	".struktly/runs",
	".struktly/memory/candidates",
	".struktly/context-packets",
	".struktly/scans",
	".struktly/ui",
	".claude/projects",
	".claude/sessions",
	".codex/sessions",
	".continue/sessions",
	".aws",
	".azure",
	".config/gcloud",
}

var sensitivePatterns = []string{
	".env",
	".env.*",
	"*.pem",
	"*.key",
	"*.p12",
	"*.pfx",
	"id_rsa",
	"id_ed25519",
	"*secret*",
	"*credential*",
	"*token*",
}

type IgnoreMatcher struct {
	rootPatterns []string
}

func NewIgnoreMatcher(root string) IgnoreMatcher {
	m := IgnoreMatcher{}
	data, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		return m
	}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		m.rootPatterns = append(m.rootPatterns, strings.TrimSuffix(filepath.ToSlash(line), "/"))
	}
	return m
}

func (m IgnoreMatcher) ShouldSkip(rel string, isDir bool) bool {
	rel = filepath.ToSlash(strings.TrimPrefix(rel, "./"))
	base := PathBase(rel)

	if isDir {
		for _, dir := range DefaultIgnoredDirs {
			if base == dir || rel == dir || strings.HasPrefix(rel, dir+"/") || strings.Contains(rel, "/"+dir+"/") {
				return true
			}
		}
	}

	for _, ignored := range DefaultIgnoredPaths {
		if rel == ignored || strings.HasPrefix(rel, ignored+"/") {
			return true
		}
	}

	if isSensitivePath(rel) {
		return true
	}

	for _, pattern := range m.rootPatterns {
		if pattern == "" {
			continue
		}
		if matchSimplePattern(rel, base, pattern) {
			return true
		}
		if isDir && (rel == pattern || strings.HasPrefix(rel, pattern+"/")) {
			return true
		}
	}

	return false
}

func isSensitivePath(rel string) bool {
	rel = filepath.ToSlash(rel)
	base := strings.ToLower(PathBase(rel))
	lowerRel := strings.ToLower(rel)

	for _, pattern := range sensitivePatterns {
		pattern = strings.ToLower(pattern)
		if matchSimplePattern(lowerRel, base, pattern) {
			return true
		}
	}

	return false
}

// IsSensitivePath reports whether a repository-relative path matches a
// built-in credential or secret filename pattern.
func IsSensitivePath(rel string) bool {
	return isSensitivePath(rel)
}

func matchSimplePattern(rel, base, pattern string) bool {
	pattern = filepath.ToSlash(pattern)
	if rel == pattern || base == pattern {
		return true
	}
	if strings.HasSuffix(pattern, "/") {
		pattern = strings.TrimSuffix(pattern, "/")
		if rel == pattern || strings.HasPrefix(rel, pattern+"/") {
			return true
		}
	}
	if strings.Contains(pattern, "*") {
		if ok, _ := filepath.Match(pattern, rel); ok {
			return true
		}
		if ok, _ := filepath.Match(pattern, base); ok {
			return true
		}
	}
	return false
}

func CleanRoot(root string) (string, error) {
	if root == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve root: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("stat root: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("root is not a directory: %s", abs)
	}
	return abs, nil
}

func RelPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func PathBase(path string) string {
	path = strings.TrimSuffix(filepath.ToSlash(path), "/")
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}

func AddString(set map[string]struct{}, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	set[value] = struct{}{}
}

func SortedStrings(set map[string]struct{}) []string {
	values := make([]string, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func LimitStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return values[:limit]
}

func ReadSmallTextFile(path string, maxBytes int64) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.Size() > maxBytes {
		return "", fmt.Errorf("file too large: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func FileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

var slugSeparator = regexp.MustCompile(`-+`)

func Slugify(value string, maxLen int) string {
	var b strings.Builder
	lastDash := false
	for _, r := range strings.ToLower(value) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastDash = false
		case !lastDash:
			b.WriteByte('-')
			lastDash = true
		}
	}
	slug := strings.Trim(slugSeparator.ReplaceAllString(b.String(), "-"), "-")
	if slug == "" {
		slug = "task"
	}
	if maxLen > 0 && len(slug) > maxLen {
		slug = strings.Trim(slug[:maxLen], "-")
	}
	return slug
}

func NormalizeNow(now time.Time) time.Time {
	if now.IsZero() {
		return time.Now().UTC()
	}
	return now.UTC()
}
