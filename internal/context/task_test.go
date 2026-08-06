package context

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const validTaskDocument = `---
type: task
schema: struktly/task/v1
id: add-timeout
title: "Add request timeout"
status: ready
priority: medium
created: 2026-07-13
agent: unassigned
---

# Add request timeout

## Pick up this task

Start here.

## Objective

Add the timeout.

## Constraints

- Keep compatibility.

## Non-goals

- Do not change the public API.

## Required outcomes

- [ ] Tests pass.
- Existing behavior remains
  covered.

## Execution plan

1. Implement it.

## Definition of done

The timeout works and ` + "`go test ./...`" + ` passes.
`

func TestLoadTasksValidatesCanonicalTask(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".struktly/tasks/add-timeout.md", validTaskDocument)

	tasks, err := LoadTasks(root)
	if err != nil {
		t.Fatalf("LoadTasks returned error: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("tasks = %#v, want one", tasks)
	}
	task := tasks[0]
	if task.Path != ".struktly/tasks/add-timeout.md" || task.ID != "add-timeout" || task.Status != "ready" || task.Agent != "unassigned" {
		t.Fatalf("unexpected task: %#v", task)
	}
	wantDigest := sha256.Sum256([]byte(validTaskDocument))
	if task.SHA256 != hex.EncodeToString(wantDigest[:]) {
		t.Fatalf("sha256 = %q, want %x", task.SHA256, wantDigest)
	}
	if task.Contract.Outcome != "Add the timeout." {
		t.Fatalf("outcome = %q", task.Contract.Outcome)
	}
	if !reflect.DeepEqual(task.Contract.DoneWhen, []string{"Tests pass.", "Existing behavior remains covered."}) {
		t.Fatalf("done_when = %#v", task.Contract.DoneWhen)
	}
	if !reflect.DeepEqual(task.Contract.NonGoals, []string{"Do not change the public API."}) {
		t.Fatalf("non_goals = %#v", task.Contract.NonGoals)
	}
	if !reflect.DeepEqual(task.Contract.RequiredChecks, []string{"go test ./..."}) {
		t.Fatalf("required_checks = %#v", task.Contract.RequiredChecks)
	}
	if len(task.CompatibilityNotes) != 0 {
		t.Fatalf("canonical task has compatibility notes: %#v", task.CompatibilityNotes)
	}
}

func TestDiscoverTasksReturnsCanonicalOrderAndPartialResults(t *testing.T) {
	root := t.TempDir()
	second := strings.ReplaceAll(validTaskDocument, "add-timeout", "z-last")
	second = strings.Replace(second, "Add request timeout", "Last task", 1)
	writeFile(t, root, ".struktly/tasks/z-last.md", second)
	writeFile(t, root, ".struktly/tasks/add-timeout.md", validTaskDocument)
	writeFile(t, root, ".struktly/tasks/broken.md", "# missing frontmatter\n")

	document, err := DiscoverTasks(root)
	if err != nil {
		t.Fatalf("DiscoverTasks returned error: %v", err)
	}
	if document.Schema != TasksSchema || len(document.Tasks) != 2 || len(document.Invalid) != 1 {
		t.Fatalf("unexpected tasks document: %#v", document)
	}
	if document.Tasks[0].Path != ".struktly/tasks/add-timeout.md" || document.Tasks[1].Path != ".struktly/tasks/z-last.md" {
		t.Fatalf("tasks are not in canonical order: %#v", document.Tasks)
	}
	if document.Invalid[0].Path != ".struktly/tasks/broken.md" || !strings.Contains(document.Invalid[0].Reason, "frontmatter") {
		t.Fatalf("unexpected invalid task: %#v", document.Invalid)
	}
}

func TestDiscoverTasksAllowsMissingBodyContract(t *testing.T) {
	root := t.TempDir()
	declaration := `---
type: task
schema: struktly/task/v1
id: historical-note
title: "Historical note"
status: done
priority: low
created: 2026-07-13
agent: unassigned
---

# Historical note
`
	writeFile(t, root, ".struktly/tasks/historical-note.md", declaration)

	document, err := DiscoverTasks(root)
	if err != nil {
		t.Fatalf("DiscoverTasks returned error: %v", err)
	}
	if len(document.Tasks) != 1 || len(document.Invalid) != 0 {
		t.Fatalf("unexpected tasks document: %#v", document)
	}
	contract := document.Tasks[0].Contract
	if contract.Outcome != "" || len(contract.DoneWhen) != 0 || len(contract.NonGoals) != 0 || len(contract.RequiredChecks) != 0 {
		t.Fatalf("missing body contract should remain empty: %#v", contract)
	}
	if len(document.Tasks[0].CompatibilityNotes) != 1 || !strings.Contains(document.Tasks[0].CompatibilityNotes[0], "canonical task/v1 validation") {
		t.Fatalf("missing body compatibility was not disclosed: %#v", document.Tasks[0].CompatibilityNotes)
	}
}

