package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	repoctx "github.com/struktly/struktly/internal/context"
)

func TestRunCLIStructuredErrorsAndExitCodes(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer
	exitCode := runCLI(context.Background(), []string{"brief", "--root", root, "--json", "task"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 1 {
		t.Fatalf("outside-Git exit code = %d, want 1; stderr=%s", exitCode, &stderr)
	}
	if stdout.Len() != 0 {
		t.Fatalf("expected clean stdout, got %s", &stdout)
	}
	var doc errorDocument
	if err := json.Unmarshal(stderr.Bytes(), &doc); err != nil {
		t.Fatalf("structured stderr is not JSON: %v\n%s", err, &stderr)
	}
	if doc.Schema != "struktly/error/v1" || doc.Error.Code != "not_git_repository" {
		t.Fatalf("unexpected error document: %+v", doc)
	}

	stdout.Reset()
	stderr.Reset()
	exitCode = runCLI(context.Background(), []string{"unknown-command"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 2 || !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("invocation failure = exit %d, stderr %q", exitCode, &stderr)
	}
}

func TestRemovedProductStateCommandsAreUnavailable(t *testing.T) {
	for _, command := range []string{"evidence", "memory", "run"} {
		t.Run(command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			exitCode := runCLI(context.Background(), []string{command}, strings.NewReader(""), &stdout, &stderr)
			if exitCode != 2 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "unknown command") {
				t.Fatalf("removed command %q: exit=%d stdout=%q stderr=%q", command, exitCode, &stdout, &stderr)
			}
		})
	}
}

func TestRunCLICanceledExitCode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var stdout, stderr bytes.Buffer
	exitCode := runCLI(ctx, []string{"brief", "--root", t.TempDir(), "--json", "task"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 130 {
		t.Fatalf("canceled exit code = %d, want 130; stderr=%s", exitCode, &stderr)
	}
	var doc errorDocument
	if err := json.Unmarshal(stderr.Bytes(), &doc); err != nil {
		t.Fatalf("structured cancellation is not JSON: %v\n%s", err, &stderr)
	}
	if doc.Error.Code != "canceled" {
		t.Fatalf("unexpected cancellation error: %+v", doc)
	}
}

func TestRunCLIClassifiesInvalidPortableTask(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "README.md"), "# Repo\n")
	writeTestFile(t, filepath.Join(root, ".struktly", "tasks", "wrong.md"), "# Missing task frontmatter\n")
	initTestGitRepo(t, root)

	var stdout, stderr bytes.Buffer
	exitCode := runCLI(context.Background(), []string{"validate", "--root", root, "--json"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 1 || stdout.Len() != 0 {
		t.Fatalf("invalid task exit=%d stdout=%q stderr=%q", exitCode, &stdout, &stderr)
	}
	var doc errorDocument
	if err := json.Unmarshal(stderr.Bytes(), &doc); err != nil {
		t.Fatalf("structured task error is not JSON: %v\n%s", err, &stderr)
	}
	if doc.Error.Code != "invalid_task" {
		t.Fatalf("error code = %q, want invalid_task", doc.Error.Code)
	}
}

func TestJSONErrorRequestedHonorsExplicitFalse(t *testing.T) {
	if jsonErrorRequested([]string{"brief", "--json=false"}) {
		t.Fatal("--json=false requested structured errors")
	}
	if !jsonErrorRequested([]string{"brief", "--json=false", "--json-errors=true"}) {
		t.Fatal("--json-errors=true did not request structured errors")
	}
}

func TestVersionCommandReportsBuildMetadata(t *testing.T) {
	stdout, stderr, err := executeTestCommand("version", "--json")
	if err != nil {
		t.Fatalf("version returned error: %v\nstderr:\n%s", err, stderr)
	}
	var info struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(stdout), &info); err != nil {
		t.Fatalf("version output is not JSON: %v\n%s", err, stdout)
	}
	if info.Version == "" {
		t.Fatalf("version was empty: %s", stdout)
	}
}

func TestCapabilitiesCommandReportsContextContract(t *testing.T) {
	stdout, stderr, err := executeTestCommand("capabilities", "--json")
	if err != nil {
		t.Fatalf("capabilities returned error: %v\nstderr:\n%s", err, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("capabilities wrote diagnostics on success: %s", stderr)
	}
	var document capabilitiesDocument
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("capabilities output is not JSON: %v\n%s", err, stdout)
	}
	if document.Schema != capabilitiesSchema || !slices.Contains(document.Features, "context.no_write") || !slices.Contains(document.Features, "context.expect_base_revision") {
		t.Fatalf("unexpected capabilities: %+v", document)
	}
	if !slices.Contains(document.Features, "context.limits") {
		t.Fatalf("capabilities missing context.limits: %+v", document)
	}
	if !slices.Contains(document.Commands, "tasks") || !slices.Contains(document.Schemas, repoctx.TasksSchema) || !slices.Contains(document.Features, "tasks.partial_results") {
		t.Fatalf("capabilities do not advertise tasks contract: %+v", document)
	}
	if !slices.Contains(document.Schemas, "struktly/init-result/v1") || !slices.Contains(document.Schemas, "struktly/instruction-suggestions/v1") {
		t.Fatalf("capabilities do not advertise init/instruction-suggestions schemas: %+v", document)
	}
	if slices.Contains(document.Schemas, "struktly/packet/v1") {
		t.Fatalf("capabilities advertise historical packet generation: %+v", document)
	}
	for _, command := range []string{"evidence", "memory", "run"} {
		if slices.Contains(document.Commands, command) {
			t.Fatalf("capabilities advertise removed command %q: %+v", command, document)
		}
	}
}

func TestContextCLIRespectsAndValidatesLimits(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "service-timeout.go"), "package main\n")
	writeTestFile(t, filepath.Join(root, "request-timeout.go"), "package main\n")
	initTestGitRepo(t, root)

	stdout, stderr, err := executeTestCommand("context", "--root", root, "--json", "--no-write", "--max-items", "1", "timeout")
	if err != nil || strings.TrimSpace(stderr) != "" {
		t.Fatalf("context --max-items failed: err=%v stderr=%q", err, stderr)
	}
	var packet struct {
		Items  []any `json:"items"`
		Limits struct {
			MaxItems      int `json:"max_items"`
			MaxFileBytes  int `json:"max_file_bytes"`
			MaxTotalBytes int `json:"max_total_bytes"`
		} `json:"limits"`
	}
	if err := json.Unmarshal([]byte(stdout), &packet); err != nil {
		t.Fatalf("context packet output is not JSON: %v\nstdout=%s", err, stdout)
	}
	if len(packet.Items) != 1 {
		t.Fatalf("expected item limit to truncate selection to one item, got %d", len(packet.Items))
	}
	if packet.Limits.MaxItems != 1 || packet.Limits.MaxFileBytes != 65536 || packet.Limits.MaxTotalBytes != 524288 {
		t.Fatalf("packet limits should include requested and default values: %+v", packet.Limits)
	}

	for _, tc := range []struct {
		name      string
		args      []string
		wantError string
	}{
		{
			name:      "zero-max-items",
			args:      []string{"context", "--root", root, "--json", "--no-write", "--max-items", "0", "timeout"},
			wantError: "max_items must be greater than 0",
		},
		{
			name:      "loosened-max-items",
			args:      []string{"context", "--root", root, "--json", "--no-write", "--max-items", "41", "timeout"},
			wantError: "max_items exceeds default max 40",
		},
		{
			name:      "loosened-file-bytes",
			args:      []string{"context", "--root", root, "--json", "--no-write", "--max-file-bytes", "65537", "timeout"},
			wantError: "max_file_bytes exceeds default max 65536",
		},
		{
			name:      "loosened-total-bytes",
			args:      []string{"context", "--root", root, "--json", "--no-write", "--max-total-bytes", "524289", "timeout"},
			wantError: "max_total_bytes exceeds default max 524288",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, stderr, exitCode := executeCLICommand(tc.args...)
			if exitCode != 2 {
				t.Fatalf("expected context limit failure for %s", tc.name)
			}
			if strings.TrimSpace(stderr) == "" {
				t.Fatalf("expected structured json for %s error, got empty stderr", tc.name)
			}
			var doc errorDocument
			if err := json.Unmarshal([]byte(stderr), &doc); err != nil {
				t.Fatalf("expected structured error for %s: %v\nstderr=%s", tc.name, err, stderr)
			}
			if doc.Error.Code != "invalid_invocation" {
				t.Fatalf("expected invalid_invocation for %s, got %+v", tc.name, doc)
			}
			if !strings.Contains(doc.Error.Message, tc.wantError) {
				t.Fatalf("unexpected %s error=%+v", tc.name, doc.Error)
			}
		})
	}
}

