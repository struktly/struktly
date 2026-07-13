package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

func TestInitScaffoldsAndScans(t *testing.T) {
	root := t.TempDir()

	stdout, stderr, err := executeTestCommand("init", "--root", root)
	if err != nil {
		t.Fatalf("init returned error: %v\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stdout, "created .struktly/direction.md") {
		t.Fatalf("expected direction.md creation line, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "created .struktly/constraints.md") {
		t.Fatalf("expected constraints.md creation line, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "wrote .struktly/project-context.md") {
		t.Fatalf("expected scan confirmation, got:\n%s", stdout)
	}
	for _, rel := range []string{".struktly/config.json", ".struktly/direction.md", ".struktly/constraints.md", ".struktly/project-context.md"} {
		if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
			t.Fatalf("expected %s after init: %v", rel, err)
		}
	}

	stdout, stderr, err = executeTestCommand("init", "--root", root)
	if err != nil {
		t.Fatalf("second init returned error: %v\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stdout, "kept .struktly/direction.md (already exists)") {
		t.Fatalf("expected direction.md kept line, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "kept .struktly/constraints.md (already exists)") {
		t.Fatalf("expected constraints.md kept line, got:\n%s", stdout)
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
	if !strings.Contains(stdout, "# Struktly Context Packet") {
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

func TestSuggestInstructionsWritesSuggestedFiles(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "Makefile"), "test:\n\tgo test ./...\n")
	writeTestFile(t, filepath.Join(root, ".struktly/direction.md"), "# Direction\n\n## Non-goals\n\n- A chat UI\n")
	writeTestFile(t, filepath.Join(root, ".struktly/constraints.md"), "# Constraints\n\n- Keep changes reviewable.\n")
	writeTestFile(t, filepath.Join(root, ".struktly/decisions.md"), "# Decisions\n\n## 2026-07-04 — Accepted\n\n**Decision:** Keep local-first.\n\n**Status:** accepted\n")

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

func TestEvidenceAppendsLedgerEntry(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "packet.md"), "# Packet\n")

	stdout, stderr, err := executeTestCommand(
		"evidence",
		"--root", root,
		"--task", "Verify evidence command",
		"--agent", "go test",
		"--outcome", "Evidence command appends structured markdown.",
		"--context-packet", "packet.md",
		"--checks", "go test ./cmd/struktly/...",
		"--result", "pass",
	)
	if err != nil {
		t.Fatalf("evidence returned error: %v\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stdout, "appended to") {
		t.Fatalf("expected append confirmation, got:\n%s", stdout)
	}

	data, err := os.ReadFile(filepath.Join(root, ".struktly", "evidence.md"))
	if err != nil {
		t.Fatalf("read evidence ledger: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "Verify evidence command") {
		t.Fatalf("expected task in evidence ledger:\n%s", content)
	}
	if !strings.Contains(content, "sha256:") {
		t.Fatalf("expected context packet hash in evidence ledger:\n%s", content)
	}
}

func TestEvidenceRunChecksExecutesDeclaredChecks(t *testing.T) {
	root := t.TempDir()

	stdout, stderr, err := executeTestCommand(
		"evidence",
		"--root", root,
		"--task", "Verified passing checks",
		"--agent", "struktly",
		"--outcome", "Checks executed by struktly.",
		"--checks", "true",
		"--run-checks",
	)
	if err != nil {
		t.Fatalf("evidence --run-checks returned error: %v\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stdout, "appended to") {
		t.Fatalf("expected append confirmation, got:\n%s", stdout)
	}

	data, err := os.ReadFile(filepath.Join(root, ".struktly", "evidence.md"))
	if err != nil {
		t.Fatalf("read evidence ledger: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "checks executed by struktly") {
		t.Fatalf("expected verification mode in evidence ledger:\n%s", content)
	}
	if !strings.Contains(content, "| **Checks result** | pass |") {
		t.Fatalf("expected derived pass result in evidence ledger:\n%s", content)
	}

	// A failing check is data, not a CLI error: recording succeeds and the entry says fail.
	_, stderr, err = executeTestCommand(
		"evidence",
		"--root", root,
		"--task", "Verified failing checks",
		"--agent", "struktly",
		"--outcome", "Checks executed by struktly.",
		"--checks", "false",
		"--run-checks",
	)
	if err != nil {
		t.Fatalf("evidence --run-checks with failing check should exit 0, got: %v\nstderr:\n%s", err, stderr)
	}

	data, err = os.ReadFile(filepath.Join(root, ".struktly", "evidence.md"))
	if err != nil {
		t.Fatalf("read evidence ledger after failing check: %v", err)
	}
	content = string(data)
	if !strings.Contains(content, "| **Checks result** | fail |") {
		t.Fatalf("expected derived fail result in evidence ledger:\n%s", content)
	}
	if !strings.Contains(content, "- `false` — fail (exit 1, ") {
		t.Fatalf("expected failing check line with exit code:\n%s", content)
	}
}

func TestRunCommandsCreateListShowEventAndComplete(t *testing.T) {
	t.Setenv("STRUKTLY_STATE_DIR", t.TempDir())
	root := t.TempDir()

	stdout, stderr, err := executeTestCommand("run", "create", "--root", root, "--goal", "Prepare handoff")
	if err != nil {
		t.Fatalf("run create returned error: %v\nstderr:\n%s", err, stderr)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(stdout), &created); err != nil {
		t.Fatalf("decode run create stdout: %v\n%s", err, stdout)
	}
	if created.ID == "" {
		t.Fatalf("expected run id in stdout: %s", stdout)
	}

	stdout, stderr, err = executeTestCommand("run", "event", "--root", root, created.ID, "--type", "agent_message", "--message", "Ready for agent.")
	if err != nil {
		t.Fatalf("run event returned error: %v\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stdout, "agent_message") {
		t.Fatalf("expected event type in stdout: %s", stdout)
	}

	stdout, stderr, err = executeTestCommand("run", "list", "--root", root)
	if err != nil {
		t.Fatalf("run list returned error: %v\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stdout, created.ID) {
		t.Fatalf("expected created run in list: %s", stdout)
	}

	stdout, stderr, err = executeTestCommand("run", "show", "--root", root, created.ID)
	if err != nil {
		t.Fatalf("run show returned error: %v\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stdout, "Ready for agent.") {
		t.Fatalf("expected event in show output: %s", stdout)
	}

	stdout, stderr, err = executeTestCommand("run", "complete", "--root", root, created.ID)
	if err != nil {
		t.Fatalf("run complete returned error: %v\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stdout, `"status": "completed"`) {
		t.Fatalf("expected completed status: %s", stdout)
	}
}

func TestScanAndBriefCanAttachToRunFromCLI(t *testing.T) {
	t.Setenv("STRUKTLY_STATE_DIR", t.TempDir())
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "README.md"), "# Repo\n")
	writeTestFile(t, filepath.Join(root, "go.mod"), "module example.com/repo\n\ngo 1.24.0\n")
	initTestGitRepo(t, root)

	stdout, stderr, err := executeTestCommand("run", "create", "--root", root, "--goal", "Attach outputs")
	if err != nil {
		t.Fatalf("run create returned error: %v\nstderr:\n%s", err, stderr)
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(stdout), &created); err != nil {
		t.Fatalf("decode run create stdout: %v\n%s", err, stdout)
	}

	if _, stderr, err := executeTestCommand("scan", "--root", root, "--run", created.ID); err != nil {
		t.Fatalf("scan --run returned error: %v\nstderr:\n%s", err, stderr)
	}
	if _, stderr, err := executeTestCommand("brief", "--root", root, "--run", created.ID, "Attach context"); err != nil {
		t.Fatalf("brief --run returned error: %v\nstderr:\n%s", err, stderr)
	}

	stdout, stderr, err = executeTestCommand("run", "show", "--root", root, created.ID)
	if err != nil {
		t.Fatalf("run show returned error: %v\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stdout, `"type": "scan"`) || !strings.Contains(stdout, `"type": "brief"`) {
		t.Fatalf("expected scan and brief artifacts in run show: %s", stdout)
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

func TestMemoryCommandsCandidateApproveRejectAndList(t *testing.T) {
	t.Setenv("STRUKTLY_STATE_DIR", t.TempDir())
	root := t.TempDir()

	runStdout, stderr, err := executeTestCommand("run", "create", "--root", root, "--goal", "Capture memory")
	if err != nil {
		t.Fatalf("run create returned error: %v\nstderr:\n%s", err, stderr)
	}
	var createdRun struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(runStdout), &createdRun); err != nil {
		t.Fatalf("decode run create stdout: %v\n%s", err, runStdout)
	}

	stdout, stderr, err := executeTestCommand(
		"memory", "candidate",
		"--root", root,
		"--scope", "repository",
		"--content", "Prefer file-backed state for v1.5 workflows.",
		"--tags", "local-first,file-contract",
		"--source-run-id", createdRun.ID,
		"--source-artifact", ".struktly/project-context.md",
	)
	if err != nil {
		t.Fatalf("memory candidate returned error: %v\nstderr:\n%s", err, stderr)
	}
	var candidate struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(stdout), &candidate); err != nil {
		t.Fatalf("decode memory candidate stdout: %v\n%s", err, stdout)
	}

	stdout, stderr, err = executeTestCommand("memory", "candidates", "--root", root)
	if err != nil {
		t.Fatalf("memory candidates returned error: %v\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stdout, candidate.ID) {
		t.Fatalf("expected candidate in list: %s", stdout)
	}

	stdout, stderr, err = executeTestCommand("memory", "approve", "--root", root, candidate.ID)
	if err != nil {
		t.Fatalf("memory approve returned error: %v\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stdout, `"status": "approved"`) {
		t.Fatalf("expected approved status: %s", stdout)
	}

	stdout, stderr, err = executeTestCommand("memory", "list", "--root", root)
	if err != nil {
		t.Fatalf("memory list returned error: %v\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stdout, "Prefer file-backed state") {
		t.Fatalf("expected approved memory in list: %s", stdout)
	}

	rejectStdout, stderr, err := executeTestCommand("memory", "candidate", "--root", root, "--content", "Reject me")
	if err != nil {
		t.Fatalf("second memory candidate returned error: %v\nstderr:\n%s", err, stderr)
	}
	var rejectedCandidate struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(rejectStdout), &rejectedCandidate); err != nil {
		t.Fatalf("decode second candidate stdout: %v\n%s", err, rejectStdout)
	}
	stdout, stderr, err = executeTestCommand("memory", "reject", "--root", root, rejectedCandidate.ID)
	if err != nil {
		t.Fatalf("memory reject returned error: %v\nstderr:\n%s", err, stderr)
	}
	if !strings.Contains(stdout, `"status": "rejected"`) {
		t.Fatalf("expected rejected status: %s", stdout)
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