func TestDiscoverTasksDisclosesHistoricalBodyMappings(t *testing.T) {
	root := t.TempDir()
	document := strings.Replace(validTaskDocument, "## Objective\n\nAdd the timeout.", "## Mission\n\nAdd the timeout.", 1)
	document = strings.Replace(document, "## Required outcomes\n\n- [ ] Tests pass.\n- Existing behavior remains\n  covered.", "## Success criteria\n\n- Tests pass.\n- Existing behavior remains covered.", 1)
	writeFile(t, root, ".struktly/tasks/add-timeout.md", document)

	discovery, err := DiscoverTasks(root)
	if err != nil {
		t.Fatalf("DiscoverTasks returned error: %v", err)
	}
	if len(discovery.Tasks) != 1 || len(discovery.Invalid) != 0 {
		t.Fatalf("unexpected tasks document: %#v", discovery)
	}
	notes := strings.Join(discovery.Tasks[0].CompatibilityNotes, "\n")
	for _, want := range []string{"Mapped historical Mission", "Mapped historical Success Criteria"} {
		if !strings.Contains(notes, want) {
			t.Fatalf("compatibility notes %q do not contain %q", notes, want)
		}
	}
	// Mission and Success criteria are accepted spellings, not defects, so the
	// mapping is disclosed without also claiming strict validation would reject
	// the body. Strict validation accepts it too.
	if strings.Contains(notes, "canonical task/v1 validation") {
		t.Fatalf("accepted historical headings reported as a validation failure: %q", notes)
	}
	if _, err := LoadTasks(root); err != nil {
		t.Fatalf("LoadTasks rejected accepted historical headings: %v", err)
	}
}

func TestDiscoverTasksToleratesHistoricalDottedIDOnly(t *testing.T) {
	root := t.TempDir()
	document := strings.ReplaceAll(validTaskDocument, "add-timeout", "release-v0.1.4")
	writeFile(t, root, ".struktly/tasks/release-v0.1.4.md", document)

	discovery, err := DiscoverTasks(root)
	if err != nil {
		t.Fatalf("DiscoverTasks returned error: %v", err)
	}
	if len(discovery.Tasks) != 1 || len(discovery.Invalid) != 0 {
		t.Fatalf("unexpected tasks document: %#v", discovery)
	}
	notes := discovery.Tasks[0].CompatibilityNotes
	if len(notes) != 1 || notes[0] != "Compatibility import: canonical task/v1 validation rejects this historical dotted task ID." {
		t.Fatalf("unexpected compatibility notes: %#v", notes)
	}
	if _, err := LoadTasks(root); err == nil || !strings.Contains(err.Error(), "single hyphens") {
		t.Fatalf("strict LoadTasks should reject dotted id, got %v", err)
	}
}

func TestDiscoverTasksMissingDirectoryIsEmpty(t *testing.T) {
	document, err := DiscoverTasks(t.TempDir())
	if err != nil {
		t.Fatalf("DiscoverTasks returned error: %v", err)
	}
	if document.Schema != TasksSchema || document.Tasks == nil || document.Invalid == nil || len(document.Tasks) != 0 || len(document.Invalid) != 0 {
		t.Fatalf("unexpected empty document: %#v", document)
	}
}

func TestDiscoverTasksRejectsSymlinkAsInvalid(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "target.md")
	if err := os.WriteFile(target, []byte(validTaskDocument), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".struktly", "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(root, ".struktly", "tasks", "add-timeout.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	document, err := DiscoverTasks(root)
	if err != nil {
		t.Fatalf("DiscoverTasks returned error: %v", err)
	}
	if len(document.Tasks) != 0 || len(document.Invalid) != 1 || !strings.Contains(document.Invalid[0].Reason, "symlink") {
		t.Fatalf("unexpected symlink result: %#v", document)
	}
}

