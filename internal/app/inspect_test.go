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

func TestDoctorReportsMalformedConfigAndLegacyRuntimeWarning(t *testing.T) {
	root := newGitRepository(t)
	writeFile(t, root, ".struktly/config.json", `{"schema":"wrong","context":{},"checks":{}}`)
	if err := os.MkdirAll(filepath.Join(root, ".struktly", "runs"), 0o755); err != nil {
		t.Fatal(err)
	}

	report, err := Doctor(context.Background(), root)
	if err != nil {
		t.Fatalf("Doctor returned error: %v", err)
	}
	if report.Schema != "struktly/doctor/v1" {
		t.Fatalf("unexpected schema: %q", report.Schema)
	}
	wantNames := []string{".struktly/memory/candidates", ".struktly/runs", "config", "git_repository"}
	for i, want := range wantNames {
		if report.Checks[i].Name != want {
			t.Fatalf("checks are not sorted: %#v", report.Checks)
		}
	}
	checks := make(map[string]DoctorCheck, len(report.Checks))
	for _, check := range report.Checks {
		checks[check.Name] = check
	}
	if checks["config"].Status != "fail" {
		t.Fatalf("config check = %#v, want failure", checks["config"])
	}
	if checks[".struktly/runs"].Status != "warning" {
		t.Fatalf("legacy runs check = %#v, want warning", checks[".struktly/runs"])
	}
	if checks[".struktly/memory/candidates"].Status != "pass" {
		t.Fatalf("absent legacy memory check = %#v, want pass", checks[".struktly/memory/candidates"])
	}
}

func TestInspectCommandsRejectNonGitDirectory(t *testing.T) {
	for name, inspect := range map[string]func(context.Context, string) error{
		"status":   func(ctx context.Context, root string) error { _, err := Status(ctx, root); return err },
		"validate": func(ctx context.Context, root string) error { _, err := Validate(ctx, root); return err },
		"doctor":   func(ctx context.Context, root string) error { _, err := Doctor(ctx, root); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := inspect(context.Background(), t.TempDir()); err == nil {
				t.Fatal("expected non-Git repository error")
			}
		})
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
priority: normal
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
