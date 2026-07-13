package memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/struktly/struktly/internal/files"
	"github.com/struktly/struktly/internal/state"
)

const (
	MemoryStatusPending  = "pending"
	MemoryStatusApproved = "approved"
	MemoryStatusRejected = "rejected"
)

type MemoryCandidateOptions struct {
	Root           string
	Scope          string
	Content        string
	Tags           []string
	SourceRunID    string
	SourceArtifact string
	Now            time.Time
}

type MemoryResolutionOptions struct {
	Root        string
	CandidateID string
	Now         time.Time
}

type ListMemoryOptions struct {
	Root string
}

type MemoryCandidateResult struct {
	Candidate Record `json:"candidate"`
	Path      string `json:"path"`
}

type MemoryResolutionResult struct {
	Candidate Record `json:"candidate"`
	Memory    Record `json:"memory,omitempty"`
	Path      string `json:"path"`
}

type ListMemoryCandidatesResult struct {
	Candidates []Record `json:"candidates"`
}

type ListApprovedMemoryResult struct {
	Items []Record `json:"items"`
}

type Record struct {
	ID             string     `json:"id"`
	Scope          string     `json:"scope"`
	Content        string     `json:"content"`
	Tags           []string   `json:"tags"`
	SourceRunID    string     `json:"source_run_id,omitempty"`
	SourceArtifact string     `json:"source_artifact,omitempty"`
	Status         string     `json:"status"`
	CreatedAt      time.Time  `json:"created_at"`
	ResolvedAt     *time.Time `json:"resolved_at,omitempty"`
}

func CreateMemoryCandidate(opts MemoryCandidateOptions) (MemoryCandidateResult, error) {
	root, err := files.CleanRoot(opts.Root)
	if err != nil {
		return MemoryCandidateResult{}, err
	}
	content := strings.TrimSpace(opts.Content)
	if content == "" {
		return MemoryCandidateResult{}, fmt.Errorf("memory content is required")
	}
	scope := normalizeMemoryScope(opts.Scope)
	now := files.NormalizeNow(opts.Now)
	id, err := uniqueMemoryID(root, now, content)
	if err != nil {
		return MemoryCandidateResult{}, err
	}
	candidate := Record{
		ID:             id,
		Scope:          scope,
		Content:        content,
		Tags:           normalizeTags(opts.Tags),
		SourceRunID:    strings.TrimSpace(opts.SourceRunID),
		SourceArtifact: normalizeMemoryArtifactPath(root, opts.SourceArtifact),
		Status:         MemoryStatusPending,
		CreatedAt:      now,
	}
	path, err := memoryCandidatePath(root, candidate.ID)
	if err != nil {
		return MemoryCandidateResult{}, err
	}
	if err := writeMemoryRecord(path, candidate); err != nil {
		return MemoryCandidateResult{}, err
	}
	return MemoryCandidateResult{Candidate: candidate, Path: path}, nil
}

func ListMemoryCandidates(opts ListMemoryOptions) (ListMemoryCandidatesResult, error) {
	root, err := files.CleanRoot(opts.Root)
	if err != nil {
		return ListMemoryCandidatesResult{}, err
	}
	dir, err := state.Path(root, "memory", "candidates")
	if err != nil {
		return ListMemoryCandidatesResult{}, err
	}
	items, err := readMemoryRecords(dir, nil)
	if err != nil {
		return ListMemoryCandidatesResult{}, err
	}
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		seen[item.ID] = struct{}{}
	}
	legacy, err := readMemoryRecords(legacyMemoryCandidatesDir(root), seen)
	if err != nil {
		return ListMemoryCandidatesResult{}, err
	}
	items = append(items, legacy...)
	sortMemoryRecords(items)
	return ListMemoryCandidatesResult{Candidates: items}, nil
}

func ListApprovedMemory(opts ListMemoryOptions) (ListApprovedMemoryResult, error) {
	root, err := files.CleanRoot(opts.Root)
	if err != nil {
		return ListApprovedMemoryResult{}, err
	}
	items, err := readMemoryRecords(filepath.Join(root, ".struktly", "memory", "approved"), nil)
	if err != nil {
		return ListApprovedMemoryResult{}, err
	}
	return ListApprovedMemoryResult{Items: items}, nil
}

func ApproveMemoryCandidate(opts MemoryResolutionOptions) (MemoryResolutionResult, error) {
	return resolveMemoryCandidate(opts, MemoryStatusApproved)
}

func RejectMemoryCandidate(opts MemoryResolutionOptions) (MemoryResolutionResult, error) {
	return resolveMemoryCandidate(opts, MemoryStatusRejected)
}