func executeCLICommand(args ...string) (stdout string, stderr string, exitCode int) {
	var commandStdout bytes.Buffer
	var commandStderr bytes.Buffer
	exitCode = runCLI(context.Background(), args, strings.NewReader(""), &commandStdout, &commandStderr)
	return commandStdout.String(), commandStderr.String(), exitCode
}

func TestTasksCommandEmitsValidAndInvalidFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, ".struktly", "tasks", "add-timeout.md"), taskDocument("add-timeout", "Add timeout"))
	writeTestFile(t, filepath.Join(root, ".struktly", "tasks", "z-review.md"), taskDocument("z-review", "Review changes"))
	writeTestFile(t, filepath.Join(root, ".struktly", "tasks", "broken.md"), "# missing frontmatter\n")

	stdout, stderr, err := executeTestCommand("tasks", "--root", root, "--json")
	if err != nil {
		t.Fatalf("tasks returned error: %v\nstderr:\n%s", err, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("tasks wrote diagnostics on success: %s", stderr)
	}
	var document repoctx.TasksDocument
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatalf("tasks output is not JSON: %v\n%s", err, stdout)
	}
	if document.Schema != repoctx.TasksSchema || len(document.Tasks) != 2 || len(document.Invalid) != 1 {
		t.Fatalf("unexpected tasks document: %#v", document)
	}
	if document.Tasks[0].ID != "add-timeout" || document.Tasks[1].ID != "z-review" || document.Invalid[0].Path != ".struktly/tasks/broken.md" {
		t.Fatalf("unexpected task ordering or invalid result: %#v", document)
	}
}

