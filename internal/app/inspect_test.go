package app

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestStatusReportsPortableFilesFromGitRoot(t *testing.T) {
	root := newGitRepository(t)
	writeFile(t, root, ".struktly/config.json", `{"schema":"struktly/config/v1","context":{},"checks":{}}`)
	writeFile(t, root, ".struktly/direction.md", "# Direction\n")
	writeFile(t, root, ".struktly/scans/latest.json", "{}\n")
	subdir := filepath.Join(root, "nested")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}

	report, err := Status(context.Background(), subdir)
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}
	if report.Schema != "struktly/status/v1" || !report.ConfigDeclared || report.ConfigPath != ".struktly/config.json" {
		t.Fatalf("unexpected status metadata: %#v", report)
	}
	want := []FileStatus{
		{Path: ".struktly/config.json", Status: "present"},
		{Path: ".struktly/direction.md", Status: "present"},
		{Path: ".struktly/constraints.md", Status: "missing"},
		{Path: ".struktly/decisions.md", Status: "missing"},
	}
	if !reflect.DeepEqual(report.PortableFiles, want) {
		t.Fatalf("portable files = %#v, want %#v", report.PortableFiles, want)
	}
	if report.LatestSnapshot != (FileStatus{Path: ".struktly/scans/latest.json", Status: "present"}) {
		t.Fatalf("unexpected latest snapshot: %#v", report.LatestSnapshot)
	}
}

func TestValidateLoadsConfig(t *testing.T) {
	root := newGitRepository(t)
	writeFile(t, root, ".struktly/config.json", `{"schema":"struktly/config/v1","context":{"exclude":["vendor/**"]},"checks":{"required":["go test ./..."]}}`)
	writeFile(t, root, ".struktly/tasks/add-timeout.md", validTask())

	report, err := Validate(context.Background(), root)
	if err != nil {
		t.Fatalf("Validate returned error: %v", err)
	}
	if report.Schema != "struktly/validation/v1" || !report.Valid || !report.ConfigDeclared {
		t.Fatalf("unexpected validation metadata: %#v", report)
	}
	if !reflect.DeepEqual(report.Config.Context.Exclude, []string{"vendor/**"}) {
		t.Fatalf("unexpected exclusions: %v", report.Config.Context.Exclude)
	}
	if len(report.Tasks) != 1 || report.Tasks[0].ID != "add-timeout" {
		t.Fatalf("unexpected tasks: %#v", report.Tasks)
	}
}

func TestValidateRejectsMalformedTask(t *testing.T) {
	root := newGitRepository(t)
	writeFile(t, root, ".struktly/tasks/wrong-name.md", validTask())

	_, err := Validate(context.Background(), root)
	if err == nil || !strings.Contains(err.Error(), "must match filename") {
		t.Fatalf("Validate error = %v, want filename mismatch", err)
	}
}

func TestValidateRejectsMalformedConfig(t *testing.T) {
	root := newGitRepository(t)
	writeFile(t, root, ".struktly/config.json", `{"schema":"struktly/config/v1","unknown":true}`)

	_, err := Validate(context.Background(), root)
	if err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("Validate error = %v, want unknown field error", err)
	}
}

func TestDoctorReportsMalformedConfig(t *testing.T) {
	root := newGitRepository(t)
	writeFile(t, root, ".struktly/config.json", `{"schema":"wrong","context":{},"checks":{}}`)

	report, err := Doctor(context.Background(), root)
	if err != nil {
		t.Fatalf("Doctor returned error: %v", err)
	}
	if report.Schema != "struktly/doctor/v1" {
		t.Fatalf("unexpected schema: %q", report.Schema)
	}
	checks := make(map[string]DoctorCheck, len(report.Checks))
	for _, check := range report.Checks {
		checks[check.Name] = check
	}
	if checks["config"].Status != "fail" {
		t.Fatalf("config check = %#v, want failure", checks["config"])
	}
	if len(report.Checks) != 2 {
		t.Fatalf("doctor should report only repository context checks: %#v", report.Checks)
	}
}

func TestInspectCommandsRejectNonGitDirectory(t *testing.T) {
	for name, inspect := range map[string]func(context.Context, string) error{
		"status":   func(ctx context.Context, root string) error { _, err := Status(ctx, root); return err },
		"validate": func(ctx context.Context, root string) error { _, err := Validate(ctx, root); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := inspect(context.Background(), t.TempDir()); err == nil {
				t.Fatal("expected non-Git repository error")
			}
		})
	}
}

// Doctor is the exception: a diagnostic that refuses to run when something is
// wrong cannot report it. Returning early left `git_repository` able only to
// pass, which told a reader nothing. It now reports the failure as a check, and
// HasFailure carries the exit code the command had before.
func TestDoctorReportsANonGitDirectoryAsAFailedCheck(t *testing.T) {
	report, err := Doctor(context.Background(), t.TempDir())
	if err != nil {
		t.Fatalf("Doctor returned error instead of a report: %v", err)
	}
	if len(report.Checks) == 0 || report.Checks[0].Name != "git_repository" {
		t.Fatalf("unexpected checks: %#v", report.Checks)
	}
	if report.Checks[0].Status != "fail" {
		t.Fatalf("git_repository = %q, want fail", report.Checks[0].Status)
	}
	if !report.HasFailure() {
		t.Fatal("HasFailure did not report the failed check")
	}
}

// A failing config check used to exit 0, so a caller branching on the exit code
// never learned about it.
func TestDoctorFailureIsVisibleToCallersBranchingOnExitCode(t *testing.T) {
	root := newGitRepository(t)
	writeFile(t, root, ".struktly/config.json", "{ not json")

	report, err := Doctor(context.Background(), root)
	if err != nil {
		t.Fatalf("Doctor returned error: %v", err)
	}
	if !report.HasFailure() {
		t.Fatalf("invalid config did not register as a failure: %#v", report.Checks)
	}
}

func newGitRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, "README.md", "# Test repository\n")
	runGit(t, root, "init", "--quiet")
	runGit(t, root, "add", "README.md")
	runGit(t, root, "-c", "user.name=Struktly Test", "-c", "user.email=test@struktly.invalid", "-c", "commit.gpgsign=false", "commit", "--quiet", "-m", "initial")
	return root
}

func validTask() string {
	return `---
type: task
schema: struktly/task/v1
id: add-timeout
title: "Add timeout"
status: ready
priority: medium
created: 2026-07-13
agent: unassigned
---

# Add timeout

## Pick up this task

Start here.

## Objective

Add timeout handling.

## Constraints

- Preserve compatibility.

## Required outcomes

- [ ] Tests pass.

## Execution plan

1. Implement it.

## Definition of done

The tests pass.
`
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
}
