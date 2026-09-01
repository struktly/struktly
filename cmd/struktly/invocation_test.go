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

	"github.com/struktly/struktly/internal/schema"
)

// C1: the exit-code contract promises 2 / invalid_invocation for an invalid
// flag value. Classification searched the error text for markers, and pflag's
// `invalid argument "abc" for "--max-items" flag` matched none of them, so these
// exited 1 as operation_failed.
func TestInvalidFlagValuesUseTheInvocationExitCode(t *testing.T) {
	for name, args := range map[string][]string{
		"non-numeric int":        {"context", "--max-items", "abc", "task"},
		"non-boolean":            {"tasks", "--json=bogus"},
		"unknown flag":           {"tasks", "--nope"},
		"unknown command":        {"nosuchcommand"},
		"wrong arg count":        {"explain"},
		"stray archive argument": {"tasks", "archive", "stray-argument"},
		"complete without an id": {"tasks", "complete"},
		"exclusive flags":        {"context", "--stdout", "--json", "task"},
		"no-write alone":         {"context", "--no-write", "task"},
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

// diff is the only context command needing no repository: a packet is
// self-describing, so the comparison is a pure function of the two documents.
func TestDiffNeedsNoRepository(t *testing.T) {
	dir := t.TempDir()
	packet := filepath.Join(dir, "packet.json")
	var stdout bytes.Buffer
	if code := runCLI(stdcontext.Background(), []string{
		"context", "--json", "--no-write",
		"--root", repoRootForTest(t), "packet selection",
	}, strings.NewReader(""), &stdout, io.Discard); code != 0 {
		t.Skip("cannot generate a packet in this environment")
	}
	if err := os.WriteFile(packet, stdout.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	code := runCLI(stdcontext.Background(), []string{"diff", "--root", dir, packet, packet},
		strings.NewReader(""), &out, io.Discard)
	if code != 0 {
		t.Fatalf("diff exit %d in a non-repository directory", code)
	}
	if !strings.Contains(out.String(), "identical") {
		t.Fatalf("a packet compared with itself is not reported identical: %q", out.String())
	}
}

func TestDiffRejectsADocumentThatIsNotAPacket(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not-a-packet.json")
	if err := os.WriteFile(path, []byte(`{"schema":"struktly/snapshot/v1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	code := runCLI(stdcontext.Background(), []string{"diff", "--json-errors", path, path},
		strings.NewReader(""), io.Discard, &stderr)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	var doc errorDocument
	if err := json.Unmarshal(stderr.Bytes(), &doc); err != nil {
		t.Fatalf("stderr is not a structured error: %q", stderr.String())
	}
	if doc.Error.Code != "invalid_packet" {
		t.Fatalf("error code = %q, want invalid_packet", doc.Error.Code)
	}
}

func repoRootForTest(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// The command-level documents, which internal/context cannot reach: they are
// assembled here and were the ones with no conformance check at all.
func TestCommandDocumentsConformToTheirSchemas(t *testing.T) {
	root := repoRootForTest(t)
	for name, test := range map[string]struct {
		schema string
		args   []string
	}{
		"capabilities": {schema: "capabilities.v1.json", args: []string{"capabilities", "--json"}},
		"version":      {schema: "version.v1.json", args: []string{"version", "--json"}},
		"status":       {schema: "status.v1.json", args: []string{"status", "--json", "--root", root}},
		"validate":     {schema: "validation.v1.json", args: []string{"validate", "--json", "--root", root}},
		"doctor":       {schema: "doctor.v1.json", args: []string{"doctor", "--json", "--root", root}},
		"tasks":        {schema: "tasks.v1.json", args: []string{"tasks", "--json", "--root", root}},
	} {
		t.Run(name, func(t *testing.T) {
			var stdout bytes.Buffer
			if code := runCLI(stdcontext.Background(), test.args, strings.NewReader(""), &stdout, io.Discard); code != 0 {
				t.Fatalf("%v exit %d", test.args, code)
			}
			assertDocumentConforms(t, test.schema, stdout.Bytes())
		})
	}

	t.Run("init", func(t *testing.T) {
		root := t.TempDir()
		writeTestFile(t, filepath.Join(root, "README.md"), "# Repo\n")
		initTestGitRepo(t, root)

		var stdout bytes.Buffer
		if code := runCLI(stdcontext.Background(), []string{"init", "--root", root, "--json"},
			strings.NewReader(""), &stdout, io.Discard); code != 0 {
			t.Fatalf("init --json exit %d", code)
		}
		assertDocumentConforms(t, "init-result.v1.json", stdout.Bytes())
	})

	t.Run("suggest-instructions", func(t *testing.T) {
		root := t.TempDir()
		writeTestFile(t, filepath.Join(root, "README.md"), "# Repo\n")
		initTestGitRepo(t, root)
		if code := runCLI(stdcontext.Background(), []string{"init", "--root", root, "--json"},
			strings.NewReader(""), io.Discard, io.Discard); code != 0 {
			t.Fatalf("init --json exit %d", code)
		}

		var stdout bytes.Buffer
		if code := runCLI(stdcontext.Background(), []string{"suggest-instructions", "--root", root, "--json"},
			strings.NewReader(""), &stdout, io.Discard); code != 0 {
			t.Fatalf("suggest-instructions --json exit %d", code)
		}
		assertDocumentConforms(t, "instruction-suggestions.v1.json", stdout.Bytes())
	})

	// The two lifecycle documents mutate the repository, so they run against
	// throwaway fixtures rather than this checkout.
	t.Run("tasks archive", func(t *testing.T) {
		root := t.TempDir()
		writeTestFile(t, filepath.Join(root, ".struktly", "tasks", "add-timeout.md"), finishedTaskDocument("add-timeout", "Add timeout"))
		var stdout bytes.Buffer
		if code := runCLI(stdcontext.Background(), []string{"tasks", "archive", "--root", root, "--json"},
			strings.NewReader(""), &stdout, io.Discard); code != 0 {
			t.Fatalf("tasks archive --json exit %d", code)
		}
		assertDocumentConforms(t, "task-archive.v1.json", stdout.Bytes())
	})

	t.Run("tasks complete", func(t *testing.T) {
		root := t.TempDir()
		writeTestFile(t, filepath.Join(root, ".struktly", "tasks", "add-timeout.md"), taskDocument("add-timeout", "Add timeout"))
		var stdout bytes.Buffer
		if code := runCLI(stdcontext.Background(), []string{"tasks", "complete", "add-timeout", "--root", root, "--json"},
			strings.NewReader(""), &stdout, io.Discard); code != 0 {
			t.Fatalf("tasks complete --json exit %d", code)
		}
		assertDocumentConforms(t, "task-transition.v1.json", stdout.Bytes())
	})

	t.Run("error", func(t *testing.T) {
		var stderr bytes.Buffer
		runCLI(stdcontext.Background(), []string{"context", "--json-errors", "--root", t.TempDir(), "x"},
			strings.NewReader(""), io.Discard, &stderr)
		assertDocumentConforms(t, "error.v1.json", stderr.Bytes())
	})

	// Both outcomes, because the report is written either way and the shape
	// must not depend on which one it carries.
	t.Run("verify", func(t *testing.T) {
		sealed := `{"schema":"struktly/provenance/v1","execution_id":"run_abc","revision":2}`
		for name, digest := range map[string]string{
			"intact":   sha256Hex(sealed),
			"tampered": strings.Repeat("0", 64),
		} {
			t.Run(name, func(t *testing.T) {
				var stdout bytes.Buffer
				runCLI(stdcontext.Background(), []string{"verify", bundleFor(t, sealed, digest), "--json"},
					strings.NewReader(""), &stdout, io.Discard)
				assertDocumentConforms(t, "record-verification.v1.json", stdout.Bytes())
			})
		}
	})
}

func assertDocumentConforms(t *testing.T, schemaName string, document []byte) {
	t.Helper()
	definition, err := os.ReadFile(filepath.Join("..", "..", "schemas", schemaName))
	if err != nil {
		t.Fatalf("read schema %s: %v", schemaName, err)
	}
	if err := schema.ValidateJSON(definition, document); err != nil {
		t.Fatalf("output does not conform to %s: %v", schemaName, err)
	}
}
