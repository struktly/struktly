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
	"unicode"
	"unicode/utf8"

	"github.com/struktly/struktly/internal/files"
)

const (
	maxPacketItems      = 40
	maxPacketFileBytes  = 64 * 1024
	maxPacketTotalBytes = 512 * 1024
)

var stopWords = map[string]struct{}{
	"a":          {},
	"about":      {},
	"an":         {},
	"and":        {},
	"are":        {},
	"as":         {},
	"at":         {},
	"codebase":   {},
	"does":       {},
	"for":        {},
	"how":        {},
	"in":         {},
	"it":         {},
	"on":         {},
	"or":         {},
	"project":    {},
	"repo":       {},
	"repository": {},
	"that":       {},
	"the":        {},
	"this":       {},
	"to":         {},
	"what":       {},
	"when":       {},
	"where":      {},
	"which":      {},
	"with":       {},
}

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

func DefaultPacketLimits() PacketLimits {
	return PacketLimits{
		MaxItems:      maxPacketItems,
		MaxFileBytes:  maxPacketFileBytes,
		MaxTotalBytes: maxPacketTotalBytes,
	}
}

func resolvePacketLimits(overrides PacketLimits) (PacketLimits, error) {
	limits := DefaultPacketLimits()
	if overrides.MaxItems != 0 {
		if overrides.MaxItems <= 0 {
			return PacketLimits{}, fmt.Errorf("max_items must be greater than 0")
		}
		if overrides.MaxItems > limits.MaxItems {
			return PacketLimits{}, fmt.Errorf("max_items exceeds default max %d", limits.MaxItems)
		}
		limits.MaxItems = overrides.MaxItems
	}
	if overrides.MaxFileBytes != 0 {
		if overrides.MaxFileBytes <= 0 {
			return PacketLimits{}, fmt.Errorf("max_file_bytes must be greater than 0")
		}
		if overrides.MaxFileBytes > limits.MaxFileBytes {
			return PacketLimits{}, fmt.Errorf("max_file_bytes exceeds default max %d", limits.MaxFileBytes)
		}
		limits.MaxFileBytes = overrides.MaxFileBytes
	}
	if overrides.MaxTotalBytes != 0 {
		if overrides.MaxTotalBytes <= 0 {
			return PacketLimits{}, fmt.Errorf("max_total_bytes must be greater than 0")
		}
		if overrides.MaxTotalBytes > limits.MaxTotalBytes {
			return PacketLimits{}, fmt.Errorf("max_total_bytes exceeds default max %d", limits.MaxTotalBytes)
		}
		limits.MaxTotalBytes = overrides.MaxTotalBytes
	}
	return limits, nil
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

func selectPacketContext(ctx stdcontext.Context, requestedRoot, task string, detectedChecks []string, limits PacketLimits) (packetSelection, error) {
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
		limits:          limits,
	}

	candidates := make([]packetCandidate, 0, len(paths))
	for _, rel := range paths {
		reason := selectionReason(rel, task, cfg.Context.Include)
		if reason == "" {
			continue
		}
		if matchesAny(rel, cfg.Context.Exclude) {
			result.exclusions = append(result.exclusions, PacketDecision{Path: rel, Reason: "config_excluded"})
			continue
		}
		candidates = append(candidates, packetCandidate{
			path:        rel,
			reason:      reason,
			relevance:   taskMatchScore(task, rel, reason),
			kindPenalty: pathPriority(rel),
		})
	}

	sort.Slice(candidates, func(i, j int) bool { return rankCandidates(candidates[i], candidates[j]) })

	total := 0
	for _, candidate := range candidates {
		rel := candidate.path
		reason := candidate.reason
		if len(result.items) >= limits.MaxItems {
			result.exclusions = append(result.exclusions, PacketDecision{Path: rel, Reason: "item_limit"})
			continue
		}
		decision, item, err := inspectSelectedFile(repo, rel, reason, limits.MaxTotalBytes-total, limits.MaxFileBytes)
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
	result.limits = limits
	return result, nil
}

type packetCandidate struct {
	path        string
	reason      string
	relevance   int
	kindPenalty int
}

func rankCandidates(left, right packetCandidate) bool {
	leftPriority := selectionReasonPriority(left.reason)
	rightPriority := selectionReasonPriority(right.reason)
	if leftPriority != rightPriority {
		return leftPriority < rightPriority
	}
	if left.relevance != right.relevance {
		return left.relevance > right.relevance
	}
	if left.kindPenalty != right.kindPenalty {
		return left.kindPenalty < right.kindPenalty
	}
	return left.path < right.path
}

func selectionReasonPriority(reason string) int {
	switch reason {
	case "selection_rule", "repository_instruction":
		return 0
	case "task_match":
		return 2
	default:
		return 4
	}
}

func pathPriority(path string) int {
	lowerPath := strings.ToLower(path)
	if strings.HasPrefix(lowerPath, ".struktly/tasks/") {
		return 4
	}
	if strings.Contains(lowerPath, "e2e") || strings.Contains(lowerPath, "/test/") || strings.Contains(lowerPath, "/tests/") || strings.Contains(lowerPath, "_test") || strings.Contains(lowerPath, "/test.") {
		return 3
	}
	return 0
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
	return taskMatchScore(task, rel, "task_match") > 0
}

func taskMatchScore(task, rel, reason string) int {
	if reason != "task_match" {
		return 0
	}
	words := selectionTaskWords(task)
	if len(words) == 0 {
		return 0
	}
	match := 0
	for token := range pathTokens(rel) {
		if _, ok := words[token]; ok {
			match++
		}
	}
	return match
}

func pathTokens(rel string) map[string]struct{} {
	tokens := map[string]struct{}{}
	for _, token := range strings.FieldsFunc(strings.ToLower(filepath.ToSlash(rel)), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		for _, segment := range splitToken(token) {
			if len(segment) < 3 {
				continue
			}
			tokens[segment] = struct{}{}
		}
	}
	return tokens
}

func splitToken(value string) []string {
	parts := splitCamelToken(value)
	if len(parts) > 0 {
		return parts
	}
	if _, stopped := stopWords[value]; !stopped {
		return []string{value}
	}
	return nil

}

func splitCamelToken(value string) []string {
	if value == "" {
		return nil
	}
	runes := []rune(value)
	out := []string{}
	start := 0
	for i := 1; i < len(runes); i++ {
		if isCamelBoundary(runes, i) {
			out = append(out, string(runes[start:i]))
			start = i
		}
	}
	out = append(out, string(runes[start:]))
	return out
}

func isCamelBoundary(runes []rune, index int) bool {
	if index <= 0 || index >= len(runes) {
		return false
	}
	current := runes[index]
	previous := runes[index-1]
	var next rune
	hasNext := index+1 < len(runes)
	if hasNext {
		next = runes[index+1]
	}
	if unicode.IsLower(previous) && unicode.IsUpper(current) {
		return true
	}
	if unicode.IsDigit(previous) != unicode.IsDigit(current) {
		return true
	}
	if unicode.IsUpper(previous) && unicode.IsUpper(current) && hasNext && unicode.IsLower(next) {
		return true
	}
	return false
}

func selectionTaskWords(task string) map[string]struct{} {
	words := map[string]struct{}{}
	for word := range pathTokens(task) {
		if _, stopped := stopWords[word]; stopped {
			continue
		}
		if len(word) >= 3 {
			words[word] = struct{}{}
		}
	}
	return words
}

func inspectSelectedFile(repo Repository, rel, reason string, remaining, maxFileBytes int) (PacketDecision, PacketItem, error) {
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
	prefix, err := io.ReadAll(io.LimitReader(f, int64(maxFileBytes)+utf8.UTFMax))
	if err != nil {
		return PacketDecision{}, PacketItem{}, fmt.Errorf("read %s: %w", rel, err)
	}
	content, binary := safeTextPrefix(prefix, info.Size(), maxFileBytes)
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

func safeTextPrefix(data []byte, size int64, maxFileBytes int) (string, bool) {
	limit := len(data)
	if limit > maxFileBytes {
		limit = maxFileBytes
	}
	for cut := limit; cut >= limit-(utf8.UTFMax-1) && cut >= 0; cut-- {
		candidate := data[:cut]
		if !bytes.ContainsRune(candidate, 0) && utf8.Valid(candidate) {
			if size <= int64(maxFileBytes) && int64(cut) != size {
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
	decision, _, err := inspectSelectedFile(repo, rel, reason, maxPacketTotalBytes, maxPacketFileBytes)
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
