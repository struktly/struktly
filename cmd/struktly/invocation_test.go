package main

import (
	"bytes"
	stdcontext "context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// C1: the exit-code contract promises 2 / invalid_invocation for an invalid
// flag value. Classification searched the error text for markers, and pflag's
// `invalid argument "abc" for "--max-items" flag` matched none of them, so these
// exited 1 as operation_failed.
func TestInvalidFlagValuesUseTheInvocationExitCode(t *testing.T) {
	for name, args := range map[string][]string{
		"non-numeric int": {"context", "--max-items", "abc", "task"},
		"non-boolean":     {"tasks", "--json=bogus"},
		"unknown flag":    {"tasks", "--nope"},
		"unknown command": {"nosuchcommand"},
		"wrong arg count": {"explain"},
		"exclusive flags": {"context", "--stdout", "--json", "task"},
		"no-write alone":  {"context", "--no-write", "task"},
	} {
		t.Run(name, func(t *testing.T) {
			code := runCLI(stdcontext.Background(), append(args, "--json-errors"), strings.NewReader(""), io.Discard, io.Discard)
			if code != 2 {
				t.Fatalf("exit code = %d, want 2", code)
			}
		})
	}
}

func TestInvalidFlagValueReportsInvalidInvocationCode(t *testing.T) {
	var stderr bytes.Buffer
	runCLI(stdcontext.Background(), []string{"context", "--max-items", "abc", "--json-errors", "task"},
		strings.NewReader(""), io.Discard, &stderr)

	var doc errorDocument
	if err := json.Unmarshal(stderr.Bytes(), &doc); err != nil {
		t.Fatalf("stderr is not a structured error: %q", stderr.String())
	}
	if doc.Error.Code != "invalid_invocation" {
		t.Fatalf("error code = %q, want invalid_invocation", doc.Error.Code)
	}
}

// C3: these four commands had no Args validator, so cobra accepted arbitrary
// positional arguments. `struktly scan repo-b` silently scanned the working
// directory instead, while sibling commands exit 2 for the same mistake.
func TestCommandsRejectStrayPositionalArguments(t *testing.T) {
	for _, command := range []string{"init", "scan", "suggest-instructions", "mcp"} {
		t.Run(command, func(t *testing.T) {
			code := runCLI(stdcontext.Background(), []string{command, "stray-argument", "--root", t.TempDir()},
				strings.NewReader(""), io.Discard, io.Discard)
			if code != 2 {
				t.Fatalf("%s accepted a stray positional argument: exit %d", command, code)
			}
		})
	}
}

// C2: init and scan anchored at the literal --root while every Git-backed
// command resolves to the repository top level, so in a monorepo
// `init --root services/api` wrote a config that `context --root services/api`
// never read.
func TestInitAndScanAnchorAtTheGitTopLevel(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "README.md"), "# Monorepo\n")
	writeTestFile(t, filepath.Join(root, "services", "api", "README.md"), "# API\n")
	initTestGitRepo(t, root)

	if code := runCLI(stdcontext.Background(), []string{"init", "--root", filepath.Join(root, "services", "api")},
		strings.NewReader(""), io.Discard, io.Discard); code != 0 {
		t.Fatalf("init exit %d", code)
	}
	if _, err := os.Stat(filepath.Join(root, ".struktly", "config.json")); err != nil {
		t.Fatalf("init did not write the config where Git-backed commands read it: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "services", "api", ".struktly", "config.json")); err == nil {
		t.Fatal("init wrote a config in a subdirectory that no command reads")
	}
}

// C5: a request longer than the message limit failed bufio.Scanner with
// ErrTooLong, which ends the stream. The server exited, the oversize request
// got no JSON-RPC error, and no later request on the connection was answered.
func TestMCPSurvivesAnOversizeRequest(t *testing.T) {
	var in bytes.Buffer
	in.WriteString(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"context_brief","arguments":{"task":"`)
	in.WriteString(strings.Repeat("A", 5*1024*1024))
	in.WriteString("\"}}}\n")
	in.WriteString(`{"jsonrpc":"2.0","id":2,"method":"ping"}` + "\n")

	var out bytes.Buffer
	if code := runCLI(stdcontext.Background(), []string{"mcp", "--root", t.TempDir()}, &in, &out, io.Discard); code != 0 {
		t.Fatalf("mcp exit %d", code)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want two responses, got %d: %q", len(lines), out.String())
	}
	var oversize, ping struct {
		ID    json.RawMessage `json:"id"`
		Error *struct {
			Code int `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &oversize); err != nil {
		t.Fatal(err)
	}
	if oversize.Error == nil || oversize.Error.Code != -32600 {
		t.Fatalf("oversize request did not get a JSON-RPC error: %s", lines[0])
	}
	if err := json.Unmarshal([]byte(lines[1]), &ping); err != nil {
		t.Fatal(err)
	}
	if string(ping.ID) != "2" || ping.Error != nil {
		t.Fatalf("the request after an oversize one was not answered: %s", lines[1])
	}
}

// A config file that cannot be read is an operational failure, not an invalid
// declaration. Classifying by searching the message for the config path could
// not tell the two apart.
func TestUnreadableConfigIsNotClassifiedAsInvalidConfig(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permissions")
	}
	root := t.TempDir()
	writeTestFile(t, filepath.Join(root, "README.md"), "# Fixture\n")
	initTestGitRepo(t, root)
	configPath := filepath.Join(root, ".struktly", "config.json")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(`{"schema":"struktly/config/v1"}`), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(configPath, 0o644) })

	var stderr bytes.Buffer
	code := runCLI(stdcontext.Background(), []string{"validate", "--root", root, "--json-errors"},
		strings.NewReader(""), io.Discard, &stderr)
	if code == 0 {
		t.Skip("filesystem does not enforce the unreadable mode")
	}
	var doc errorDocument
	if err := json.Unmarshal(stderr.Bytes(), &doc); err != nil {
		t.Fatalf("stderr is not a structured error: %q", stderr.String())
	}
	if doc.Error.Code == "invalid_config" {
		t.Fatalf("an unreadable config was reported as an invalid one: %q", doc.Error.Message)
	}
}
