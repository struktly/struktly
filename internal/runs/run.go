package runs

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/struktly/struktly/internal/files"
	"github.com/struktly/struktly/internal/state"
)

const (
	RunStatusCreated   = "created"
	RunStatusCompleted = "completed"
	RunStatusFailed    = "failed"
)

type CreateRunOptions struct {
	Root          string
	Goal          string
	RepoPath      string
	SourceCommand string
	Now           time.Time
}

type RunResult struct {
	Run        RunRecord
	RunPath    string
	Events     []RunEvent
	EventsPath string
}

type RunRecord struct {
	ID            string        `json:"id"`
	Goal          string        `json:"goal"`
	RepoPath      string        `json:"repo_path"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
	Status        string        `json:"status"`
	SourceCommand string        `json:"source_command,omitempty"`
	Artifacts     []RunArtifact `json:"artifacts"`
}

type RunArtifact struct {
	Type      string    `json:"type"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
}

type RunEvent struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
}

type RunEventOptions struct {
	Root    string
	RunID   string
	Type    string
	Message string
	Now     time.Time
}

type RunEventResult struct {
	Run       RunRecord
	Event     RunEvent
	EventPath string
}

type ListRunsOptions struct {
	Root string
}

type ListRunsResult struct {
	Runs []RunRecord `json:"runs"`
}

type ShowRunOptions struct {
	Root  string
	RunID string
}

type ShowRunResult struct {
	Run    RunRecord  `json:"run"`
	Events []RunEvent `json:"events"`
}

type UpdateRunStatusOptions struct {
	Root  string
	RunID string
	Now   time.Time
}

type AttachRunArtifactOptions struct {
	Root         string
	RunID        string
	ArtifactType string
	Path         string
	Message      string
	Now          time.Time
}

func CreateRun(opts CreateRunOptions) (RunResult, error) {
	root, err := files.CleanRoot(opts.Root)
	if err != nil {
		return RunResult{}, err
	}
	goal := strings.TrimSpace(opts.Goal)
	if goal == "" {
		return RunResult{}, fmt.Errorf("goal is required")
	}

	now := files.NormalizeNow(opts.Now)
	repoPath := strings.TrimSpace(opts.RepoPath)
	if repoPath == "" {
		repoPath = root
	}
	if !filepath.IsAbs(repoPath) {
		repoPath = filepath.Join(root, repoPath)
	}
	repoPath = filepath.ToSlash(repoPath)

	id, err := uniqueRunID(root, now, goal)
	if err != nil {
		return RunResult{}, err
	}
	run := RunRecord{
		ID:            id,
		Goal:          goal,
		RepoPath:      repoPath,
		CreatedAt:     now,
		UpdatedAt:     now,
		Status:        RunStatusCreated,
		SourceCommand: strings.TrimSpace(opts.SourceCommand),
		Artifacts:     []RunArtifact{},
	}
	if err := writeRun(root, run); err != nil {
		return RunResult{}, err
	}
	event, err := appendEvent(root, id, RunEvent{
		ID:        eventID(now, "run-created"),
		Type:      "run_created",
		Message:   "Run created.",
		CreatedAt: now,
	})
	if err != nil {
		return RunResult{}, err
	}
	runPath, err := runJSONPath(root, id)
	if err != nil {
		return RunResult{}, err
	}
	eventPath, err := eventsPath(root, id)
	if err != nil {
		return RunResult{}, err
	}
	return RunResult{
		Run:        run,
		RunPath:    runPath,
		Events:     []RunEvent{event},
		EventsPath: eventPath,
	}, nil
}

func AppendRunEvent(opts RunEventOptions) (RunEventResult, error) {
	root, run, err := loadRunForUpdate(opts.Root, opts.RunID)
	if err != nil {
		return RunEventResult{}, err
	}
	eventType := strings.TrimSpace(opts.Type)
	if eventType == "" {
		return RunEventResult{}, fmt.Errorf("event type is required")
	}
	message := strings.TrimSpace(opts.Message)
	if message == "" {
		return RunEventResult{}, fmt.Errorf("event message is required")
	}
	now := files.NormalizeNow(opts.Now)
	event, err := appendEvent(root, run.ID, RunEvent{
		ID:        eventID(now, eventType),
		Type:      eventType,
		Message:   message,
		CreatedAt: now,
	})
	if err != nil {
		return RunEventResult{}, err
	}
	run.UpdatedAt = now
	if err := writeRun(root, run); err != nil {
		return RunEventResult{}, err
	}
	eventPath, err := eventsPath(root, run.ID)
	if err != nil {
		return RunEventResult{}, err
	}
	return RunEventResult{Run: run, Event: event, EventPath: eventPath}, nil
}

