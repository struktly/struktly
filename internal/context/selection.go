package context

import (
	"bytes"
	stdcontext "context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/struktly/struktly/internal/files"
)

const (
	maxPacketItems      = 40
	maxPacketFileBytes  = 64 * 1024
	maxPacketTotalBytes = 512 * 1024
)

var secretContentPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}\b`),
	regexp.MustCompile(`(?i)(?:api[_-]?key|client[_-]?secret|access[_-]?token|password|passwd)\s*[:=]\s*["']?[A-Za-z0-9_./+=-]{12,}`),
}

type PacketLimits struct {
	MaxItems      int `json:"max_items"`
	MaxFileBytes  int `json:"max_file_bytes"`
	MaxTotalBytes int `json:"max_total_bytes"`
}

type PacketMetadata struct {
	GeneratedAt     string `json:"generated_at"`
	AbsoluteGitRoot string `json:"absolute_git_root"`
}

type PacketDecision struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
	Detail string `json:"detail,omitempty"`
}

type PacketItem struct {
	Kind          string     `json:"kind"`
	Path          string     `json:"path"`
	Content       string     `json:"content"`
	ContentHash   string     `json:"content_hash"`
	Provenance    Provenance `json:"provenance"`
	Reason        string     `json:"reason"`
	OriginalBytes int64      `json:"original_bytes"`
	IncludedBytes int        `json:"included_bytes"`
	Truncated     bool       `json:"truncated"`
}

type SelectionExplanation struct {
	Schema   string `json:"schema"`
	Path     string `json:"path"`
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
	Detail   string `json:"detail,omitempty"`
}

const ExplanationSchema = "struktly/explanation/v1"

type packetSelection struct {
	repository      Repository
	items           []PacketItem
	exclusions      []PacketDecision
	truncations     []PacketDecision
	requiredChecks  []string
	suggestedChecks []string
	instructions    []string
	limits          PacketLimits
}

func selectPacketContext(ctx stdcontext.Context, requestedRoot, task string, detectedChecks []string) (packetSelection, error) {
	repo, err := ResolveRepository(ctx, requestedRoot)
	if err != nil {
		return packetSelection{}, err
	}
	cfg, _, err := LoadConfig(repo.absoluteRoot)
	if err != nil {
		return packetSelection{}, err
	}
	paths, err := gitContextFiles(ctx, repo.absoluteRoot)
	if err != nil {
		return packetSelection{}, fmt.Errorf("enumerate Git files: %w", err)
	}

	result := packetSelection{
		items:           []PacketItem{},
		exclusions:      []PacketDecision{},
		truncations:     []PacketDecision{},
		repository:      repo,
		requiredChecks:  append([]string{}, cfg.Checks.Required...),
		suggestedChecks: uniqueSorted(append(append([]string(nil), cfg.Checks.Suggested...), detectedChecks...)),
		instructions:    []string{},
		limits: PacketLimits{
			MaxItems:      maxPacketItems,
			MaxFileBytes:  maxPacketFileBytes,
			MaxTotalBytes: maxPacketTotalBytes,
		},
	}
	total := 0
	for _, rel := range paths {
		reason := selectionReason(rel, task, cfg.Context.Include)
		if reason == "" {
			continue
		}
		if matchesAny(rel, cfg.Context.Exclude) {
			result.exclusions = append(result.exclusions, PacketDecision{Path: rel, Reason: "config_excluded"})
			continue
		}
		if len(result.items) >= maxPacketItems {
			result.exclusions = append(result.exclusions, PacketDecision{Path: rel, Reason: "item_limit"})
			continue
		}
		decision, item, err := inspectSelectedFile(repo, rel, reason, maxPacketTotalBytes-total)
		if err != nil {
			return packetSelection{}, err
		}
		if decision.Reason != "" {
			result.exclusions = append(result.exclusions, decision)
			continue
		}
		if item.Truncated {
			result.truncations = append(result.truncations, PacketDecision{
				Path: rel, Reason: "content_limit",
				Detail: fmt.Sprintf("included %d of %d bytes", item.IncludedBytes, item.OriginalBytes),
			})
		}
		total += item.IncludedBytes
		result.items = append(result.items, item)
		if item.Kind == "instruction" {
			result.instructions = append(result.instructions, item.Path)
		}
	}
	sort.Slice(result.items, func(i, j int) bool { return result.items[i].Path < result.items[j].Path })
	sortDecisions(result.exclusions)
	sortDecisions(result.truncations)
	sort.Strings(result.instructions)
	return result, nil
}

