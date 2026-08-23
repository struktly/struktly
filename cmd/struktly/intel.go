package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
)

// Driving the desktop platform headlessly.
//
// `struktly intel` is a bridge, not a feature. Everything it names — plans,
// approvals, runs, evidence, decisions — belongs to the Struktly desktop
// platform, which is a separate closed product. This CLI keeps its boundary:
// it does not import platform code, does not speak to a platform process over
// HTTP, and does not call a model. It locates the `intel` binary the installed
// desktop app ships beside `struktly-server`, hands the process over to it with
// the arguments and environment untouched, and returns its exit code.
//
// The reason to have the bridge at all is that people already have `struktly`
// on their PATH, and the platform's headless entrypoint lives inside an
// application bundle whose path nobody should have to memorise. The reason to
// have it be nothing but a handover is that anything more — parsing its output,
// re-describing its subcommands, defaulting its flags — would make this
// repository carry a copy of a contract it does not own, which would then rot
// against the product it describes. So the output of `struktly intel` is the
// platform's output verbatim, and this file stays the size it is.

const (
	intelBinaryName = "intel"

	// intelEnvVar names an explicit path to the platform's intel binary and
	// takes precedence over every other resolution step, so a developer running
	// an unbundled build can point the bridge at it.
	intelEnvVar = "STRUKTLY_INTEL"

	// intelMissingExit is distinct from 1 (operational failure) and 2 (invalid
	// invocation) because "the platform is not installed" is neither: the
	// command was well formed and nothing was attempted. A caller can branch on
	// it to decide whether to prompt for an install.
	//
	// 127 rather than a low number because the platform's ladder is 0-4 and 3
	// there means "no daemon at -addr". A caller that could not tell "Struktly
	// is not on this machine" from "Struktly is here and not running" would
	// have no use for either answer. Reserving a low code for this would also
	// make the distinction depend on which build of the app sits beside a CLI
	// that is installed separately from it. 127 is the shell's own code for a
	// command that does not exist, which is exactly this condition, and it is
	// outside the range the platform uses.
	intelMissingExit = 127

	// intelUnusableExit is the shell's code for a file that was found and could
	// not be executed, which is exactly the case a wrong-architecture download,
	// a truncated bundle or a stale STRUKTLY_INTEL produces. It is not 1: that
	// is an ordinary failure of this CLI, and the platform uses it for "the
	// daemon refused".
	intelUnusableExit = 126

	// intelSignalledExitBase is the shell's convention for "killed by signal N":
	// 128+N. Only the subprocess handover can observe this; on unix the child
	// has replaced this process and the signal is its own.
	intelSignalledExitBase = 128

	intelMissingMessage = "Struktly's desktop platform is not installed on this machine, so `struktly intel` has nothing to drive. Install Struktly, or set " + intelEnvVar + " to its intel binary."
)

// intelBridgeHelp says what this repository is responsible for, so
// `struktly intel -h` is useful even on a machine where the platform is absent
// and the real help cannot be shown. It deliberately does not enumerate the
// platform's subcommands: an earlier version did, and was wrong within a day of
// the platform gaining one.
var intelBridgeHelp = fmt.Sprintf(intelBridgeHelpTemplate, intelMissingExit)

const intelBridgeHelpTemplate = `struktly intel drives the headless entrypoint of the Struktly desktop app.

This CLI implements none of it. Every argument and the whole environment are
passed through to the installed platform's ` + intelBinaryName + ` binary, and its exit code is
returned unchanged. Its output belongs to the platform, not to this CLI's
versioned JSON contract.

What it accepts is the platform's to say, and this help does not list it: a
copy of another program's command surface here would be wrong the first time
that program grew one. When the platform is installed, its own help follows
this text.

The binary is resolved as ` + intelEnvVar + `, then ` + intelBinaryName + ` beside this executable
(the app bundle ships struktly, struktly-server, llama-server and ` + intelBinaryName + ` in one
directory), then the app's install location, then ` + intelBinaryName + ` on PATH. When none of
those exist, this command exits %d.

`

// exitCodeError carries an exit code that classifyError cannot derive, because
// it belongs to another program. A pass-through has to be able to return
// intelMissingExit for an absent platform and whatever the child returned
// otherwise; classification by error identity only reaches the codes this CLI
// defines for itself.
type exitCodeError struct {
	code int
	// message is written to stderr as-is. It is empty when the child process
	// already reported its own failure, so the bridge stays silent about it.
	message string
}

func (e exitCodeError) Error() string { return e.message }

func newIntelCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "intel [arguments...]",
		Short: "Drive the installed Struktly desktop platform's headless entrypoint",
		Long:  intelBridgeHelp,
		// The point of the command is that this CLI does not interpret what it
		// is given. Without this, cobra would claim --root, --json-errors and
		// -h for itself and the platform would never see them.
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runIntel(cmd, args)
		},
	}
}

func runIntel(cmd *cobra.Command, args []string) error {
	if intelHelpRequested(args) {
		if _, err := io.WriteString(cmd.OutOrStdout(), intelBridgeHelp); err != nil {
			return err
		}
	}
	path, found := resolveIntelBinary()
	if !found {
		if intelHelpRequested(args) {
			// The help was the whole request and it has been answered as well
			// as this machine can answer it. Failing here would make
			// `--help` exit nonzero, which no caller expects.
			return nil
		}
		return exitCodeError{code: intelMissingExit, message: intelMissingMessage}
	}
	return runIntelBinary(path, args, cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr())
}

func intelHelpRequested(args []string) bool {
	return len(args) == 0 || args[0] == "-h" || args[0] == "--help"
}

// executablePath is a seam for the beside-self resolution step: a test cannot
// move the running test binary into a fixture directory, but it can say where
// the binary should be considered to live.
var executablePath = os.Executable

// resolveIntelBinary answers where the platform's intel binary is, in the order
// an explicit instruction beats an installed app and an installed app beats a
// loose PATH entry — the more certain the evidence that a file really is the
// platform, the earlier it is tried. An explicit STRUKTLY_INTEL that does not
// resolve is a
// failure rather than a reason to keep looking: silently ignoring it would send
// the caller's arguments to a binary they did not name.
func resolveIntelBinary() (string, bool) {
	if explicit := strings.TrimSpace(os.Getenv(intelEnvVar)); explicit != "" {
		if isExecutableFile(explicit) {
			return explicit, true
		}
		return "", false
	}

	if self, err := executablePath(); err == nil {
		// The macOS bundle is commonly reached through a symlink on PATH;
		// resolving it is what makes "beside this executable" mean the
		// directory inside Struktly.app rather than /usr/local/bin.
		if resolved, err := filepath.EvalSymlinks(self); err == nil {
			self = resolved
		}
		beside := filepath.Join(filepath.Dir(self), intelBinaryName+intelExecutableSuffix)
		if isExecutableFile(beside) {
			return beside, true
		}
	}

	// A `struktly` installed on its own — Homebrew, `go install`, a release
	// archive — is not beside the app's binaries and the app bundle is not on
	// anyone's PATH. The platform's install location is known, so look there
	// rather than asking every such user to export a variable.
	//
	// This is deliberately tried before PATH. A file at the install location is
	// the platform by construction; a file named `intel` on PATH is only
	// evidence that something on this machine is called `intel`, and handing it
	// the caller's whole argv and environment on that basis would be wrong.
	for _, dir := range knownInstallDirs() {
		candidate := filepath.Join(dir, intelBinaryName+intelExecutableSuffix)
		if isExecutableFile(candidate) {
			return candidate, true
		}
	}

	// PATH is the last resort, and on an operating system where no install
	// location is known it is the only one an ordinary installation has.
	if onPath, err := exec.LookPath(intelBinaryName); err == nil {
		return onPath, true
	}
	return "", false
}

// knownInstallDirs lists where the installed desktop platform keeps its
// binaries on this operating system. It is a variable so a test can replace
// the list with a fixture directory; the running test binary cannot install
// an application bundle.
var knownInstallDirs = func() []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"/Applications/Struktly.app/Contents/MacOS"}
	default:
		return nil
	}
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode().Perm()&0o111 != 0
}

// runIntelBinary hands this process over to the resolved binary. It is a
// variable because the unix implementation replaces the running program, which
// a test cannot observe from inside it.
var runIntelBinary = handOverToIntel

// runIntelProcess is the handover for platforms without exec(2): the child runs
// as a subprocess on the same streams and its exit code is returned as this
// process's own. It is also what the tests exercise, so the fallback path is
// covered on every platform rather than only where it is the default.
func runIntelProcess(path string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	command := exec.Command(path, args...)
	command.Env = os.Environ()
	command.Stdin = stdin
	command.Stdout = stdout
	command.Stderr = stderr

	err := command.Run()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		if code < 0 {
			// The child was signalled, so it has no code of its own. 128+signal
			// is the shell's convention; 1 would be indistinguishable from the
			// platform's own "the daemon refused".
			code = intelSignalledExitBase
			var status syscall.WaitStatus
			if s, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				status = s
			}
			if status.Signaled() {
				code = intelSignalledExitBase + int(status.Signal())
			}
		}
		return exitCodeError{code: code}
	}
	if err != nil {
		return fmt.Errorf("run %s: %w", path, err)
	}
	return nil
}
