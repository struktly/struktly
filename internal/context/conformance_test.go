package context

import (
	stdcontext "context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/struktly/struktly/internal/schema"
)

func readSchemaBytes(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "schemas", name))
	if err != nil {
		t.Fatalf("read schema %s: %v", name, err)
	}
	return data
}

func assertConforms(t *testing.T, schemaName string, document any) {
	t.Helper()
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("encode %T: %v", document, err)
	}
	if err := schema.ValidateJSON(readSchemaBytes(t, schemaName), encoded); err != nil {
		t.Fatalf("%s does not conform to %s: %v", schemaName, schemaName, err)
	}
}

// Every schema in schemas/ must be one this checker fully understands. If a
// schema grows a keyword internal/schema does not implement, this fails rather
// than the checker quietly ignoring it and reporting conformance it never
// established.
func TestEveryShippedSchemaIsEnforceable(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("..", "..", "schemas"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no schemas found")
	}
	for _, entry := range entries {
		t.Run(entry.Name(), func(t *testing.T) {
			// Validating an empty object exercises every keyword in the schema
			// tree that applies before any value is examined.
			err := schema.ValidateJSON(readSchemaBytes(t, entry.Name()), []byte(`{}`))
			if err != nil && containsUnsupported(err.Error()) {
				t.Fatalf("%s uses a keyword internal/schema cannot enforce: %v", entry.Name(), err)
			}
		})
	}
}

// containsUnsupported distinguishes "this schema uses something the checker
// cannot enforce" from an ordinary validation failure against a probe document.
func containsUnsupported(message string) bool {
	for _, marker := range []string{"unsupported keyword", "only local", "unsupported format"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

// The check that would have caught `provenance.confidence` emitting a value the
// enum did not list. Real output, real schemas, in the ordinary test run.
func TestEmittedDocumentsConformToTheirSchemas(t *testing.T) {
	root := initSelectionRepo(t)
	writeFile(t, root, "AGENTS.md", "# Rules\n")
	writeFile(t, root, "docs/architecture.md", "# Architecture\n")
	writeFile(t, root, "server/wrap.go", "package server\n\nfunc WithTimeout() {}\n")
	writeFile(t, root, ".struktly/tasks/add-timeout.md", validTaskDocument)

	t.Run("packet", func(t *testing.T) {
		result, err := Brief(BriefOptions{
			Root: root, Task: "add request timeout middleware", NoWrite: true,
			Seeds: []string{"docs/architecture.md"},
		})
		if err != nil {
			t.Fatalf("Brief returned error: %v", err)
		}
		assertConforms(t, "packet.v2.json", result.Packet)
	})

	t.Run("snapshot", func(t *testing.T) {
		result, err := Scan(ScanOptions{Root: root, NoWrite: true, Now: time.Unix(0, 0).UTC()})
		if err != nil {
			t.Fatalf("Scan returned error: %v", err)
		}
		assertConforms(t, "snapshot.v1.json", result.Snapshot)
	})

	t.Run("tasks", func(t *testing.T) {
		document, err := DiscoverTasks(root)
		if err != nil {
			t.Fatalf("DiscoverTasks returned error: %v", err)
		}
		assertConforms(t, "tasks.v1.json", document)
	})

	t.Run("explanation", func(t *testing.T) {
		explanation, err := ExplainSelection(stdcontext.Background(), root, "server/wrap.go", "add request timeout", "")
		if err != nil {
			t.Fatalf("ExplainSelection returned error: %v", err)
		}
		assertConforms(t, "explanation.v1.json", explanation)
	})

	t.Run("packet diff", func(t *testing.T) {
		before, err := Brief(BriefOptions{Root: root, Task: "add request timeout", NoWrite: true})
		if err != nil {
			t.Fatalf("Brief returned error: %v", err)
		}
		after, err := Brief(BriefOptions{Root: root, Task: "add request timeout", NoWrite: true, MaxItems: 2})
		if err != nil {
			t.Fatalf("Brief returned error: %v", err)
		}
		assertConforms(t, "packet-diff.v1.json", DiffPackets(before.Packet, after.Packet))
	})

	t.Run("config", func(t *testing.T) {
		assertConforms(t, "config.v1.json", DefaultConfig())
	})
}