func TestTasksCommandMissingDirectoryIsSuccess(t *testing.T) {
	stdout, stderr, err := executeTestCommand("tasks", "--root", t.TempDir(), "--json")
	if err != nil {
		t.Fatalf("tasks returned error: %v\nstderr:\n%s", err, stderr)
	}
	var document repoctx.TasksDocument
	if err := json.Unmarshal([]byte(stdout), &document); err != nil {
		t.Fatal(err)
	}
	if document.Tasks == nil || document.Invalid == nil || len(document.Tasks) != 0 || len(document.Invalid) != 0 {
		t.Fatalf("unexpected empty document: %#v", document)
	}
}

func TestTasksCommandUnreadableRootUsesStructuredError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	var stdout, stderr bytes.Buffer
	exitCode := runCLI(context.Background(), []string{"tasks", "--root", missing, "--json"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 1 || stdout.Len() != 0 {
		t.Fatalf("tasks failure exit=%d stdout=%q stderr=%q", exitCode, &stdout, &stderr)
	}
	var document errorDocument
	if err := json.Unmarshal(stderr.Bytes(), &document); err != nil {
		t.Fatalf("tasks error is not structured JSON: %v\n%s", err, &stderr)
	}
	if document.Error.Code != "operation_failed" || !strings.Contains(document.Error.Message, "stat root") {
		t.Fatalf("unexpected error document: %#v", document)
	}
}

func TestInitScaffoldsAndScans(t *testing.T) {
	root := t.TempDir()

	stdout, stderr, err := executeTestCommand("init", "--root", root)
	if err != nil {
		t.Fatalf("init returned error: %v\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stdout, "created .struktly/config.json") {
		t.Fatalf("expected config.json creation line, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "wrote .struktly/project-context.md") {
		t.Fatalf("expected scan confirmation, got:\n%s", stdout)
	}
	for _, rel := range []string{".struktly/config.json", ".struktly/project-context.md"} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("expected %s after init: %v", rel, err)
		}
	}

	stdout, stderr, err = executeTestCommand("init", "--root", root)
	if err != nil {
		t.Fatalf("second init returned error: %v\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stdout, "kept .struktly/config.json (already exists)") {
		t.Fatalf("expected config.json kept line, got:\n%s", stdout)
	}
}

func TestBriefStdoutPrintsPacket(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "README.md"), "# Repo\n")
	writeTestFile(t, filepath.Join(root, "go.mod"), "module example.com/repo\n\ngo 1.24.0\n")

	if _, stderr, err := executeTestCommand("scan", "--root", root); err != nil {
		t.Fatalf("scan returned error: %v\nstderr:\n%s", err, stderr)
	}
	initTestGitRepo(t, root)

	stdout, stderr, err := executeTestCommand("brief", "--root", root, "--stdout", "Add feature")
	if err != nil {
		t.Fatalf("brief --stdout returned error: %v\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stdout, "# Context packet") {
		t.Fatalf("expected packet content on stdout, got:\n%s", stdout)
	}
	if strings.Contains(stdout, "wrote") {
		t.Fatalf("expected no wrote confirmation on stdout, got:\n%s", stdout)
	}
	if !strings.HasPrefix(stderr, "wrote ") {
		t.Fatalf("expected wrote confirmation on stderr, got:\n%s", stderr)
	}

	packetPath := strings.TrimSpace(strings.TrimPrefix(stderr, "wrote "))
	data, err := os.ReadFile(packetPath)
	if err != nil {
		t.Fatalf("read packet %s: %v", packetPath, err)
	}
	if stdout != string(data) {
		t.Fatalf("stdout does not match packet file content\nstdout:\n%s\nfile:\n%s", stdout, data)
	}
}

func TestContextNoWriteProducesPacketWithoutArtifacts(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "README.md"), "# Repo\n")
	initTestGitRepo(t, root)

	stdout, stderr, err := executeTestCommand("context", "--root", root, "--json", "--no-write", "inspect repository")
	if err != nil {
		t.Fatalf("context --no-write returned error: %v\nstderr:\n%s", err, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("context --no-write wrote diagnostics: %s", stderr)
	}
	var packet struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal([]byte(stdout), &packet); err != nil || packet.Schema != repoctx.PacketSchema {
		t.Fatalf("unexpected packet output: err=%v output=%s", err, stdout)
	}
	if _, err := os.Stat(filepath.Join(root, ".struktly")); !os.IsNotExist(err) {
		t.Fatalf("context --no-write created repository files: %v", err)
	}
}