func resolveMemoryCandidate(opts MemoryResolutionOptions, status string) (MemoryResolutionResult, error) {
	root, err := files.CleanRoot(opts.Root)
	if err != nil {
		return MemoryResolutionResult{}, err
	}
	candidateID := strings.TrimSpace(opts.CandidateID)
	if candidateID == "" {
		return MemoryResolutionResult{}, fmt.Errorf("candidate id is required")
	}
	if !validMemoryCandidateID(candidateID) {
		return MemoryResolutionResult{}, fmt.Errorf("invalid candidate id %q", candidateID)
	}
	path, err := memoryCandidatePath(root, candidateID)
	if err != nil {
		return MemoryResolutionResult{}, err
	}
	candidate, err := readMemoryRecord(path)
	if errors.Is(err, os.ErrNotExist) {
		candidate, err = readMemoryRecord(filepath.Join(legacyMemoryCandidatesDir(root), candidateID+".json"))
	}
	if err != nil {
		return MemoryResolutionResult{}, err
	}
	if candidate.Status != MemoryStatusPending {
		return MemoryResolutionResult{}, fmt.Errorf("memory candidate %s is already %s", candidate.ID, candidate.Status)
	}

	now := files.NormalizeNow(opts.Now)
	candidate.Status = status
	candidate.ResolvedAt = &now
	if err := writeMemoryRecord(path, candidate); err != nil {
		return MemoryResolutionResult{}, err
	}

	result := MemoryResolutionResult{Candidate: candidate, Path: path}
	if status == MemoryStatusApproved {
		approvedPath := approvedMemoryPath(root, candidate.ID)
		if err := writeMemoryRecord(approvedPath, candidate); err != nil {
			return MemoryResolutionResult{}, err
		}
		result.Memory = candidate
		result.Path = approvedPath
	}
	return result, nil
}

func ReadApprovedForBrief(root string, limit int) ([]Record, error) {
	result, err := ListApprovedMemory(ListMemoryOptions{Root: root})
	if err != nil {
		return nil, err
	}
	if limit > 0 && len(result.Items) > limit {
		return result.Items[:limit], nil
	}
	return result.Items, nil
}

func readMemoryRecords(dir string, skip map[string]struct{}) ([]Record, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Record{}, nil
		}
		return nil, fmt.Errorf("read memory dir: %w", err)
	}
	items := make([]Record, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		if _, ok := skip[strings.TrimSuffix(entry.Name(), ".json")]; ok {
			continue
		}
		item, err := readMemoryRecord(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	sortMemoryRecords(items)
	return items, nil
}

func sortMemoryRecords(items []Record) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].CreatedAt.Equal(items[j].CreatedAt) {
			return items[i].ID < items[j].ID
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
}

func readMemoryRecord(path string) (Record, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Record{}, fmt.Errorf("read memory record: %w", err)
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, fmt.Errorf("decode memory record: %w", err)
	}
	if record.Tags == nil {
		record.Tags = []string{}
	}
	return record, nil
}

func writeMemoryRecord(path string, record Record) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create memory dir: %w", err)
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("encode memory record: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write memory record: %w", err)
	}
	return nil
}

func normalizeMemoryScope(scope string) string {
	scope = strings.ToLower(strings.TrimSpace(scope))
	switch scope {
	case "", "repo":
		return "repository"
	case "global", "project", "repository":
		return scope
	default:
		return "repository"
	}
}

func normalizeTags(values []string) []string {
	seen := map[string]struct{}{}
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(strings.ToLower(part))
			if part == "" {
				continue
			}
			seen[part] = struct{}{}
		}
	}
	return files.SortedStrings(seen)
}

func normalizeMemoryArtifactPath(root, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(path)
}

func uniqueMemoryID(root string, now time.Time, content string) (string, error) {
	base := "mem-" + now.Format("20060102-150405") + "-" + files.Slugify(content, 48)
	id := base
	for i := 2; ; i++ {
		candidatePath, err := memoryCandidatePath(root, id)
		if err != nil {
			return "", err
		}
		legacyPath := filepath.Join(legacyMemoryCandidatesDir(root), id+".json")
		if !files.FileExists(candidatePath) && !files.FileExists(legacyPath) && !files.FileExists(approvedMemoryPath(root, id)) {
			return id, nil
		}
		id = fmt.Sprintf("%s-%d", base, i)
	}
}

func memoryCandidatePath(root, id string) (string, error) {
	return state.Path(root, "memory", "candidates", id+".json")
}

func legacyMemoryCandidatesDir(root string) string {
	return filepath.Join(root, ".struktly", "memory", "candidates")
}

func approvedMemoryPath(root, id string) string {
	return filepath.Join(root, ".struktly", "memory", "approved", id+".json")
}

func validMemoryCandidateID(id string) bool {
	if id == "." || id == ".." || strings.ContainsAny(id, "/\\") {
		return false
	}
	for _, r := range id {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return id != ""
}
