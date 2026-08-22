package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeFakeIntel writes an executable that reports how it was invoked, so a
// test can assert on the arguments the bridge actually handed over rather than
// on the ones it intended to.
func writeFakeIntel(t *testing.T, dir, name, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake platform binary is a shell script")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatalf("write fake intel: %v", err)
	}
	return path
}

// useProcessHandover swaps the unix exec(2) handover, which would replace the
// test binary itself, for the portable subprocess handover that Windows uses.
// The resolution and argument handling under test are the same either way.
func useProcessHandover(t *testing.T) {
	t.Helper()
	original := runIntelBinary
	runIntelBinary = runIntelProcess
	t.Cleanup(func() { runIntelBinary = original })
}

// pretendExecutableLivesIn makes "beside this executable" mean a fixture
// directory. The running test binary cannot be moved into one.
func pretendExecutableLivesIn(t *testing.T, dir string) {
	t.Helper()
	original := executablePath
	executablePath = func() (string, error) { return filepath.Join(dir, "struktly"), nil }
	t.Cleanup(func() { executablePath = original })
}

func TestIntelResolutionOrder(t *testing.T) {
	explicitDir := t.TempDir()
	besideDir := t.TempDir()
	pathDir := t.TempDir()

	explicit := writeFakeIntel(t, explicitDir, "explicit-intel", "exit 0\n")
	beside := writeFakeIntel(t, besideDir, "intel", "exit 0\n")
	onPath := writeFakeIntel(t, pathDir, "intel", "exit 0\n")

	pretendExecutableLivesIn(t, besideDir)
	t.Setenv("PATH", pathDir)

	t.Setenv(intelEnvVar, explicit)
	if resolved, found := resolveIntelBinary(); !found || resolved != explicit {
		t.Fatalf("%s should win: got %q found=%v", intelEnvVar, resolved, found)
	}

	t.Setenv(intelEnvVar, "")
	if resolved, found := resolveIntelBinary(); !found || resolved != beside {
		t.Fatalf("binary beside struktly should win over PATH: got %q found=%v", resolved, found)
	}

	pretendExecutableLivesIn(t, t.TempDir())
	if resolved, found := resolveIntelBinary(); !found || resolved != onPath {
		t.Fatalf("PATH should be the last resort: got %q found=%v", resolved, found)
	}
}

func TestIntelExplicitPathIsNotSecondGuessed(t *testing.T) {
	besideDir := t.TempDir()
	writeFakeIntel(t, besideDir, "intel", "exit 0\n")
	pretendExecutableLivesIn(t, besideDir)
	t.Setenv(intelEnvVar, filepath.Join(t.TempDir(), "absent"))

	if resolved, found := resolveIntelBinary(); found {
		t.Fatalf("an unusable %s must not fall back to %q", intelEnvVar, resolved)
	}
}

func TestIntelPassesArgumentsAndEnvironmentVerbatim(t *testing.T) {
	useProcessHandover(t)
	dir := t.TempDir()
	writeFakeIntel(t, dir, "intel", "for arg in \"$@\"; do echo \"arg=$arg\"; done\necho \"env=$STRUKTLY_TEST_MARKER\"\n")
	pretendExecutableLivesIn(t, dir)
	t.Setenv(intelEnvVar, "")
	t.Setenv("PATH", t.TempDir())
	t.Setenv("STRUKTLY_TEST_MARKER", "carried")

	var stdout, stderr bytes.Buffer
	args := []string{"intel", "plan", "--json", "--root", "/elsewhere", "-h", "a b"}
	exitCode := runCLI(context.Background(), args, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", exitCode, &stderr)
	}

	want := "arg=plan\narg=--json\narg=--root\narg=/elsewhere\narg=-h\narg=a b\nenv=carried\n"
	if stdout.String() != want {
		t.Fatalf("handover altered the invocation:\ngot  %q\nwant %q", stdout.String(), want)
	}
}

func TestIntelPropagatesExitCode(t *testing.T) {
	useProcessHandover(t)
	dir := t.TempDir()
	writeFakeIntel(t, dir, "intel", "echo 'platform said no' >&2\nexit 7\n")
	t.Setenv(intelEnvVar, filepath.Join(dir, "intel"))

	var stdout, stderr bytes.Buffer
	exitCode := runCLI(context.Background(), []string{"intel", "run"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 7 {
		t.Fatalf("exit code = %d, want 7", exitCode)
	}
	if stderr.String() != "platform said no\n" {
		t.Fatalf("the bridge added to the platform's stderr: %q", stderr.String())
	}
}

func TestIntelWithoutPlatformExitsThree(t *testing.T) {
	pretendExecutableLivesIn(t, t.TempDir())
	t.Setenv(intelEnvVar, "")
	t.Setenv("PATH", t.TempDir())

	// --json is one of the platform's own flags here, so the absence of the
	// platform must not be reported as this CLI's structured error document.
	for _, args := range [][]string{{"intel", "status"}, {"intel", "status", "--json"}} {
		var stdout, stderr bytes.Buffer
		exitCode := runCLI(context.Background(), args, strings.NewReader(""), &stdout, &stderr)
		if exitCode != intelMissingExit {
			t.Fatalf("%v: exit code = %d, want %d", args, exitCode, intelMissingExit)
		}
		if stderr.String() != intelMissingMessage+"\n" {
			t.Fatalf("%v: stderr = %q", args, stderr.String())
		}
		if stdout.Len() != 0 {
			t.Fatalf("%v: stdout = %q, want empty", args, stdout.String())
		}
	}
}

func TestIntelHelpNamesTheBridgeThenAsksThePlatform(t *testing.T) {
	useProcessHandover(t)
	dir := t.TempDir()
	writeFakeIntel(t, dir, "intel", "echo \"platform help: $*\"\n")
	t.Setenv(intelEnvVar, filepath.Join(dir, "intel"))

	var stdout, stderr bytes.Buffer
	exitCode := runCLI(context.Background(), []string{"intel"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, want 0; stderr=%s", exitCode, &stderr)
	}
	out := stdout.String()
	for _, subcommand := range []string{"plan", "approve", "graph", "run", "decisions", "evidence", "record"} {
		if !strings.Contains(out, subcommand) {
			t.Fatalf("bridge help does not name %q:\n%s", subcommand, out)
		}
	}
	if !strings.HasSuffix(out, "platform help: -h\n") {
		t.Fatalf("bare invocation did not ask the platform for its help:\n%s", out)
	}
}

// The bridge is deliberately absent from the versioned machine contract: it
// carries no schema and describes another program's output.
func TestIntelIsNotAdvertisedAsAMachineCommand(t *testing.T) {
	for _, command := range currentCapabilities().Commands {
		if command == "intel" {
			t.Fatal("capabilities advertise intel as a stable machine command")
		}
	}
}
