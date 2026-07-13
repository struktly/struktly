package memory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMemoryCandidatesUseRuntimeStateAndApprovedMemoryStaysPortable(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	t.Setenv("STRUKTLY_STATE_DIR", stateDir)

	created, err := CreateMemoryCandidate(MemoryCandidateOptions{
		Root:    root,
		Content: "Run focused tests before committing.",
		Now:     time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if isWithin(created.Path, root) {
		t.Fatalf("candidate path is inside repository: %s", created.Path)
	}
	if !isWithin(created.Path, stateDir) {
		t.Fatalf("candidate path %q is outside state directory %q", created.Path, stateDir)
	}
	legacyPath := filepath.Join(root, ".struktly", "memory", "candidates", created.Candidate.ID+".json")
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy candidate should not exist: %v", err)
	}

	approved, err := ApproveMemoryCandidate(MemoryResolutionOptions{
		Root:        root,
		CandidateID: created.Candidate.ID,
		Now:         time.Date(2026, 7, 13, 12, 1, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	wantApproved := filepath.Join(root, ".struktly", "memory", "approved", created.Candidate.ID+".json")
	if approved.Path != wantApproved {
		t.Fatalf("approved path = %q, want %q", approved.Path, wantApproved)
	}
	if _, err := os.Stat(wantApproved); err != nil {
		t.Fatalf("approved memory is not portable: %v", err)
	}
}

func TestLegacyMemoryCandidateFallbackMigratesResolutionToRuntimeState(t *testing.T) {
	root := t.TempDir()
	stateDir := t.TempDir()
	t.Setenv("STRUKTLY_STATE_DIR", stateDir)
	createdAt := time.Date(2026, 7, 13, 13, 0, 0, 0, time.UTC)
	legacy := Record{
		ID:        "mem-legacy",
		Scope:     "repository",
		Content:   "Legacy candidate",
		Tags:      []string{},
		Status:    MemoryStatusPending,
		CreatedAt: createdAt,
	}
	legacyPath := filepath.Join(legacyMemoryCandidatesDir(root), legacy.ID+".json")
	if err := writeMemoryRecord(legacyPath, legacy); err != nil {
		t.Fatal(err)
	}

	listed, err := ListMemoryCandidates(ListMemoryOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(listed.Candidates) != 1 || listed.Candidates[0].ID != legacy.ID {
		t.Fatalf("legacy candidate not listed: %+v", listed.Candidates)
	}

	resolved, err := RejectMemoryCandidate(MemoryResolutionOptions{
		Root:        root,
		CandidateID: legacy.ID,
		Now:         createdAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !isWithin(resolved.Path, stateDir) || resolved.Candidate.Status != MemoryStatusRejected {
		t.Fatalf("legacy resolution was not migrated to runtime state: %+v", resolved)
	}
	unchanged, err := readMemoryRecord(legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Status != MemoryStatusPending {
		t.Fatalf("legacy candidate was mutated: %+v", unchanged)
	}
	if err := os.WriteFile(legacyPath, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	listed, err = ListMemoryCandidates(ListMemoryOptions{Root: root})
	if err != nil {
		t.Fatalf("shadowed legacy candidate was read: %v", err)
	}
	if len(listed.Candidates) != 1 || listed.Candidates[0].Status != MemoryStatusRejected {
		t.Fatalf("runtime candidate did not shadow legacy candidate: %+v", listed.Candidates)
	}
}

func TestMemoryCandidateIDRejectsUnsafePaths(t *testing.T) {
	root := t.TempDir()
	t.Setenv("STRUKTLY_STATE_DIR", t.TempDir())

	for _, id := range []string{".", "..", "../candidate", "sub/candidate", `sub\candidate`, "candidate.json"} {
		t.Run(id, func(t *testing.T) {
			_, err := RejectMemoryCandidate(MemoryResolutionOptions{Root: root, CandidateID: id})
			if err == nil {
				t.Fatalf("RejectMemoryCandidate(%q) succeeded", id)
			}
		})
	}
}

func isWithin(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
