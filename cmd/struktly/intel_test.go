package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
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
	installDir := t.TempDir()
	pretendPlatformInstalledIn(t, installDir)

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
	installed := writeFakeIntel(t, installDir, "intel", "exit 0\n")
	if resolved, found := resolveIntelBinary(); !found || resolved != installed {
		t.Fatalf("the app's install location should beat a loose PATH entry: got %q found=%v", resolved, found)
	}

	// PATH is what an operating system with no known install location has.
	pretendPlatformInstalledIn(t, t.TempDir())
	if resolved, found := resolveIntelBinary(); !found || resolved != onPath {
		t.Fatalf("PATH should still be reached when nothing is installed: got %q found=%v", resolved, found)
	}
}

// A file named `intel` on PATH is not evidence of Struktly. Handing it the
// caller's whole argv and environment because it won a lookup would be a way to
// leak both, so an installed platform must win.
func TestIntelPrefersTheInstalledAppOverAnUnrelatedProgramOnPath(t *testing.T) {
	useProcessHandover(t)
	installDir, pathDir := t.TempDir(), t.TempDir()
	writeFakeIntel(t, installDir, "intel", "echo real-platform\n")
	writeFakeIntel(t, pathDir, "intel", "echo IMPOSTOR \"$*\"\nexit 9\n")

	pretendExecutableLivesIn(t, t.TempDir())
	pretendPlatformInstalledIn(t, installDir)
	t.Setenv(intelEnvVar, "")
	t.Setenv("PATH", pathDir)

	var stdout, stderr bytes.Buffer
	exitCode := runCLI(context.Background(), []string{"intel", "status"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != 0 || !strings.Contains(stdout.String(), "real-platform") {
		t.Fatalf("an unrelated PATH entry was preferred: exit=%d stdout=%q", exitCode, stdout.String())
	}
	if strings.Contains(stdout.String(), "IMPOSTOR") {
		t.Fatalf("the caller's arguments reached an unrelated program:\n%s", stdout.String())
	}
}

// pretendPlatformInstalledIn makes the platform's known install location mean
// a fixture directory. The running test binary cannot install Struktly.app.
func pretendPlatformInstalledIn(t *testing.T, dir string) {
	t.Helper()
	original := knownInstallDirs
	knownInstallDirs = func() []string { return []string{dir} }
	t.Cleanup(func() { knownInstallDirs = original })
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

func TestIntelWithoutPlatformExitsDistinctlyFromThePlatform(t *testing.T) {
	pretendExecutableLivesIn(t, t.TempDir())
	pretendPlatformInstalledIn(t, t.TempDir())
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
	// What the bridge says is its own boundary, never a copy of the platform's
	// command surface: a list here would go stale the first time the platform
	// gained a subcommand, which is what happened.
	for _, ownWords := range []string{"implements none of it", "exit code is\nreturned unchanged", "127"} {
		if !strings.Contains(out, ownWords) {
			t.Fatalf("bridge help does not state %q:\n%s", ownWords, out)
		}
	}
	for _, platformWord := range []string{"approve", "decisions", "evidence", "keep"} {
		if strings.Contains(out, platformWord) {
			t.Fatalf("bridge help enumerates the platform's subcommand %q; it must not:\n%s", platformWord, out)
		}
	}
	// The platform is reached with exactly what the caller typed -- nothing.
	// Substituting -h here would turn the platform's usage error into success.
	if !strings.HasSuffix(out, "platform help: \n") {
		t.Fatalf("bare invocation did not reach the platform unchanged:\n%s", out)
	}
}

// A pass-through that rewrote a bare invocation into `-h` reported success for
// what the platform calls a usage error.
func TestIntelBareInvocationKeepsThePlatformsExitCode(t *testing.T) {
	useProcessHandover(t)
	dir := t.TempDir()
	writeFakeIntel(t, dir, "intel", "echo \"usage\" >&2\nexit 2\n")
	t.Setenv(intelEnvVar, filepath.Join(dir, "intel"))

	var stdout, stderr bytes.Buffer
	if exitCode := runCLI(context.Background(), []string{"intel"}, strings.NewReader(""), &stdout, &stderr); exitCode != 2 {
		t.Fatalf("exit code = %d, want the platform's 2", exitCode)
	}
}

// Help is the whole request when the platform is absent, and it was answered.
func TestIntelHelpSucceedsWithoutThePlatform(t *testing.T) {
	pretendExecutableLivesIn(t, t.TempDir())
	pretendPlatformInstalledIn(t, t.TempDir())
	t.Setenv(intelEnvVar, "")
	t.Setenv("PATH", t.TempDir())

	for _, args := range [][]string{{"intel", "-h"}, {"intel", "--help"}} {
		var stdout, stderr bytes.Buffer
		if exitCode := runCLI(context.Background(), args, strings.NewReader(""), &stdout, &stderr); exitCode != 0 {
			t.Fatalf("%v: exit code = %d, want 0", args, exitCode)
		}
		if !strings.Contains(stdout.String(), "implements none of it") {
			t.Fatalf("%v: bridge help was not printed:\n%s", args, stdout.String())
		}
	}
}

// The number must not be one the platform uses, or a caller cannot tell "not
// on this machine" from an answer the platform gave. A literal in the help
// text drifted from the constant once already.
func TestIntelMissingExitIsOutsideThePlatformLadder(t *testing.T) {
	if intelMissingExit <= 4 {
		t.Fatalf("intelMissingExit = %d collides with the platform's 0-4 ladder", intelMissingExit)
	}
	if intelUnusableExit <= 4 || intelUnusableExit == intelMissingExit {
		t.Fatalf("intelUnusableExit = %d is not a distinct code outside the ladder", intelUnusableExit)
	}
	if !strings.Contains(intelBridgeHelp, fmt.Sprintf("exits %d", intelMissingExit)) {
		t.Fatalf("bridge help does not state the exit code it actually uses:\n%s", intelBridgeHelp)
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

// The unix handover replaces this process, so no in-process test can observe
// it and every other test here swaps it for the subprocess fallback. That left
// the path that actually runs on macOS and Linux covered nowhere. This builds
// the CLI and runs it, which is the only way to exercise syscall.Exec.
func TestIntelUnixHandoverReplacesTheProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("there is no exec(2) to replace this process with")
	}
	if testing.Short() {
		t.Skip("builds the CLI")
	}

	dir := t.TempDir()
	cli := filepath.Join(dir, "struktly")
	build := exec.Command("go", "build", "-o", cli, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build the CLI: %v\n%s", err, out)
	}
	fake := writeFakeIntel(t, dir, "intel", "echo \"argv: $*\"\necho \"marker: $INTEL_TEST_MARKER\"\nexit 4\n")

	run := exec.Command(cli, "intel", "plan", "--json", "a b")
	run.Env = append(os.Environ(), intelEnvVar+"="+fake, "INTEL_TEST_MARKER=carried")
	output, err := run.CombinedOutput()

	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ExitCode() != 4 {
		t.Fatalf("the platform's exit code did not survive the handover: err=%v\n%s", err, output)
	}
	// Quoting must survive: "a b" is one argument, not two.
	if !strings.Contains(string(output), "argv: plan --json a b") {
		t.Fatalf("arguments did not reach the platform verbatim:\n%s", output)
	}
	if !strings.Contains(string(output), "marker: carried") {
		t.Fatalf("the environment did not reach the platform:\n%s", output)
	}
	// An ordinary invocation is the platform's output and nothing else.
	if strings.Contains(string(output), "implements none of it") {
		t.Fatalf("the bridge added its own words to an ordinary invocation:\n%s", output)
	}

	// Help is the one case the bridge writes first. That write must survive the
	// process being replaced, and must come before the platform's own words.
	helpRun := exec.Command(cli, "intel", "-h")
	helpRun.Env = append(os.Environ(), intelEnvVar+"="+fake)
	helpOut, _ := helpRun.CombinedOutput()
	bridge := strings.Index(string(helpOut), "implements none of it")
	platform := strings.Index(string(helpOut), "argv: -h")
	if bridge < 0 || platform < 0 {
		t.Fatalf("help did not carry both voices across exec:\n%s", helpOut)
	}
	if bridge > platform {
		t.Fatalf("the bridge's words arrived after the platform's:\n%s", helpOut)
	}
}

// A resolved file that cannot be executed is not this CLI's operational
// failure, and must not surface as its versioned error document.
func TestIntelUnusableBinaryIsNotAStructuredError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the fixture is not a Windows executable")
	}
	dir := t.TempDir()
	broken := filepath.Join(dir, "intel")
	if err := os.WriteFile(broken, []byte("not a binary\n"), 0o755); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	t.Setenv(intelEnvVar, broken)

	var stdout, stderr bytes.Buffer
	exitCode := runCLI(context.Background(), []string{"intel", "plan", "--json"}, strings.NewReader(""), &stdout, &stderr)
	if exitCode != intelUnusableExit {
		t.Fatalf("exit code = %d, want %d", exitCode, intelUnusableExit)
	}
	if strings.Contains(stderr.String(), "struktly/error/v1") {
		t.Fatalf("the CLI's error schema leaked out of a pass-through:\n%s", stderr.String())
	}
	if !strings.Contains(stderr.String(), broken) {
		t.Fatalf("the failure does not name the binary it could not run:\n%s", stderr.String())
	}
}