func gitContextFiles(ctx stdcontext.Context, root string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	parts := bytes.Split(out, []byte{0})
	paths := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		rel := filepath.ToSlash(string(part))
		if rel == ".git" || strings.HasPrefix(rel, ".git/") || defaultRuntimePath(rel) {
			continue
		}
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	return paths, nil
}

func selectionReason(rel, task string, patterns []string) string {
	if matchesAny(rel, patterns) {
		return "selection_rule"
	}
	if isAgentInstructionPath(rel) {
		return "repository_instruction"
	}
	if taskPathMatch(task, rel) {
		return "task_match"
	}
	return ""
}

func matchesAny(rel string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, _ := path.Match(pattern, rel); matched {
			return true
		}
	}
	return false
}

func taskPathMatch(task, rel string) bool {
	base := strings.ToLower(files.PathBase(rel))
	for word := range taskWords(task) {
		if len(word) >= 3 && strings.Contains(base, word) {
			return true
		}
	}
	return false
}

func inspectSelectedFile(repo Repository, rel, reason string, remaining int) (PacketDecision, PacketItem, error) {
	full := filepath.Join(repo.absoluteRoot, filepath.FromSlash(rel))
	info, err := os.Lstat(full)
	if err != nil {
		return PacketDecision{Path: rel, Reason: "unreadable", Detail: "cannot inspect file"}, PacketItem{}, nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return PacketDecision{Path: rel, Reason: "symlink"}, PacketItem{}, nil
	}
	if !info.Mode().IsRegular() {
		return PacketDecision{Path: rel, Reason: "non_regular"}, PacketItem{}, nil
	}
	if files.IsSensitivePath(rel) {
		return PacketDecision{Path: rel, Reason: "sensitive_path"}, PacketItem{}, nil
	}

	f, err := os.Open(full)
	if err != nil {
		return PacketDecision{Path: rel, Reason: "unreadable", Detail: "cannot open file"}, PacketItem{}, nil
	}
	defer f.Close()
	prefix, err := io.ReadAll(io.LimitReader(f, maxPacketFileBytes+utf8.UTFMax))
	if err != nil {
		return PacketDecision{}, PacketItem{}, fmt.Errorf("read %s: %w", rel, err)
	}
	content, binary := safeTextPrefix(prefix, info.Size())
	if binary {
		return PacketDecision{Path: rel, Reason: "binary"}, PacketItem{}, nil
	}
	if containsSecret(content) {
		return PacketDecision{Path: rel, Reason: "secret_detected"}, PacketItem{}, nil
	}
	if remaining <= 0 {
		return PacketDecision{Path: rel, Reason: "total_limit"}, PacketItem{}, nil
	}
	if len(content) > remaining {
		content = truncateUTF8(content, remaining)
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return PacketDecision{}, PacketItem{}, fmt.Errorf("hash %s: %w", rel, err)
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return PacketDecision{}, PacketItem{}, fmt.Errorf("hash %s: %w", rel, err)
	}
	item := PacketItem{
		Kind:        packetItemKind(rel),
		Path:        rel,
		Content:     content,
		ContentHash: "sha256:" + hex.EncodeToString(h.Sum(nil)),
		Provenance: Provenance{
			Source: rel, Revision: repo.HeadRevision, Method: reason, Confidence: "detected",
		},
		Reason:        reason,
		OriginalBytes: info.Size(),
		IncludedBytes: len(content),
		Truncated:     int64(len(content)) < info.Size(),
	}
	return PacketDecision{}, item, nil
}

func safeTextPrefix(data []byte, size int64) (string, bool) {
	limit := len(data)
	if limit > maxPacketFileBytes {
		limit = maxPacketFileBytes
	}
	for cut := limit; cut >= limit-(utf8.UTFMax-1) && cut >= 0; cut-- {
		candidate := data[:cut]
		if !bytes.ContainsRune(candidate, 0) && utf8.Valid(candidate) {
			if size <= maxPacketFileBytes && int64(cut) != size {
				continue
			}
			return string(candidate), false
		}
	}
	return "", true
}

