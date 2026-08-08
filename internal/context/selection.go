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

// stopWords are the words in a request that say nothing about which files it
// concerns. The second group is verbs: a request names an action and a subject,
// and only the subject identifies code. Without them "add request timeout"
// matched every AddString and addOpenQuestion in the repository.
//
// The list stays short and evidence-backed on purpose. A batch of apparent
// function words — one, per, any, each, from — was written and then measured,
// and "one" alone dropped four correctly-selected files from a repository whose
// vocabulary includes "one composer" and "one release pipeline". A word that
// looks generic in isolation can be a project's own terminology, so nothing
// joins this list without a measurement showing it removes noise rather than
// signal.
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

	"add":       {},
	"allow":     {},
	"change":    {},
	"create":    {},
	"delete":    {},
	"ensure":    {},
	"fix":       {},
	"handle":    {},
	"implement": {},
	"improve":   {},
	"make":      {},
	"refactor":  {},
	"remove":    {},
	"support":   {},
	"update":    {},
	"use":       {},
}

var ErrInvalidPacketLimit = errors.New("invalid packet limit")

var secretContentPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)-----BEGIN (?:RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{20,}\b`),
	// GitHub fine-grained tokens do not use the gh?_ shape at all.
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{36,}\b`),
	regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}\b`),
	regexp.MustCompile(`\b[sr]k_(?:live|test)_[A-Za-z0-9]{16,}\b`),
	regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`),
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
			return PacketLimits{}, fmt.Errorf("%w: max_items must be greater than 0", ErrInvalidPacketLimit)
		}
		if overrides.MaxItems > limits.MaxItems {
			return PacketLimits{}, fmt.Errorf("%w: max_items exceeds default max %d", ErrInvalidPacketLimit, limits.MaxItems)
		}
		limits.MaxItems = overrides.MaxItems
	}
	if overrides.MaxFileBytes != 0 {
		if overrides.MaxFileBytes <= 0 {
			return PacketLimits{}, fmt.Errorf("%w: max_file_bytes must be greater than 0", ErrInvalidPacketLimit)
		}
		if overrides.MaxFileBytes > limits.MaxFileBytes {
			return PacketLimits{}, fmt.Errorf("%w: max_file_bytes exceeds default max %d", ErrInvalidPacketLimit, limits.MaxFileBytes)
		}
		limits.MaxFileBytes = overrides.MaxFileBytes
	}
	if overrides.MaxTotalBytes != 0 {
		if overrides.MaxTotalBytes <= 0 {
			return PacketLimits{}, fmt.Errorf("%w: max_total_bytes must be greater than 0", ErrInvalidPacketLimit)
		}
		if overrides.MaxTotalBytes > limits.MaxTotalBytes {
			return PacketLimits{}, fmt.Errorf("%w: max_total_bytes exceeds default max %d", ErrInvalidPacketLimit, limits.MaxTotalBytes)
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
	Kind        string     `json:"kind"`
	Path        string     `json:"path"`
	Content     string     `json:"content"`
	ContentHash string     `json:"content_hash"`
	Provenance  Provenance `json:"provenance"`
	Reason      string     `json:"reason"`
	// Rendering names the form Content takes when it is not verbatim source.
	// Absent means verbatim; "declarations" means function bodies were omitted,
	// which a consumer must know before reading a body-less function as one
	// that does nothing.
	Rendering     string `json:"rendering,omitempty"`
	OriginalBytes int64  `json:"original_bytes"`
	IncludedBytes int    `json:"included_bytes"`
	Truncated     bool   `json:"truncated"`
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
	readWarnings    []string
	scope           string
	seeds           []string
	limits          PacketLimits
}

type limitDecision struct {
	count int
	first string
	last  string
}

// selectionRequest is everything one selection depends on. It became a struct
// when the parameter list reached seven; the fields are the request, not
// options, so every one of them belongs to packet identity.
type selectionRequest struct {
	root   string
	task   string
	scope  string
	seeds  []string
	checks []string
	limits PacketLimits
}

func selectPacketContext(ctx stdcontext.Context, req selectionRequest) (packetSelection, error) {
	requestedRoot, task, scope, limits := req.root, req.task, req.scope, req.limits
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
		suggestedChecks: uniqueSorted(append(append([]string(nil), cfg.Checks.Suggested...), req.checks...)),
		instructions:    []string{},
		readWarnings:    []string{},
		limits:          limits,
	}
	limitOmissions := map[string]limitDecision{
		"item_limit":  {},
		"total_limit": {},
	}

	// Symbol matching only adds candidates: a repository in another language,
	// or one where nothing parses, selects exactly what it selected before.
	notSelectable := func(rel string) bool {
		return !withinScope(rel, scope) || ignoredDirPath(rel) || matchesAny(rel, cfg.Context.Exclude)
	}
	content := buildContentIndex(repo.absoluteRoot, paths, selectionTaskWords(task), notSelectable)
	if content.skipped > 0 {
		result.readWarnings = append(result.readWarnings, fmt.Sprintf(
			"Content matching covered %d files and stopped at the %d-file limit; %d were not indexed.",
			content.indexed, maxIndexedSymbolFiles, content.skipped))
	}

	seeded := make(map[string]struct{}, len(req.seeds))
	for _, rel := range req.seeds {
		seeded[rel] = struct{}{}
	}

	candidates := make([]packetCandidate, 0, len(paths))
	for _, rel := range paths {
		// Out of scope is not an exclusion worth recording: the caller asked
		// for a subtree, so naming every file outside it would bury the
		// decisions they did not make in decisions they did.
		if !withinScope(rel, scope) {
			continue
		}
		reason := selectionReason(rel, task, cfg.Context.Include)
		symbol := content.match(rel)
		if _, isSeed := seeded[rel]; isSeed {
			// Naming a file gets it considered, not included: it still goes
			// through every exclusion below, like any other candidate.
			reason = "seed"
		} else if reason == "" {
			if symbol.score == 0 {
				continue
			}
			reason = symbol.reason
		}
		// A tracked file under a directory named build/ or dist/ used to vanish
		// before selection, so a packet omitted it with nothing recording that
		// it had. It is excluded for the same reason as before, but the reason
		// is now in the packet. Only candidates that would otherwise have been
		// selected are listed, so this does not flood the record.
		if ignoredDirPath(rel) {
			result.exclusions = append(result.exclusions, PacketDecision{
				Path: rel, Reason: "default_excluded",
				Detail: "dependency, build, cache, or local runtime path",
			})
			continue
		}
		if matchesAny(rel, cfg.Context.Exclude) {
			result.exclusions = append(result.exclusions, PacketDecision{Path: rel, Reason: "config_excluded"})
			continue
		}
		candidates = append(candidates, packetCandidate{
			path: rel,
			// Relevance is how many distinct request words the file answers by
			// any means, counted once. A file both named for the request and
			// declaring what it names still outranks one carrying half the
			// evidence, because its two sources cover different words.
			reason:      reason,
			relevance:   len(mergedMatchWords(task, rel, symbol.words)),
			symbols:     symbol.names,
			evidence:    symbol.reason,
			kindPenalty: pathPriority(rel),
		})
	}

	sort.Slice(candidates, func(i, j int) bool { return rankCandidates(candidates[i], candidates[j]) })

	countOmission := func(limit, rel string) {
		entry := limitOmissions[limit]
		entry.count++
		if entry.count == 1 {
			entry.first = rel
		}
		entry.last = rel
		limitOmissions[limit] = entry
	}

	total := 0
	// Two passes over one budget. The second considers files reachable from
	// what the first selected, and runs afterwards so it can only use the
	// budget the request itself did not need — an import neighbour is weaker
	// evidence than a direct match and must never displace one.
	considered := map[string]struct{}{}
	for _, candidate := range candidates {
		considered[candidate.path] = struct{}{}
	}
	admit := func(candidate packetCandidate) error {
		rel := candidate.path
		reason := candidate.reason
		// Past the item limit a candidate is still classified, because
		// "omitted 40 candidates that did not fit" is a different claim from
		// "omitted 40 candidates, some of which were secrets and could never
		// have been included". Classification stops before hashing, so the
		// honest count does not cost a full read of every remaining file.
		atItemLimit := len(result.items) >= limits.MaxItems
		inspection, err := inspectSelectedFile(repo, rel, reason, limits.MaxTotalBytes-total, limits.MaxFileBytes, atItemLimit)
		if err != nil {
			return err
		}
		if inspection.decision.Reason != "" {
			if inspection.decision.Reason == "total_limit" {
				countOmission("total_limit", rel)
				return nil
			}
			result.exclusions = append(result.exclusions, inspection.decision)
			return nil
		}
		if atItemLimit {
			countOmission("item_limit", rel)
			return nil
		}
		item := inspection.item
		if reason == "seed" {
			item.Provenance.Confidence = "declared"
		}
		if len(candidate.symbols) > 0 {
			label := "declares:"
			switch candidate.evidence {
			case "title_match":
				label = "titled:"
			case "import_neighbor":
				label = "provides:"
			}
			item.Provenance.Location = label + strings.Join(candidate.symbols, ",")
		}
		if inspection.truncatedBy != "" {
			result.truncations = append(result.truncations, PacketDecision{
				Path: rel, Reason: inspection.truncatedBy,
				Detail: truncationDetail(inspection.truncatedBy, item, limits),
			})
		}
		total += item.IncludedBytes
		result.items = append(result.items, item)
		if item.Kind == "instruction" {
			result.instructions = append(result.instructions, item.Path)
		}
		return nil
	}

	for _, candidate := range candidates {
		if err := admit(candidate); err != nil {
			return packetSelection{}, err
		}
	}

	if len(result.items) < limits.MaxItems && total < limits.MaxTotalBytes {
		// Expansion follows only from files the request named directly or the
		// caller seeded. A file selected because it happens to declare one
		// matching identifier is a weak enough signal on its own; expanding
		// from it multiplies that weakness by its whole import surface, which
		// is how a request about documentation acquired fifteen Go files from
		// cmd/struktly/main.go on the strength of a type called errorDocument.
		roots := make([]PacketItem, 0, len(result.items))
		for _, item := range result.items {
			if item.Reason == "seed" || item.Reason == "task_match" {
				roots = append(roots, item)
			}
		}
		neighbors := findImportNeighbors(repo.absoluteRoot, roots, considered, paths, func(rel string) bool {
			return withinScope(rel, scope) && !ignoredDirPath(rel) && !matchesAny(rel, cfg.Context.Exclude)
		})
		expansion := make([]packetCandidate, 0, len(neighbors))
		for _, neighbor := range neighbors {
			expansion = append(expansion, packetCandidate{
				path:        neighbor.path,
				reason:      "import_neighbor",
				relevance:   len(neighbor.provides),
				symbols:     neighbor.provides,
				evidence:    "import_neighbor",
				kindPenalty: pathPriority(neighbor.path),
			})
		}
		for _, candidate := range expansion {
			if err := admit(candidate); err != nil {
				return packetSelection{}, err
			}
		}
	}

	if omitted := limitOmissions["item_limit"]; omitted.count > 0 {
		result.exclusions = append(result.exclusions, PacketDecision{
			Path:   "item_limit",
			Reason: "item_limit",
			Detail: limitExclusionDetail("item_limit", omitted.count, omitted.first, omitted.last),
		})
	}
	if omitted := limitOmissions["total_limit"]; omitted.count > 0 {
		result.exclusions = append(result.exclusions, PacketDecision{
			Path:   "total_limit",
			Reason: "total_limit",
			Detail: limitExclusionDetail("total_limit", omitted.count, omitted.first, omitted.last),
		})
	}
	sort.Slice(result.items, func(i, j int) bool { return result.items[i].Path < result.items[j].Path })
	sortDecisions(result.exclusions)
	sortDecisions(result.truncations)
	sort.Strings(result.instructions)
	result.limits = limits
	result.scope = scope
	result.seeds = req.seeds
	return result, nil
}

