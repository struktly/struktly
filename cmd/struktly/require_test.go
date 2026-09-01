package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRequirements(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "required.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write requirements: %v", err)
	}
	return path
}

// runRequire invokes capabilities with a requirements file and returns
// everything a consumer's gate can observe: the exit code, the document, and
// the structured error.
func runRequire(t *testing.T, path string, args ...string) (int, string, errorDocument) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	invocation := append([]string{"capabilities", "--require", path}, args...)
	exitCode := runCLI(context.Background(), invocation, strings.NewReader(""), &stdout, &stderr)
	var document errorDocument
	if strings.TrimSpace(stderr.String()) != "" {
		if err := json.Unmarshal(stderr.Bytes(), &document); err != nil {
			t.Fatalf("stderr is not a structured error: %v\n%s", err, &stderr)
		}
	}
	return exitCode, stdout.String(), document
}

func TestCapabilitiesRequireAcceptsASatisfiedContract(t *testing.T) {
	// Deliberately a mix of all three categories, and deliberately entries the
	// negotiated contract already holds: a requirements file that asked for
	// something incidental would pass without proving the mechanism reads
	// anything.
	path := writeRequirements(t, `{
	  "schema": "struktly/capability-requirements/v1",
	  "commands": ["context", "tasks archive"],
	  "schemas": ["struktly/packet/v2", "struktly/tasks/v1"],
	  "features": ["context.no_write", "capabilities.require"]
	}`)

	exitCode, stdout, document := runRequire(t, path, "--json")
	if exitCode != 0 {
		t.Fatalf("satisfied requirements exited %d, want 0; error=%+v", exitCode, document)
	}
	var capabilities capabilitiesDocument
	if err := json.Unmarshal([]byte(stdout), &capabilities); err != nil {
		t.Fatalf("capabilities document is not JSON: %v\n%s", err, stdout)
	}
	if capabilities.Schema != capabilitiesSchema {
		t.Fatalf("capabilities schema = %q, want %q", capabilities.Schema, capabilitiesSchema)
	}
}

func TestCapabilitiesRequireNamesEveryMissingEntry(t *testing.T) {
	path := writeRequirements(t, `{
	  "schema": "struktly/capability-requirements/v1",
	  "commands": ["context", "conjure"],
	  "schemas": ["struktly/packet/v2", "struktly/packet/v3"],
	  "features": ["context.no_write", "context.telepathy"]
	}`)

	exitCode, stdout, document := runRequire(t, path, "--json")
	if exitCode != 1 {
		t.Fatalf("unsatisfied requirements exited %d, want 1", exitCode)
	}
	if document.Error.Code != "capabilities_unsatisfied" {
		t.Fatalf("error code = %q, want capabilities_unsatisfied: %+v", document.Error.Code, document)
	}
	// Every missing entry, not the first: a gate answered one entry at a time
	// takes as many bumps to pass as the consumer has requirements.
	for _, want := range []string{`command "conjure"`, `schema "struktly/packet/v3"`, `feature "context.telepathy"`} {
		if !strings.Contains(document.Error.Message, want) {
			t.Errorf("error message omits %s: %s", want, document.Error.Message)
		}
	}
	for _, satisfied := range []string{`"context"`, `"struktly/packet/v2"`, `"context.no_write"`} {
		if strings.Contains(document.Error.Message, satisfied) {
			t.Errorf("error message names the satisfied entry %s: %s", satisfied, document.Error.Message)
		}
	}
	// The document is still the payload. A consumer that failed the gate wants
	// to see what it got, not only what it wanted.
	if !strings.Contains(stdout, capabilitiesSchema) {
		t.Errorf("capabilities document was not written on failure: %s", stdout)
	}
}

func TestCapabilitiesRequireRefusesAMalformedRequirement(t *testing.T) {
	for name, body := range map[string]string{
		"not JSON":            `commands: [context]`,
		"unknown key":         `{"schema": "struktly/capability-requirements/v1", "capabilities": ["context"]}`,
		"no schema":           `{"commands": ["context"]}`,
		"wrong schema":        `{"schema": "struktly/capabilities/v1", "commands": ["context"]}`,
		"wrong element type":  `{"schema": "struktly/capability-requirements/v1", "commands": [7]}`,
		"empty entry":         `{"schema": "struktly/capability-requirements/v1", "commands": [""]}`,
		"requires nothing":    `{"schema": "struktly/capability-requirements/v1"}`,
		"requires empty sets": `{"schema": "struktly/capability-requirements/v1", "commands": [], "features": []}`,
	} {
		t.Run(name, func(t *testing.T) {
			exitCode, stdout, document := runRequire(t, writeRequirements(t, body), "--json-errors")
			if exitCode != 2 {
				t.Fatalf("exit = %d, want 2 (invalid invocation)", exitCode)
			}
			if document.Error.Code != "invalid_invocation" {
				t.Fatalf("error code = %q, want invalid_invocation: %+v", document.Error.Code, document)
			}
			// Nothing on stdout: a question this binary was not properly asked
			// must not come back looking like an answer.
			if stdout != "" {
				t.Fatalf("a malformed requirement produced output: %s", stdout)
			}
		})
	}
}

// A gate reads its path from a variable, and an unset one must not look like a
// build that passed.
func TestCapabilitiesRequireRefusesAnEmptyPath(t *testing.T) {
	exitCode, stdout, document := runRequire(t, "", "--json-errors")
	if exitCode != 2 || document.Error.Code != "invalid_invocation" {
		t.Fatalf("exit = %d, code = %q; want 2 and invalid_invocation", exitCode, document.Error.Code)
	}
	if stdout != "" {
		t.Fatalf("an empty --require produced output: %s", stdout)
	}
}

func TestCapabilitiesRequireRefusesAMissingFile(t *testing.T) {
	exitCode, stdout, document := runRequire(t, filepath.Join(t.TempDir(), "absent.json"), "--json-errors")
	if exitCode != 2 || document.Error.Code != "invalid_invocation" {
		t.Fatalf("exit = %d, code = %q; want 2 and invalid_invocation", exitCode, document.Error.Code)
	}
	if stdout != "" {
		t.Fatalf("a missing requirements file produced output: %s", stdout)
	}
}