func truncateUTF8(content string, limit int) string {
	if len(content) <= limit {
		return content
	}
	for limit > 0 && !utf8.ValidString(content[:limit]) {
		limit--
	}
	return content[:limit]
}

func containsSecret(content string) bool {
	for _, pattern := range secretContentPatterns {
		if pattern.MatchString(content) {
			return true
		}
	}
	return false
}

func packetItemKind(rel string) string {
	switch {
	case isAgentInstructionPath(rel):
		return "instruction"
	case strings.HasPrefix(rel, ".struktly/tasks/"):
		return "task"
	case strings.HasPrefix(rel, ".struktly/"):
		return "declaration"
	case isDocPath(rel):
		return "documentation"
	case files.PathBase(rel) == "go.mod", files.PathBase(rel) == "go.work", files.PathBase(rel) == "package.json", files.PathBase(rel) == "pyproject.toml", files.PathBase(rel) == "Cargo.toml", files.PathBase(rel) == "Makefile":
		return "manifest"
	default:
		return "source"
	}
}

func defaultRuntimePath(rel string) bool {
	for _, prefix := range files.DefaultIgnoredPaths {
		if rel == prefix || strings.HasPrefix(rel, prefix+"/") {
			return true
		}
	}
	for _, dir := range files.DefaultIgnoredDirs {
		for _, part := range strings.Split(rel, "/") {
			if part == dir {
				return true
			}
		}
	}
	return false
}

func sortDecisions(values []PacketDecision) {
	sort.Slice(values, func(i, j int) bool {
		if values[i].Path == values[j].Path {
			return values[i].Reason < values[j].Reason
		}
		return values[i].Path < values[j].Path
	})
}

func ExplainSelection(ctx stdcontext.Context, requestedRoot, requestedPath, task string) (SelectionExplanation, error) {
	repo, err := ResolveRepository(ctx, requestedRoot)
	if err != nil {
		return SelectionExplanation{}, err
	}
	rel, err := cleanRequestedPath(repo.absoluteRoot, requestedPath)
	if err != nil {
		return SelectionExplanation{}, err
	}
	cfg, _, err := LoadConfig(repo.absoluteRoot)
	if err != nil {
		return SelectionExplanation{}, err
	}
	explanation := SelectionExplanation{Schema: ExplanationSchema, Path: rel, Decision: "excluded"}
	if rel == ".git" || strings.HasPrefix(rel, ".git/") || defaultRuntimePath(rel) {
		explanation.Reason = "default_excluded"
		return explanation, nil
	}
	if files.IsSensitivePath(rel) {
		explanation.Reason = "sensitive_path"
		return explanation, nil
	}
	tracked := gitPathTracked(ctx, repo.absoluteRoot, rel)
	if !tracked && gitPathIgnored(ctx, repo.absoluteRoot, rel) {
		explanation.Reason = "git_ignored"
		return explanation, nil
	}
	if matchesAny(rel, cfg.Context.Exclude) {
		explanation.Reason = "config_excluded"
		return explanation, nil
	}
	reason := selectionReason(rel, task, cfg.Context.Include)
	if reason == "" {
		explanation.Reason = "not_selected"
		return explanation, nil
	}
	decision, _, err := inspectSelectedFile(repo, rel, reason, maxPacketTotalBytes)
	if err != nil {
		return SelectionExplanation{}, err
	}
	if decision.Reason != "" {
		explanation.Reason = decision.Reason
		explanation.Detail = decision.Detail
		return explanation, nil
	}
	explanation.Decision = "included"
	explanation.Reason = reason
	return explanation, nil
}

func cleanRequestedPath(root, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return "", errors.New("path is required")
	}
	full := requested
	if !filepath.IsAbs(full) {
		full = filepath.Join(root, filepath.FromSlash(requested))
	}
	full = filepath.Clean(full)
	rel, err := filepath.Rel(root, full)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("path must stay inside the Git repository")
	}
	return filepath.ToSlash(rel), nil
}

func gitPathTracked(ctx stdcontext.Context, root, rel string) bool {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "--error-unmatch", "--", rel)
	return cmd.Run() == nil
}

func gitPathIgnored(ctx stdcontext.Context, root, rel string) bool {
	cmd := exec.CommandContext(ctx, "git", "-C", root, "check-ignore", "--quiet", "--no-index", "--", rel)
	return cmd.Run() == nil
}