type packetCandidate struct {
	path      string
	reason    string
	relevance int
	symbols   []string
	// evidence names which content signal produced symbols, which is not always
	// the selection reason: a file can match by path and still carry a title.
	evidence    string
	kindPenalty int
}

// mergedMatchWords is every request word the file answers, from its path and
// from its content, counted once.
func mergedMatchWords(task, rel string, content map[string]struct{}) map[string]struct{} {
	merged := map[string]struct{}{}
	words := selectionTaskWords(task)
	for token := range pathTokens(rel) {
		if _, ok := words[token]; ok {
			merged[token] = struct{}{}
		}
	}
	for token := range content {
		merged[token] = struct{}{}
	}
	return merged
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
	// The caller said so, which outranks anything the CLI worked out.
	case "seed":
		return 0
	case "selection_rule", "repository_instruction":
		return 1
	// A file that says what it is about is stronger evidence than one merely
	// named like it.
	case "symbol_match", "title_match":
		return 2
	case "task_match":
		return 3
	// Reachable from something selected, which is a reason to look rather than
	// evidence about the request. Ranked last so it fills budget the request
	// did not need instead of competing for it.
	case "import_neighbor":
		return 4
	default:
		return 5
	}
}

func pathPriority(path string) int {
	lowerPath := strings.ToLower(path)
	if strings.HasPrefix(lowerPath, ".struktly/tasks/") {
		return 4
	}
	if strings.Contains(lowerPath, "e2e") || strings.Contains(lowerPath, "/test/") || strings.Contains(lowerPath, "/tests/") || strings.Contains(lowerPath, "_test") || strings.Contains(lowerPath, "_test.") || strings.Contains(lowerPath, ".test") || strings.Contains(lowerPath, ".spec") {
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
		// Git internals and generated runtime state are dropped here.
		// Directory conventions like build/ and dist/ are not: those can hold
		// tracked source, so the selector records them as exclusions instead.
		if rel == ".git" || strings.HasPrefix(rel, ".git/") || runtimeStatePath(rel) {
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
	for _, token := range strings.FieldsFunc(filepath.ToSlash(rel), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		for _, segment := range splitToken(token) {
			segment = strings.ToLower(segment)
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
	return []string{value}
}

// truncationDetail names the limit that actually cut the file short. A file
// stopped by the packet budget used to be reported as `content_limit`, which
// humanReason renders as "per-file size limit reached" — a limit that may have
// had nothing to do with it.
func truncationDetail(truncatedBy string, item PacketItem, limits PacketLimits) string {
	detail := fmt.Sprintf("included %d of %d bytes", item.IncludedBytes, item.OriginalBytes)
	if item.Rendering == declarationRendering {
		detail += " as declarations, function bodies omitted"
	}
	if truncatedBy == "total_limit" {
		return fmt.Sprintf("%s; the %d-byte packet budget was exhausted", detail, limits.MaxTotalBytes)
	}
	return fmt.Sprintf("%s; the per-file limit is %d bytes", detail, limits.MaxFileBytes)
}

func limitExclusionDetail(reason string, count int, first, last string) string {
	reasonLabel := strings.ReplaceAll(reason, "_", " ")
	reasonLabel = strings.TrimSuffix(reasonLabel, " limit")
	detail := fmt.Sprintf("omitted %d matching candidates due to %s limit", count, reasonLabel)
	if first == "" {
		return detail
	}
	if first == last {
		return detail + "; first/last: " + first
	}
	return detail + "; first: " + first + "; last: " + last
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

// fileInspection is the outcome of classifying one candidate. Exactly one of
// decision and item is meaningful: a non-empty decision.Reason means the file
// was excluded, otherwise item holds the selected content. truncatedBy names
// which limit cut the content short, so the audit trail can say which one.
type fileInspection struct {
	decision    PacketDecision
	item        PacketItem
	truncatedBy string
}

// inspectSelectedFile classifies one candidate file. When countOnly is set it
// stops after the exclusion checks: that is enough to tell whether the file
// could have been included at all, without paying to hash a file that will not
// be in the packet.
func inspectSelectedFile(repo Repository, rel, reason string, remaining, maxFileBytes int, countOnly bool) (fileInspection, error) {
	excluded := func(reason, detail string) (fileInspection, error) {
		return fileInspection{decision: PacketDecision{Path: rel, Reason: reason, Detail: detail}}, nil
	}
	full := filepath.Join(repo.absoluteRoot, filepath.FromSlash(rel))
	info, err := os.Lstat(full)
	if err != nil {
		return excluded("unreadable", "cannot inspect file")
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return excluded("symlink", "")
	}
	if !info.Mode().IsRegular() {
		return excluded("non_regular", "")
	}
	if files.IsSensitivePath(rel) {
		return excluded("sensitive_path", "")
	}

	f, err := os.Open(full)
	if err != nil {
		return excluded("unreadable", "cannot open file")
	}
	defer f.Close()
	prefix, err := io.ReadAll(io.LimitReader(f, int64(maxFileBytes)+utf8.UTFMax))
	if err != nil {
		return fileInspection{}, fmt.Errorf("read %s: %w", rel, err)
	}
	content, binary := safeTextPrefix(prefix, info.Size(), maxFileBytes)
	if binary {
		return excluded("binary", "")
	}
	if containsSecret(content) {
		return excluded("secret_detected", "")
	}
	if countOnly {
		return fileInspection{}, nil
	}
	if remaining <= 0 {
		return excluded("total_limit", "")
	}
	truncatedBy := ""
	if int64(len(content)) < info.Size() {
		truncatedBy = "content_limit"
	}
	if len(content) > remaining {
		truncatedBy = "total_limit"
	}
	// A file that does not fit is worth more as declarations than as its first
	// N bytes. Only attempted when the content would be cut anyway: when the
	// whole file fits, verbatim source beats a summary of it.
	rendering := ""
	if truncatedBy != "" {
		skeleton, err := declarationSkeleton(rel, f, info.Size())
		if err != nil {
			return fileInspection{}, fmt.Errorf("read %s: %w", rel, err)
		}
		switch {
		case skeleton.secret:
			// Reading the whole file to parse it also scans the whole file, so
			// this catches secrets past the per-file prefix that the check
			// above could not see. Emitting anything from this file would break
			// the rule that the packet never carries unscanned bytes.
			return excluded("secret_detected", "")
		case skeleton.text != "":
			content = skeleton.text
			rendering = declarationRendering
		}
	}
	// The skeleton is built from the whole file, so unlike the prefix it is not
	// already inside the per-file budget. A caller who tightens --max-file-bytes
	// is bounding what they will be charged for, and a summary that ignores the
	// bound is still over the bound. Truncating declarations still beats
	// truncating source: the same bytes carry signatures instead of one body.
	if len(content) > maxFileBytes {
		content = truncateUTF8(content, maxFileBytes)
		truncatedBy = "content_limit"
	}
	if len(content) > remaining {
		content = truncateUTF8(content, remaining)
		truncatedBy = "total_limit"
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fileInspection{}, fmt.Errorf("hash %s: %w", rel, err)
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fileInspection{}, fmt.Errorf("hash %s: %w", rel, err)
	}
	return fileInspection{
		item: PacketItem{
			Kind:        packetItemKind(rel),
			Path:        rel,
			Content:     content,
			ContentHash: "sha256:" + hex.EncodeToString(h.Sum(nil)),
			Provenance: Provenance{
				Source: rel, Revision: repo.HeadRevision, Method: reason, Confidence: "detected",
			},
			Reason:        reason,
			Rendering:     rendering,
			OriginalBytes: info.Size(),
			IncludedBytes: len(content),
			Truncated:     int64(len(content)) < info.Size(),
		},
		truncatedBy: truncatedBy,
	}, nil
}

// declarationSkeleton reads f whole and renders its declarations, for the Go
// sources small enough to parse. It reports secret detection over the entire
// file rather than the prefix, because rendering declarations draws on bytes
// the prefix scan never saw.
//
// An empty text with no secret means "not applicable": not Go, too large, not
// valid UTF-8, or it did not parse. The caller falls back to byte truncation.
func declarationSkeleton(rel string, f *os.File, size int64) (struct {
	text   string
	secret bool
}, error) {
	var result struct {
		text   string
		secret bool
	}
	if !isGoSource(rel) || size > maxDeclarationParseBytes {
		return result, nil
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return result, err
	}
	src, err := io.ReadAll(io.LimitReader(f, maxDeclarationParseBytes))
	if err != nil {
		return result, err
	}
	if bytes.ContainsRune(src, 0) || !utf8.Valid(src) {
		return result, nil
	}
	if containsSecret(string(src)) {
		result.secret = true
		return result, nil
	}
	if rendered, ok := goDeclarations(src); ok {
		result.text = rendered
	}
	return result, nil
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

// runtimeStatePath reports generated or provider-owned state that is never
// repository context: Struktly's own exports and scans, and agent session
// directories. These are filtered before selection rather than recorded,
// because recording them would make a packet's contents depend on the packets
// generated before it.
func runtimeStatePath(rel string) bool {
	for _, prefix := range files.DefaultIgnoredPaths {
		if rel == prefix || strings.HasPrefix(rel, prefix+"/") {
			return true
		}
	}
	return false
}

// ignoredDirPath reports a path under a dependency, build output, or cache
// directory convention. Unlike runtimeStatePath these can hold tracked source,
// so an omission here is recorded rather than silent.
func ignoredDirPath(rel string) bool {
	for _, dir := range files.DefaultIgnoredDirs {
		for _, part := range strings.Split(rel, "/") {
			if part == dir {
				return true
			}
		}
	}
	return false
}

func defaultRuntimePath(rel string) bool {
	return runtimeStatePath(rel) || ignoredDirPath(rel)
}

func sortDecisions(values []PacketDecision) {
	sort.Slice(values, func(i, j int) bool {
		if values[i].Path == values[j].Path {
			return values[i].Reason < values[j].Reason
		}
		return values[i].Path < values[j].Path
	})
}

func ExplainSelection(ctx stdcontext.Context, requestedRoot, requestedPath, task, requestedScope string) (SelectionExplanation, error) {
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
	scope, err := cleanScope(repo.absoluteRoot, requestedScope)
	if err != nil {
		return SelectionExplanation{}, err
	}
	explanation := SelectionExplanation{Schema: ExplanationSchema, Path: rel, Decision: "excluded"}
	// Reported before every other reason: under a scope, "outside the requested
	// subtree" is the whole answer, and the rules that would have applied had it
	// been in scope are beside the point.
	if !withinScope(rel, scope) {
		explanation.Reason = "out_of_scope"
		explanation.Detail = "outside " + scope
		return explanation, nil
	}
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
	symbolDetail := ""
	if reason == "" {
		// One path, so this parses one file rather than building an index.
		// A selection nobody can justify is worse than a selection nobody
		// makes, so the matched declarations are named in the answer.
		match, ok := fileContentMatch(repo.absoluteRoot, rel, selectionTaskWords(task))
		if !ok || match.score == 0 {
			// Import expansion depends on what else the request selected, so it
			// cannot be answered from this path alone. Running the selection is
			// the cost of not reporting `not_selected` for a file the packet
			// would in fact contain.
			if provides, ok := explainImportNeighbor(ctx, repo.absoluteRoot, rel, task, requestedScope); ok {
				explanation.Decision = "included"
				explanation.Reason = "import_neighbor"
				explanation.Detail = "provides " + strings.Join(provides, ", ")
				return explanation, nil
			}
			explanation.Reason = "not_selected"
			return explanation, nil
		}
		reason = match.reason
		if reason == "title_match" {
			symbolDetail = "titled " + strings.Join(match.names, ", ")
		} else {
			symbolDetail = "declares " + strings.Join(match.names, ", ")
		}
	}
	inspection, err := inspectSelectedFile(repo, rel, reason, maxPacketTotalBytes, maxPacketFileBytes, false)
	if err != nil {
		return SelectionExplanation{}, err
	}
	if inspection.decision.Reason != "" {
		explanation.Reason = inspection.decision.Reason
		explanation.Detail = inspection.decision.Detail
		return explanation, nil
	}
	explanation.Decision = "included"
	explanation.Reason = reason
	explanation.Detail = symbolDetail
	return explanation, nil
}

// explainImportNeighbor reports whether the request would reach rel through
// something else it selected, and what rel supplies.
func explainImportNeighbor(ctx stdcontext.Context, root, rel, task, scope string) ([]string, bool) {
	selection, err := selectPacketContext(ctx, selectionRequest{
		root: root, task: task, scope: scope, limits: DefaultPacketLimits(),
	})
	if err != nil {
		return nil, false
	}
	for _, item := range selection.items {
		if item.Path != rel || item.Reason != "import_neighbor" {
			continue
		}
		return strings.Split(strings.TrimPrefix(item.Provenance.Location, "provides:"), ","), true
	}
	return nil, false
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