func ListRuns(opts ListRunsOptions) (ListRunsResult, error) {
	root, err := files.CleanRoot(opts.Root)
	if err != nil {
		return ListRunsResult{}, err
	}
	runsDir, err := state.Path(root, "runs")
	if err != nil {
		return ListRunsResult{}, err
	}
	runsByID := map[string]RunRecord{}
	if err := collectRuns(root, runsDir, false, runsByID); err != nil {
		return ListRunsResult{}, err
	}
	legacyDir := filepath.Join(root, ".struktly", "runs")
	if err := collectRuns(root, legacyDir, true, runsByID); err != nil {
		return ListRunsResult{}, err
	}

	runs := make([]RunRecord, 0, len(runsByID))
	for _, run := range runsByID {
		runs = append(runs, run)
	}
	sort.SliceStable(runs, func(i, j int) bool {
		if !runs[i].CreatedAt.Equal(runs[j].CreatedAt) {
			return runs[i].CreatedAt.Before(runs[j].CreatedAt)
		}
		return runs[i].ID < runs[j].ID
	})
	return ListRunsResult{Runs: runs}, nil
}

func ShowRun(opts ShowRunOptions) (ShowRunResult, error) {
	root, err := files.CleanRoot(opts.Root)
	if err != nil {
		return ShowRunResult{}, err
	}
	run, err := readRun(root, opts.RunID)
	legacy := false
	if errors.Is(err, os.ErrNotExist) {
		run, err = readLegacyRun(root, opts.RunID)
		legacy = true
	}
	if err != nil {
		return ShowRunResult{}, err
	}
	events, err := readEventsFrom(root, run.ID, legacy)
	if err != nil {
		return ShowRunResult{}, err
	}
	return ShowRunResult{Run: run, Events: events}, nil
}

func CompleteRun(opts UpdateRunStatusOptions) (RunResult, error) {
	return updateRunStatus(opts, RunStatusCompleted, "run_completed", "Run completed.")
}

func FailRun(opts UpdateRunStatusOptions) (RunResult, error) {
	return updateRunStatus(opts, RunStatusFailed, "run_failed", "Run failed.")
}

func AttachRunArtifact(opts AttachRunArtifactOptions) (RunResult, error) {
	root, run, err := loadRunForUpdate(opts.Root, opts.RunID)
	if err != nil {
		return RunResult{}, err
	}
	artifactType := strings.TrimSpace(opts.ArtifactType)
	if artifactType == "" {
		return RunResult{}, fmt.Errorf("artifact type is required")
	}
	path := strings.TrimSpace(opts.Path)
	if path == "" {
		return RunResult{}, fmt.Errorf("artifact path is required")
	}
	if filepath.IsAbs(path) {
		if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
			path = rel
		}
	}
	path = filepath.ToSlash(path)

	now := files.NormalizeNow(opts.Now)
	artifact := RunArtifact{Type: artifactType, Path: path, CreatedAt: now}
	replaced := false
	for i, existing := range run.Artifacts {
		if existing.Type == artifact.Type && existing.Path == artifact.Path {
			run.Artifacts[i] = artifact
			replaced = true
			break
		}
	}
	if !replaced {
		run.Artifacts = append(run.Artifacts, artifact)
	}
	sort.SliceStable(run.Artifacts, func(i, j int) bool {
		if run.Artifacts[i].Type != run.Artifacts[j].Type {
			return run.Artifacts[i].Type < run.Artifacts[j].Type
		}
		return run.Artifacts[i].Path < run.Artifacts[j].Path
	})
	run.UpdatedAt = now
	if err := writeRun(root, run); err != nil {
		return RunResult{}, err
	}

	message := strings.TrimSpace(opts.Message)
	if message == "" {
		message = fmt.Sprintf("Attached %s artifact `%s`.", artifact.Type, artifact.Path)
	}
	event, err := appendEvent(root, run.ID, RunEvent{
		ID:        eventID(now, "artifact-written"),
		Type:      "artifact_written",
		Message:   message,
		CreatedAt: now,
	})
	if err != nil {
		return RunResult{}, err
	}
	return runResult(root, run, event)
}

func updateRunStatus(opts UpdateRunStatusOptions, status, eventType, message string) (RunResult, error) {
	root, run, err := loadRunForUpdate(opts.Root, opts.RunID)
	if err != nil {
		return RunResult{}, err
	}
	now := files.NormalizeNow(opts.Now)
	run.Status = status
	run.UpdatedAt = now
	if err := writeRun(root, run); err != nil {
		return RunResult{}, err
	}
	event, err := appendEvent(root, run.ID, RunEvent{
		ID:        eventID(now, eventType),
		Type:      eventType,
		Message:   message,
		CreatedAt: now,
	})
	if err != nil {
		return RunResult{}, err
	}
	return runResult(root, run, event)
}

func runResult(root string, run RunRecord, event RunEvent) (RunResult, error) {
	runPath, err := runJSONPath(root, run.ID)
	if err != nil {
		return RunResult{}, err
	}
	eventPath, err := eventsPath(root, run.ID)
	if err != nil {
		return RunResult{}, err
	}
	return RunResult{Run: run, RunPath: runPath, Events: []RunEvent{event}, EventsPath: eventPath}, nil
}

