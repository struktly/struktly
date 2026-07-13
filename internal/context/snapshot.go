package context

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/struktly/struktly/internal/files"
)

// SnapshotSchema identifies the structured scan snapshot format.
const SnapshotSchema = "struktly/snapshot/v1"

// Provenance records where a scanned fact came from and how it was derived.
type Provenance struct {
	Source     string `json:"source"`
	Revision   string `json:"revision,omitempty"`
	Location   string `json:"location,omitempty"`
	Method     string `json:"method"`
	Confidence string `json:"confidence"`
}

// SnapshotItem is one scanned fact with its supporting provenance.
type SnapshotItem struct {
	Value      string       `json:"value"`
	Kind       string       `json:"kind,omitempty"`
	Provenance []Provenance `json:"provenance,omitempty"`
}

// RepositoryIdent identifies the scanned repository.
type RepositoryIdent struct {
	Name     string `json:"name"`
	Root     string `json:"root"`
	Revision string `json:"revision,omitempty"`
	VCS      string `json:"vcs,omitempty"`
}

// DirectionExcerpt carries the product direction excerpt and its source.
type DirectionExcerpt struct {
	Excerpt    string     `json:"excerpt"`
	Provenance Provenance `json:"provenance"`
}

// SnapshotStats summarizes the scan itself.
type SnapshotStats struct {
	FilesScanned int   `json:"files_scanned"`
	DurationMS   int64 `json:"duration_ms"`
}

// Snapshot is the structured result of one repository scan.
type Snapshot struct {
	Schema           string            `json:"schema"`
	GeneratedAt      time.Time         `json:"generated_at"`
	Repository       RepositoryIdent   `json:"repository"`
	TopDirs          []SnapshotItem    `json:"top_dirs"`
	Languages        []SnapshotItem    `json:"languages"`
	Commands         []SnapshotItem    `json:"commands"`
	Docs             []SnapshotItem    `json:"docs"`
	ADRs             []SnapshotItem    `json:"adrs"`
	InstructionFiles []SnapshotItem    `json:"instruction_files"`
	Direction        *DirectionExcerpt `json:"direction,omitempty"`
	Ignored          []string          `json:"ignored,omitempty"`
	Deprioritized    []string          `json:"deprioritized,omitempty"`
	OpenQuestions    []string          `json:"open_questions,omitempty"`
	SourceRefs       []string          `json:"source_refs,omitempty"`
	Stats            SnapshotStats     `json:"stats"`
}

const maxProvenancePerItem = 3

// note records provenance for one fact, deduping identical entries and
// capping at maxProvenancePerItem distinct entries per item.
func (s *repositoryScan) note(kind, value string, p Provenance) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	key := kind + "\x00" + value
	existing := s.prov[key]
	for _, entry := range existing {
		if entry == p {
			return
		}
	}
	if len(existing) >= maxProvenancePerItem {
		return
	}
	s.prov[key] = append(existing, p)
}

// items builds SnapshotItems from a collected set, attaching recorded
// provenance and an optional per-value kind.
func (s *repositoryScan) items(kind string, set map[string]struct{}, kindOf func(string) string) []SnapshotItem {
	values := files.SortedStrings(set)
	out := make([]SnapshotItem, 0, len(values))
	for _, value := range values {
		item := SnapshotItem{Value: value, Provenance: s.prov[kind+"\x00"+value]}
		if kindOf != nil {
			item.Kind = kindOf(value)
		}
		out = append(out, item)
	}
	return out
}

// commandKind maps commandScore categories to snapshot command kinds.
func commandKind(command string) string {
	switch commandScore(command) / 2 {
	case 0:
		return "test"
	case 1:
		return "build"
	case 2:
		return "check"
	default:
		return "other"
	}
}

func (s *repositoryScan) snapshot(generatedAt time.Time, duration time.Duration) Snapshot {
	revision := gitRevision(s.root)
	vcs := ""
	if revision != "" {
		vcs = "git"
	}
	snap := Snapshot{
		Schema:      SnapshotSchema,
		GeneratedAt: generatedAt,
		Repository: RepositoryIdent{
			Name:     s.name,
			Root:     ".",
			Revision: revision,
			VCS:      vcs,
		},
		TopDirs:          s.items("top_dir", s.topDirs, nil),
		Languages:        s.items("language", s.languages, nil),
		Commands:         s.items("command", s.commands, commandKind),
		Docs:             s.items("doc", s.docs, nil),
		ADRs:             s.items("adr", s.adrs, nil),
		InstructionFiles: s.items("instruction_file", s.agentFiles, nil),
		Ignored:          files.SortedStrings(s.ignored),
		Deprioritized:    files.SortedStrings(s.stale),
		OpenQuestions:    append([]string(nil), s.openQuestions...),
		SourceRefs:       files.SortedStrings(s.sourceRefs),
		Stats: SnapshotStats{
			FilesScanned: s.filesScanned,
			DurationMS:   duration.Milliseconds(),
		},
	}
	if s.productDirection != "" {
		snap.Direction = &DirectionExcerpt{
			Excerpt: s.productDirection,
			Provenance: Provenance{
				Source:     s.productSourcePath,
				Location:   firstMarkdownHeading(s.productDirection),
				Method:     "user-file",
				Confidence: "detected",
			},
		}
	}
	return snap
}

func firstMarkdownHeading(text string) string {
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			return line
		}
	}
	return ""
}

// gitRevision resolves the repository HEAD commit by reading git metadata
// files directly; internal/context must stay exec-free. Returns "" when the
// revision cannot be determined.
func gitRevision(root string) string {
	gitPath := filepath.Join(root, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return ""
	}
	gitDir := gitPath
	if !info.IsDir() {
		data, err := os.ReadFile(gitPath)
		if err != nil {
			return ""
		}
		target, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir:")
		if !ok {
			return ""
		}
		target = strings.TrimSpace(target)
		if !filepath.IsAbs(target) {
			target = filepath.Join(root, target)
		}
		gitDir = target
	}

	head, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil {
		return ""
	}
	headLine := strings.TrimSpace(string(head))
	ref, ok := strings.CutPrefix(headLine, "ref:")
	if !ok {
		// Detached HEAD stores the commit hash directly.
		if isHexRevision(headLine) {
			return headLine
		}
		return ""
	}
	revision := resolveGitRef(gitDir, strings.TrimSpace(ref))
	if !isHexRevision(revision) {
		return ""
	}
	return revision
}

// resolveGitRef reads a loose ref under gitDir, falling back to the
// commondir (linked worktrees) and then packed-refs. Returns "" on miss.
func resolveGitRef(gitDir, ref string) string {
	if data, err := os.ReadFile(filepath.Join(gitDir, filepath.FromSlash(ref))); err == nil {
		return strings.TrimSpace(string(data))
	}
	refsDir := gitDir
	if data, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
		common := strings.TrimSpace(string(data))
		if !filepath.IsAbs(common) {
			common = filepath.Join(gitDir, common)
		}
		refsDir = common
		if data, err := os.ReadFile(filepath.Join(refsDir, filepath.FromSlash(ref))); err == nil {
			return strings.TrimSpace(string(data))
		}
	}
	data, err := os.ReadFile(filepath.Join(refsDir, "packed-refs"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "^") {
			continue
		}
		if strings.HasSuffix(line, " "+ref) {
			return strings.Fields(line)[0]
		}
	}
	return ""
}

func isHexRevision(value string) bool {
	if len(value) < 4 {
		return false
	}
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}
