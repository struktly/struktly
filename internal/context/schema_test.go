package context

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type schemaDocument struct {
	ID                   string                      `json:"$id"`
	Dialect              string                      `json:"$schema"`
	Properties           map[string]json.RawMessage  `json:"properties"`
	Required             []string                    `json:"required"`
	AdditionalProperties bool                        `json:"additionalProperties"`
	Defs                 map[string]schemaDefinition `json:"$defs"`
}

type schemaDefinition struct {
	Properties map[string]json.RawMessage `json:"properties"`
	Required   []string                   `json:"required"`
}

func TestSnapshotSchema(t *testing.T) {
	doc := readSchema(t, "snapshot.v1.json")
	value := Snapshot{
		Schema:      SnapshotSchema,
		GeneratedAt: time.Unix(0, 0).UTC(),
		Repository:  RepositoryIdent{Name: "example", Root: "."},
		TopDirs:     []SnapshotItem{}, Languages: []SnapshotItem{}, Commands: []SnapshotItem{},
		Docs: []SnapshotItem{}, ADRs: []SnapshotItem{}, InstructionFiles: []SnapshotItem{},
		Stats: SnapshotStats{},
	}
	assertSchemaMatchesValue(t, doc, SnapshotSchema, value)
}

func TestPacketSchema(t *testing.T) {
	doc := readSchema(t, "packet.v1.json")
	value := Packet{
		Schema:      PacketSchema,
		GeneratedAt: time.Unix(0, 0).UTC(),
		Metadata: PacketMetadata{
			GeneratedAt:     time.Unix(0, 0).UTC().Format(time.RFC3339),
			AbsoluteGitRoot: "/repo",
		},
		Repository: Repository{Name: "example", Identity: "git:example", VCS: "git", Root: ".", HeadRevision: "abc"},
		Items: []PacketItem{{
			Kind: "instruction", Path: "AGENTS.md", Content: "instructions",
			ContentHash: "sha256:7b7d4e97923c3d3c63b3762e132fd0d9f04f6d6c3b33a1c641ca95e400c7b27e",
			Provenance:  Provenance{Source: "AGENTS.md", Method: "instruction", Confidence: "detected"},
			Reason:      "instruction", OriginalBytes: 12, IncludedBytes: 12,
		}},
		RequiredChecks: []string{}, SuggestedChecks: []string{}, Exclusions: []PacketDecision{},
		Truncations: []PacketDecision{}, Limits: PacketLimits{MaxItems: 40, MaxFileBytes: 65536, MaxTotalBytes: 524288},
		PacketHash: "sha256:7b7d4e97923c3d3c63b3762e132fd0d9f04f6d6c3b33a1c641ca95e400c7b27e",
		Task:       "test", VerificationCommands: []string{}, SuggestedFiles: []string{}, SourceRefs: []string{},
	}
	assertSchemaMatchesValue(t, doc, PacketSchema, value)
	assertRequired(t, doc.Defs["item"].Required, "content_hash", "provenance")
	assertRequired(t, doc.Required, "packet_hash", "limits")
}

func TestConfigSchema(t *testing.T) {
	doc := readSchema(t, "config.v1.json")
	if doc.ID != ConfigSchema || doc.AdditionalProperties {
		t.Fatalf("config schema must be strict and use %q: %+v", ConfigSchema, doc)
	}
	data, err := json.Marshal(DefaultConfig())
	if err != nil || !json.Valid(data) {
		t.Fatalf("default config is not valid JSON: %v", err)
	}
}

func TestCommandSchemas(t *testing.T) {
	for _, name := range []string{"capabilities", "error", "status", "validation", "doctor", "explanation"} {
		doc := readSchema(t, name+".v1.json")
		if doc.ID != "struktly/"+name+"/v1" || len(doc.Required) == 0 || !doc.AdditionalProperties {
			t.Fatalf("invalid %s schema metadata: %+v", name, doc)
		}
	}
}

func TestPacketV1BackwardCompatibility(t *testing.T) {
	legacy := []byte(`{
  "schema": "struktly/packet/v1",
  "generated_at": "2026-07-11T12:00:00Z",
  "task": "legacy task",
  "verification_commands": [],
  "suggested_files": [],
  "source_refs": []
}`)
	var packet Packet
	if err := json.Unmarshal(legacy, &packet); err != nil {
		t.Fatalf("unmarshal legacy packet: %v", err)
	}
	if packet.Schema != PacketSchema || packet.Task != "legacy task" {
		t.Fatalf("unexpected legacy packet: schema=%q task=%q", packet.Schema, packet.Task)
	}
}

func TestCurrentPacketRequiredCollectionsAreArrays(t *testing.T) {
	root := initSelectionRepo(t)
	packet, err := Brief(BriefOptions{Root: root, Task: "inspect repository"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(packet.Packet)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"items", "required_checks", "suggested_checks", "exclusions", "truncations"} {
		if len(object[field]) == 0 || object[field][0] != '[' {
			t.Fatalf("%s must encode as an array, got %s", field, object[field])
		}
	}
}

func readSchema(t *testing.T, name string) schemaDocument {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "schemas", name))
	if err != nil {
		t.Fatal(err)
	}
	var doc schemaDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return doc
}

func assertSchemaMatchesValue(t *testing.T, doc schemaDocument, wantID string, value any) {
	t.Helper()
	if doc.Dialect != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("schema dialect = %q", doc.Dialect)
	}
	if doc.ID != wantID {
		t.Fatalf("schema $id = %q, want %q", doc.ID, wantID)
	}
	if !doc.AdditionalProperties {
		t.Fatal("v1 schema must allow additive properties")
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		t.Fatal(err)
	}
	var schema string
	if err := json.Unmarshal(object["schema"], &schema); err != nil {
		t.Fatal(err)
	}
	if schema != wantID {
		t.Fatalf("value schema = %q, want %q", schema, wantID)
	}
	for _, key := range doc.Required {
		if _, ok := doc.Properties[key]; !ok {
			t.Errorf("required property %q is not defined", key)
		}
		if _, ok := object[key]; !ok {
			t.Errorf("representative value is missing required property %q", key)
		}
	}
}

func assertRequired(t *testing.T, required []string, keys ...string) {
	t.Helper()
	set := make(map[string]bool, len(required))
	for _, key := range required {
		set[key] = true
	}
	for _, key := range keys {
		if !set[key] {
			t.Errorf("%q is not required", key)
		}
	}
}