func loadRunForUpdate(rootOpt, runID string) (string, RunRecord, error) {
	root, err := files.CleanRoot(rootOpt)
	if err != nil {
		return "", RunRecord{}, err
	}
	run, err := readRun(root, runID)
	if err != nil {
		return "", RunRecord{}, err
	}
	return root, run, nil
}

func readRun(root, runID string) (RunRecord, error) {
	return readRunFrom(root, runID, false)
}

func readLegacyRun(root, runID string) (RunRecord, error) {
	return readRunFrom(root, runID, true)
}

func readRunFrom(root, runID string, legacy bool) (RunRecord, error) {
	runID, err := validateRunID(runID)
	if err != nil {
		return RunRecord{}, err
	}
	path, err := runJSONPath(root, runID)
	if legacy {
		path, err = legacyRunJSONPath(root, runID)
	}
	if err != nil {
		return RunRecord{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return RunRecord{}, fmt.Errorf("read run %s: %w", runID, err)
	}
	var run RunRecord
	if err := json.Unmarshal(data, &run); err != nil {
		return RunRecord{}, fmt.Errorf("decode run %s: %w", runID, err)
	}
	if run.Artifacts == nil {
		run.Artifacts = []RunArtifact{}
	}
	return run, nil
}

func writeRun(root string, run RunRecord) error {
	path, err := runJSONPath(root, run.ID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create run dir: %w", err)
	}
	data, err := json.MarshalIndent(run, "", "  ")
	if err != nil {
		return fmt.Errorf("encode run: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write run: %w", err)
	}
	return nil
}

func appendEvent(root, runID string, event RunEvent) (RunEvent, error) {
	path, err := eventsPath(root, runID)
	if err != nil {
		return RunEvent{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return RunEvent{}, fmt.Errorf("create events dir: %w", err)
	}
	data, err := json.Marshal(event)
	if err != nil {
		return RunEvent{}, fmt.Errorf("encode event: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return RunEvent{}, fmt.Errorf("open events: %w", err)
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return RunEvent{}, fmt.Errorf("write event: %w", err)
	}
	return event, nil
}

func readEventsFrom(root, runID string, legacy bool) ([]RunEvent, error) {
	path, err := eventsPath(root, runID)
	if legacy {
		path, err = legacyEventsPath(root, runID)
	}
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []RunEvent{}, nil
		}
		return nil, fmt.Errorf("read events: %w", err)
	}
	defer f.Close()

	var events []RunEvent
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event RunEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return nil, fmt.Errorf("decode event: %w", err)
		}
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan events: %w", err)
	}
	if events == nil {
		events = []RunEvent{}
	}
	return events, nil
}

func uniqueRunID(root string, now time.Time, goal string) (string, error) {
	base := "run-" + now.Format("20060102-150405") + "-" + files.Slugify(goal, 48)
	id := base
	for i := 2; ; i++ {
		path, err := runJSONPath(root, id)
		if err != nil {
			return "", err
		}
		legacyPath, err := legacyRunJSONPath(root, id)
		if err != nil {
			return "", err
		}
		if !files.FileExists(path) && !files.FileExists(legacyPath) {
			return id, nil
		}
		id = fmt.Sprintf("%s-%d", base, i)
	}
}

func eventID(now time.Time, eventType string) string {
	return "evt-" + now.Format("20060102-150405") + "-" + files.Slugify(eventType, 40)
}

func collectRuns(root, dir string, legacy bool, runsByID map[string]RunRecord) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read runs dir: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, exists := runsByID[entry.Name()]; exists {
			continue
		}
		run, err := readRunFrom(root, entry.Name(), legacy)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}
		runsByID[entry.Name()] = run
	}
	return nil
}

func validateRunID(runID string) (string, error) {
	runID = strings.TrimSpace(runID)
	if runID == "" {
		return "", fmt.Errorf("run id is required")
	}
	if runID == "." || runID == ".." || strings.ContainsAny(runID, `/\\`) {
		return "", fmt.Errorf("invalid run id %q", runID)
	}
	for _, r := range runID {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			continue
		}
		return "", fmt.Errorf("invalid run id %q", runID)
	}
	return runID, nil
}

func runJSONPath(root, runID string) (string, error) {
	runID, err := validateRunID(runID)
	if err != nil {
		return "", err
	}
	return state.Path(root, "runs", runID, "run.json")
}

func eventsPath(root, runID string) (string, error) {
	runID, err := validateRunID(runID)
	if err != nil {
		return "", err
	}
	return state.Path(root, "runs", runID, "events.jsonl")
}

func legacyRunJSONPath(root, runID string) (string, error) {
	runID, err := validateRunID(runID)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".struktly", "runs", runID, "run.json"), nil
}

func legacyEventsPath(root, runID string) (string, error) {
	runID, err := validateRunID(runID)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, ".struktly", "runs", runID, "events.jsonl"), nil
}
