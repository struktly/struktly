//go:build windows

package main

import "io"

// intelExecutableSuffix names the bundled binary as Windows installs it.
const intelExecutableSuffix = ".exe"

// handOverToIntel runs the platform's intel binary as a subprocess, because
// Windows has no exec(2) to replace this process with. The observable contract
// is the same: same streams, same environment, same exit code.
func handOverToIntel(path string, args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	return runIntelProcess(path, args, stdin, stdout, stderr)
}
