//go:build !windows

package main

import (
	"io"
	"os"
	"syscall"
)

// intelExecutableSuffix is empty on unix; the bundled binary is plain `intel`.
const intelExecutableSuffix = ""

// handOverToIntel replaces this process with the platform's intel binary, so
// there is no wrapper left to translate signals, buffer streams, or reinterpret
// an exit code. The file descriptors are already the caller's; the writers are
// unused here for that reason and exist only to match the portable fallback.
func handOverToIntel(path string, args []string, _ io.Reader, _, _ io.Writer) error {
	// Exec only returns when the handover failed, and that failure is about the
	// resolved file rather than about anything this CLI attempted, so it
	// carries its own exit code instead of becoming an operational error with a
	// struktly/error/v1 document attached.
	err := syscall.Exec(path, append([]string{path}, args...), os.Environ())
	return exitCodeError{code: intelUnusableExit, message: "run " + path + ": " + err.Error()}
}