// OKF v0.2 §4.1: producers may add frontmatter keys, and a consumer "MUST NOT
// reject documents with unrecognized fields" and SHOULD preserve them when
// round-tripping. This asserted the opposite until a task file carrying one
// extra key was found vanishing from the list entirely.
func TestUnknownFrontmatterIsPreservedNotRejected(t *testing.T) {
	root := t.TempDir()
	content := strings.Replace(validTaskDocument, "agent: unassigned", "agent: unassigned\nowner: team-platform", 1)
	writeFile(t, root, ".struktly/tasks/add-timeout.md", content)

	tasks, err := LoadTasks(root)
	if err != nil {
		t.Fatalf("LoadTasks rejected an extension field: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected the task to survive, got %d", len(tasks))
	}
	if got := tasks[0].Extensions["owner"]; got != "team-platform" {
		t.Fatalf("extension dropped: Extensions = %#v", tasks[0].Extensions)
	}
	if tasks[0].Title == "" || tasks[0].Status == "" {
		t.Fatalf("known fields lost alongside the extension: %#v", tasks[0])
	}
}

// OKF v0.2 reserves index.md (§8) and log.md (§9). DiscoverTasks skipped them
// while LoadTasks strict-validated them, so a conforming task bundle failed
// `struktly validate` on files the format says are not tasks.
func TestLoadTasksSkipsReservedOKFFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".struktly/tasks/add-timeout.md", validTaskDocument)
	writeFile(t, root, ".struktly/tasks/index.md", "---\ntype: index\n---\n\n# Tasks\n")
	writeFile(t, root, ".struktly/tasks/log.md", "---\ntype: log\n---\n\n# Log\n")

	tasks, err := LoadTasks(root)
	if err != nil {
		t.Fatalf("LoadTasks rejected an OKF-conforming bundle: %v", err)
	}
	if len(tasks) != 1 || tasks[0].ID != "add-timeout" {
		t.Fatalf("unexpected tasks: %#v", tasks)
	}
}

// One malformed file used to abort the whole load, so `validate` disclosed the
// first problem and hid both the remaining problems and every valid sibling.
func TestLoadTasksReportsEveryInvalidFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".struktly/tasks/add-timeout.md", validTaskDocument)
	writeFile(t, root, ".struktly/tasks/broken-one.md", "no frontmatter\n")
	writeFile(t, root, ".struktly/tasks/broken-two.md", "also no frontmatter\n")

	_, err := LoadTasks(root)
	if err == nil {
		t.Fatal("LoadTasks accepted malformed tasks")
	}
	for _, want := range []string{"broken-one.md", "broken-two.md"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q does not report %q", err, want)
		}
	}
	if !errors.Is(err, ErrInvalidTask) {
		t.Fatalf("aggregated error lost its ErrInvalidTask identity: %v", err)
	}
}

// schemas/tasks.v1.json constrains priority to an enum and created to a date.
// Emitting "" for an absent optional field produced JSON the CLI's own schema
// rejects.
func TestTaskJSONOmitsAbsentOptionalFields(t *testing.T) {
	root := t.TempDir()
	minimal := `---
type: task
schema: struktly/task/v1
id: minimal
title: "Minimal"
status: ready
---

# Minimal

## Mission

Do the thing.

## Done when

- It is done.
`
	writeFile(t, root, ".struktly/tasks/minimal.md", minimal)

	document, err := DiscoverTasks(root)
	if err != nil {
		t.Fatalf("DiscoverTasks returned error: %v", err)
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	for _, absent := range []string{`"priority":""`, `"created":""`, `"agent":""`} {
		if strings.Contains(string(encoded), absent) {
			t.Fatalf("emitted %s, which schemas/tasks.v1.json rejects: %s", absent, encoded)
		}
	}
}

func TestLoadTasksRejectsMalformedTasks(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		content string
		want    string
	}{
		{name: "noncanonical status", path: "add-timeout.md", content: strings.Replace(validTaskDocument, "status: ready", "status: completed", 1), want: "unsupported status"},
		{name: "filename mismatch", path: "different.md", content: validTaskDocument, want: "must match filename"},
		{name: "no objective section", path: "add-timeout.md", content: strings.Replace(validTaskDocument, "## Objective", "## Background", 1), want: "needs an \"Objective\" section"},
		// Both accepted outcome spellings have to go: "Definition of done" is
		// as good a done-condition as "Required outcomes".
		{name: "no outcome section", path: "add-timeout.md", content: strings.NewReplacer("## Required outcomes", "## Notes", "## Definition of done", "## Background").Replace(validTaskDocument), want: "needs a \"Required outcomes\" section"},
		{name: "partial handoff", path: "add-timeout.md", content: strings.Replace(validTaskDocument, "agent: unassigned", "agent: codex\nagent_session: session-1", 1), want: "declared together"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, ".struktly/tasks/"+test.path, test.content)
			_, err := LoadTasks(root)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("LoadTasks error = %v, want %q", err, test.want)
			}
		})
	}
}
