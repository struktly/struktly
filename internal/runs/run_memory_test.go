package runs_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	repoctx "github.com/struktly/struktly/internal/context"
	"github.com/struktly/struktly/internal/memory"
	"github.com/struktly/struktly/internal/runs"
	"github.com/struktly/struktly/internal/state"
)

func TestRunLedgerCreateAppendListShowAndComplete(t *testing.T) {
	useStateDir(t)
	root := t.TempDir()
	now := time.Date(2026, 7, 7, 9, 30, 0, 0, time.UTC)

	created, err := runs.CreateRun(runs.CreateRunOptions{
		Root:          root,
		Goal:          "Prepare task spec",
		RepoPath:      root,
		SourceCommand: "struktly run create --goal Prepare task spec",
		Now:           now,
	})
	if err != nil {
		t.Fatalf("runs.CreateRun returned error: %v", err)
	}
	if created.Run.ID != "run-20260707-093000-prepare-task-spec" {
		t.Fatalf("unexpected run id: %s", created.Run.ID)
	}
	if created.Run.Status != "created" {
		t.Fatalf("unexpected status: %s", created.Run.Status)
	}

	event, err := runs.AppendRunEvent(runs.RunEventOptions{
		Root:    root,
		RunID:   created.Run.ID,
		Type:    "agent_message",
		Message: "Prepared initial handoff.",
		Now:     now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("runs.AppendRunEvent returned error: %v", err)
	}
	if event.Event.Type != "agent_message" {
		t.Fatalf("unexpected event type: %s", event.Event.Type)
	}

	listed, err := runs.ListRuns(runs.ListRunsOptions{Root: root})
	if err != nil {
		t.Fatalf("runs.ListRuns returned error: %v", err)
	}
	if len(listed.Runs) != 1 || listed.Runs[0].ID != created.Run.ID {
		t.Fatalf("unexpected runs: %+v", listed.Runs)
	}

	shown, err := runs.ShowRun(runs.ShowRunOptions{Root: root, RunID: created.Run.ID})
	if err != nil {
		t.Fatalf("runs.ShowRun returned error: %v", err)
	}
	if len(shown.Events) != 2 {
		t.Fatalf("expected create + appended event, got %d", len(shown.Events))
	}

	completed, err := runs.CompleteRun(runs.UpdateRunStatusOptions{
		Root:  root,
		RunID: created.Run.ID,
		Now:   now.Add(2 * time.Minute),
	})
	if err != nil {
		t.Fatalf("runs.CompleteRun returned error: %v", err)
	}
	if completed.Run.Status != "completed" {
		t.Fatalf("unexpected completed status: %s", completed.Run.Status)
	}

	wantRunPath, err := state.Path(root, "runs", created.Run.ID, "run.json")
	if err != nil {
		t.Fatalf("state.Path returned error: %v", err)
	}
	if created.RunPath != wantRunPath {
		t.Fatalf("unexpected run path: got %s, want %s", created.RunPath, wantRunPath)
	}
	wantEventsPath, err := state.Path(root, "runs", created.Run.ID, "events.jsonl")
	if err != nil {
		t.Fatalf("state.Path returned error: %v", err)
	}
	if created.EventsPath != wantEventsPath || event.EventPath != wantEventsPath {
		t.Fatalf("unexpected events paths: create %s, append %s, want %s", created.EventsPath, event.EventPath, wantEventsPath)
	}
	runData, err := os.ReadFile(wantRunPath)
	if err != nil {
		t.Fatalf("read run.json: %v", err)
	}
	var persisted runs.RunRecord
	if err := json.Unmarshal(runData, &persisted); err != nil {
		t.Fatalf("decode run.json: %v", err)
	}
	if persisted.Goal != "Prepare task spec" {
		t.Fatalf("unexpected persisted run: %+v", persisted)
	}
	if _, err := os.Stat(filepath.Join(root, ".struktly", "runs")); !os.IsNotExist(err) {
		t.Fatalf("repository-local runs directory exists: %v", err)
	}
}

func TestScanAndBriefAttachArtifactsToRun(t *testing.T) {
	useStateDir(t)
	root := t.TempDir()
	writeFile(t, root, "README.md", "# Repo\n")
	writeFile(t, root, "go.mod", "module example.com/repo\n\ngo 1.24.0\n")
	initGitRepository(t, root)

	run, err := runs.CreateRun(runs.CreateRunOptions{
		Root: root,
		Goal: "Attach scan and brief",
		Now:  time.Date(2026, 7, 7, 10, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("runs.CreateRun returned error: %v", err)
	}

	scan, err := repoctx.Scan(repoctx.ScanOptions{Root: root, RunID: run.Run.ID})
	if err != nil {
		t.Fatalf("repoctx.Scan returned error: %v", err)
	}
	brief, err := repoctx.Brief(repoctx.BriefOptions{
		Root:  root,
		Task:  "Attach context packet",
		RunID: run.Run.ID,
		Now:   time.Date(2026, 7, 7, 10, 1, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("repoctx.Brief returned error: %v", err)
	}

	shown, err := runs.ShowRun(runs.ShowRunOptions{Root: root, RunID: run.Run.ID})
	if err != nil {
		t.Fatalf("runs.ShowRun returned error: %v", err)
	}
	wantArtifacts := map[string]string{
		"scan":  ".struktly/project-context.md",
		"brief": relForTest(root, brief.OutputPath),
	}
	for _, artifact := range shown.Run.Artifacts {
		delete(wantArtifacts, artifact.Type)
	}
	if len(wantArtifacts) != 0 {
		t.Fatalf("missing artifacts after scan %s brief %s: %+v", scan.OutputPath, brief.OutputPath, wantArtifacts)
	}
}

func TestMemoryCandidateApproveRejectAndApprovedMemoryInBrief(t *testing.T) {
	useStateDir(t)
	root := t.TempDir()
	writeFile(t, root, "README.md", "# Repo\n")
	writeFile(t, root, "go.mod", "module example.com/repo\n\ngo 1.24.0\n")
	initGitRepository(t, root)
	if _, err := repoctx.Scan(repoctx.ScanOptions{Root: root}); err != nil {
		t.Fatalf("repoctx.Scan returned error: %v", err)
	}

	run, err := runs.CreateRun(runs.CreateRunOptions{
		Root: root,
		Goal: "Capture durable memory",
		Now:  time.Date(2026, 7, 7, 11, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("runs.CreateRun returned error: %v", err)
	}

	candidate, err := memory.CreateMemoryCandidate(memory.MemoryCandidateOptions{
		Root:           root,
		Scope:          "repository",
		Content:        "Always run go test ./... before completing CLI changes.",
		Tags:           []string{"go", "verification"},
		SourceRunID:    run.Run.ID,
		SourceArtifact: ".struktly/project-context.md",
		Now:            time.Date(2026, 7, 7, 11, 1, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("memory.CreateMemoryCandidate returned error: %v", err)
	}
	if candidate.Candidate.Status != "pending" {
		t.Fatalf("unexpected status: %s", candidate.Candidate.Status)
	}

	approved, err := memory.ApproveMemoryCandidate(memory.MemoryResolutionOptions{
		Root:        root,
		CandidateID: candidate.Candidate.ID,
		Now:         time.Date(2026, 7, 7, 11, 2, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("memory.ApproveMemoryCandidate returned error: %v", err)
	}
	if approved.Memory.Status != "approved" {
		t.Fatalf("unexpected approved status: %s", approved.Memory.Status)
	}

	rejectedCandidate, err := memory.CreateMemoryCandidate(memory.MemoryCandidateOptions{
		Root:        root,
		Scope:       "project",
		Content:     "Discard this suggestion.",
		SourceRunID: run.Run.ID,
		Now:         time.Date(2026, 7, 7, 11, 3, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("memory.CreateMemoryCandidate for reject returned error: %v", err)
	}
	rejected, err := memory.RejectMemoryCandidate(memory.MemoryResolutionOptions{
		Root:        root,
		CandidateID: rejectedCandidate.Candidate.ID,
		Now:         time.Date(2026, 7, 7, 11, 4, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("memory.RejectMemoryCandidate returned error: %v", err)
	}
	if rejected.Candidate.Status != "rejected" {
		t.Fatalf("unexpected rejected status: %s", rejected.Candidate.Status)
	}

	memories, err := memory.ListApprovedMemory(memory.ListMemoryOptions{Root: root})
	if err != nil {
		t.Fatalf("memory.ListApprovedMemory returned error: %v", err)
	}
	if len(memories.Items) != 1 {
		t.Fatalf("expected one approved memory, got %+v", memories.Items)
	}

	brief, err := repoctx.Brief(repoctx.BriefOptions{
		Root: root,
		Task: "Use approved memory",
		Now:  time.Date(2026, 7, 7, 11, 5, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("repoctx.Brief returned error: %v", err)
	}
	data, err := os.ReadFile(brief.OutputPath)
	if err != nil {
		t.Fatalf("read brief: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "## Approved Memory") {
		t.Fatalf("expected approved memory section:\n%s", content)
	}
	if !strings.Contains(content, "Always run go test ./...") {
		t.Fatalf("expected approved memory content:\n%s", content)
	}
	if !strings.Contains(content, run.Run.ID) {
		t.Fatalf("expected source run provenance:\n%s", content)
	}
}

func TestListAndShowFallBackToLegacyRuns(t *testing.T) {
	useStateDir(t)
	root := t.TempDir()
	runID := "run-legacy"
	createdAt := time.Date(2026, 7, 7, 8, 0, 0, 0, time.UTC)
	legacy := runs.RunRecord{
		ID:        runID,
		Goal:      "Legacy goal",
		RepoPath:  root,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
		Status:    runs.RunStatusCreated,
		Artifacts: []runs.RunArtifact{},
	}
	legacyData, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("encode legacy run: %v", err)
	}
	writeFile(t, root, ".struktly/runs/"+runID+"/run.json", string(legacyData)+"\n")
	event := runs.RunEvent{ID: "evt-legacy", Type: "run_created", Message: "Legacy.", CreatedAt: createdAt}
	eventData, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("encode legacy event: %v", err)
	}
	writeFile(t, root, ".struktly/runs/"+runID+"/events.jsonl", string(eventData)+"\n")

	listed, err := runs.ListRuns(runs.ListRunsOptions{Root: root})
	if err != nil {
		t.Fatalf("runs.ListRuns returned error: %v", err)
	}
	if len(listed.Runs) != 1 || listed.Runs[0].Goal != legacy.Goal {
		t.Fatalf("unexpected legacy runs: %+v", listed.Runs)
	}
	shown, err := runs.ShowRun(runs.ShowRunOptions{Root: root, RunID: runID})
	if err != nil {
		t.Fatalf("runs.ShowRun returned error: %v", err)
	}
	if shown.Run.Goal != legacy.Goal || len(shown.Events) != 1 {
		t.Fatalf("unexpected legacy run: %+v", shown)
	}
	if _, err := runs.AppendRunEvent(runs.RunEventOptions{Root: root, RunID: runID, Type: "note", Message: "No migration"}); err == nil {
		t.Fatal("AppendRunEvent unexpectedly mutated a legacy run")
	}

	current := legacy
	current.Goal = "Current goal"
	currentData, err := json.Marshal(current)
	if err != nil {
		t.Fatalf("encode current run: %v", err)
	}
	currentPath, err := state.Path(root, "runs", runID, "run.json")
	if err != nil {
		t.Fatalf("state.Path returned error: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(currentPath), 0o755); err != nil {
		t.Fatalf("create current run directory: %v", err)
	}
	if err := os.WriteFile(currentPath, append(currentData, '\n'), 0o644); err != nil {
		t.Fatalf("write current run: %v", err)
	}

	shown, err = runs.ShowRun(runs.ShowRunOptions{Root: root, RunID: runID})
	if err != nil {
		t.Fatalf("runs.ShowRun current returned error: %v", err)
	}
	if shown.Run.Goal != current.Goal || len(shown.Events) != 0 {
		t.Fatalf("new-state run did not take precedence: %+v", shown)
	}
	listed, err = runs.ListRuns(runs.ListRunsOptions{Root: root})
	if err != nil {
		t.Fatalf("runs.ListRuns current returned error: %v", err)
	}
	if len(listed.Runs) != 1 || listed.Runs[0].Goal != current.Goal {
		t.Fatalf("new-state list did not take precedence: %+v", listed.Runs)
	}
}

func TestRunIDRejectsUnsafePathComponents(t *testing.T) {
	useStateDir(t)
	root := t.TempDir()
	for _, runID := range []string{".", "..", "../escape", "sub/run", `sub\\run`, "unsafe id"} {
		t.Run(runID, func(t *testing.T) {
			_, err := runs.ShowRun(runs.ShowRunOptions{Root: root, RunID: runID})
			if err == nil || !strings.Contains(err.Error(), "invalid run id") {
				t.Fatalf("ShowRun error = %v, want invalid run id", err)
			}
		})
	}
}

func relForTest(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}