func TestScanNoWriteProducesSnapshotWithoutArtifacts(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "README.md"), "# Repo\n")

	stdout, stderr, err := executeTestCommand("scan", "--root", root, "--json", "--no-write")
	if err != nil {
		t.Fatalf("scan --no-write returned error: %v\nstderr:\n%s", err, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("scan --no-write wrote diagnostics: %s", stderr)
	}
	var snapshot struct {
		Schema string `json:"schema"`
	}
	if err := json.Unmarshal([]byte(stdout), &snapshot); err != nil || snapshot.Schema != repoctx.SnapshotSchema {
		t.Fatalf("unexpected snapshot output: err=%v output=%s", err, stdout)
	}
	if _, err := os.Stat(filepath.Join(root, ".struktly")); !os.IsNotExist(err) {
		t.Fatalf("scan --no-write created repository files: %v", err)
	}
}

func TestContextExpectedRevisionMismatchIsStructured(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "README.md"), "# Repo\n")
	initTestGitRepo(t, root)

	var stdout, stderr bytes.Buffer
	exitCode := runCLI(context.Background(), []string{
		"context", "--root", root, "--json", "--no-write",
		"--expect-base-revision", strings.Repeat("0", 40), "inspect repository",
	}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 1 || stdout.Len() != 0 {
		t.Fatalf("revision mismatch exit=%d stdout=%q stderr=%q", exitCode, &stdout, &stderr)
	}
	var document errorDocument
	if err := json.Unmarshal(stderr.Bytes(), &document); err != nil {
		t.Fatalf("revision mismatch error is not JSON: %v\n%s", err, &stderr)
	}
	if document.Error.Code != "repository_changed" {
		t.Fatalf("error code = %q, want repository_changed", document.Error.Code)
	}
}

func TestSuggestInstructionsWritesSuggestedFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "Makefile"), "test:\n\tgo test ./...\n")
	writeTestFile(t, filepath.Join(root, ".struktly/direction.md"), "# Direction\n\n## Non-goals\n\n- A chat UI\n")
	writeTestFile(t, filepath.Join(root, ".struktly/constraints.md"), "# Constraints\n\n- Keep changes reviewable.\n")
	writeTestFile(t, filepath.Join(root, ".struktly/decisions.md"), "# Decisions\n\n## 2026-07-04 — Accepted\n\n**Decision:** Keep output stable.\n\n**Status:** accepted\n")

	scanStdout, scanStderr, err := executeTestCommand("scan", "--root", root)
	if err != nil {
		t.Fatalf("scan returned error: %v\nstderr:\n%s", err, scanStderr)
	}
	if !strings.Contains(scanStdout, "wrote") {
		t.Fatalf("expected scan confirmation, got:\n%s", scanStdout)
	}

	stdout, stderr, err := executeTestCommand("suggest-instructions", "--root", root)
	if err != nil {
		t.Fatalf("suggest-instructions returned error: %v\nstderr:\n%s", err, stderr)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("expected empty stderr, got:\n%s", stderr)
	}
	for _, name := range []string{"AGENTS.suggested.md", "CLAUDE.suggested.md", "CURSOR.suggested.md"} {
		if !strings.Contains(stdout, name) {
			t.Fatalf("expected %s in stdout, got:\n%s", name, stdout)
		}
		path := filepath.Join(root, ".struktly", "agent-instructions", name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if !strings.Contains(string(data), "Keep changes reviewable.") {
			t.Fatalf("expected constraints content in %s", name)
		}
		if !strings.Contains(string(data), "A chat UI") {
			t.Fatalf("expected direction non-goals in %s", name)
		}
	}
}

func TestInspectCommandsEmitStructuredOutput(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "README.md"), "# Repo\n")
	writeTestFile(t, filepath.Join(root, ".struktly/config.json"), `{"schema":"struktly/config/v1","context":{},"checks":{}}`)
	initTestGitRepo(t, root)

	for _, command := range []struct {
		args   []string
		schema string
	}{
		{args: []string{"status", "--root", root, "--json"}, schema: "struktly/status/v1"},
		{args: []string{"validate", "--root", root, "--json"}, schema: "struktly/validation/v1"},
		{args: []string{"doctor", "--root", root, "--json"}, schema: "struktly/doctor/v1"},
		{args: []string{"explain", "--root", root, "--json", "README.md"}, schema: "struktly/explanation/v1"},
	} {
		stdout, stderr, err := executeTestCommand(command.args...)
		if err != nil {
			t.Fatalf("%v returned error: %v\nstderr:\n%s", command.args, err, stderr)
		}
		if strings.TrimSpace(stderr) != "" {
			t.Fatalf("%v wrote diagnostics on success: %s", command.args, stderr)
		}
		var document struct {
			Schema string `json:"schema"`
		}
		if err := json.Unmarshal([]byte(stdout), &document); err != nil {
			t.Fatalf("%v output is not JSON: %v\n%s", command.args, err, stdout)
		}
		if document.Schema != command.schema {
			t.Fatalf("%v schema = %q, want %q", command.args, document.Schema, command.schema)
		}
	}
}

func executeTestCommand(args ...string) (string, string, error) {
	cmd := newRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir fixture dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func taskDocument(id, title string) string {
	return `---
type: task
schema: struktly/task/v1
id: ` + id + `
title: "` + title + `"
status: ready
priority: medium
created: 2026-07-13
agent: unassigned
---

# ` + title + `

## Objective

Complete the task.
`
}

func initTestGitRepo(t *testing.T, root string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"},
		{"config", "commit.gpgsign", "false"},
		{"add", "-A"},
		{"commit", "-qm", "fixture"},
	} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
}

// The two commands that write into the repository carry versioned machine
// contracts, so a caller that refuses to parse prose — Platform is one — can
// consume what they did.
func TestInitAndSuggestInstructionsSpeakJSON(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "README.md"), "# Repo\n")

	stdout, stderr, err := executeTestCommand("init", "--root", root, "--json")
	if err != nil {
		t.Fatalf("init --json: %v\nstderr:\n%s", err, stderr)
	}
	var initDoc struct {
		Schema   string   `json:"schema"`
		Root     string   `json:"root"`
		Created  []string `json:"created"`
		Skipped  []string `json:"skipped"`
		Snapshot string   `json:"snapshot"`
	}
	if err := json.Unmarshal([]byte(stdout), &initDoc); err != nil {
		t.Fatalf("init output is not JSON: %v\n%s", err, stdout)
	}
	if initDoc.Schema != "struktly/init-result/v1" || initDoc.Snapshot != ".struktly/project-context.md" {
		t.Fatalf("init document = %+v", initDoc)
	}
	// The absolute root names the workstation that ran the command, so it must
	// not reach a document whose other paths are repository-relative.
	if initDoc.Root != "." {
		t.Fatalf("init root = %q, want %q", initDoc.Root, ".")
	}
	if len(initDoc.Created) == 0 || initDoc.Created[0] != ".struktly/config.json" {
		t.Fatalf("init created = %v, want config.json first", initDoc.Created)
	}

	stdout, stderr, err = executeTestCommand("init", "--root", root, "--json")
	if err != nil {
		t.Fatalf("second init --json: %v\nstderr:\n%s", err, stderr)
	}
	if err := json.Unmarshal([]byte(stdout), &initDoc); err != nil {
		t.Fatal(err)
	}
	if len(initDoc.Skipped) == 0 {
		t.Fatalf("second init document = %+v, want the existing config reported as skipped, not rewritten", initDoc)
	}

	stdout, stderr, err = executeTestCommand("suggest-instructions", "--root", root, "--json")
	if err != nil {
		t.Fatalf("suggest-instructions --json: %v\nstderr:\n%s", err, stderr)
	}
	var suggestions struct {
		Schema  string   `json:"schema"`
		Root    string   `json:"root"`
		Written []string `json:"written"`
	}
	if err := json.Unmarshal([]byte(stdout), &suggestions); err != nil {
		t.Fatalf("suggestions output is not JSON: %v\n%s", err, stdout)
	}
	if suggestions.Schema != "struktly/instruction-suggestions/v1" || len(suggestions.Written) == 0 {
		t.Fatalf("suggestions document = %+v, want written draft paths", suggestions)
	}
	if suggestions.Root != "." {
		t.Fatalf("suggestions root = %q, want %q", suggestions.Root, ".")
	}
	for _, path := range suggestions.Written {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Fatalf("reported draft %s does not exist: %v", path, err)
		}
	}
}
