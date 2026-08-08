package context

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func samplePacket() Packet {
	return Packet{
		Schema:     PacketSchema,
		Repository: Repository{Identity: "git:abc", VCS: "git", Root: ".", HeadRevision: "1111111"},
		Items: []PacketItem{{
			Kind: "source", Path: "server/wrap.go", Content: "package server\n",
			ContentHash: "sha256:aaaa", Reason: "task_match",
			OriginalBytes: 15, IncludedBytes: 15,
		}},
		RequiredChecks:  []string{"go test ./..."},
		SuggestedChecks: []string{},
		Exclusions:      []PacketDecision{},
		Truncations:     []PacketDecision{},
		Limits:          DefaultPacketLimits(),
		PacketHash:      "sha256:before",
		Task:            "wrap the server",
	}
}

func TestDiffReportsIdenticalPackets(t *testing.T) {
	packet := samplePacket()
	diff := DiffPackets(packet, packet)
	if !diff.Identical {
		t.Fatal("a packet compared with itself is not identical")
	}
	if len(diff.Items.Added) != 0 || len(diff.Items.Removed) != 0 || len(diff.Items.Changed) != 0 {
		t.Fatalf("unchanged packet reported item differences: %#v", diff.Items)
	}
	if diff.Items.Unchanged != 1 {
		t.Fatalf("unchanged = %d, want 1", diff.Items.Unchanged)
	}
}

func TestDiffReportsWhatMoved(t *testing.T) {
	before := samplePacket()
	after := samplePacket()
	after.PacketHash = "sha256:after"
	after.Repository.HeadRevision = "2222222"
	after.Limits.MaxItems = 20
	after.RequiredChecks = []string{"make test"}
	// One item changed in place, one arrived, one left.
	after.Items[0].ContentHash = "sha256:bbbb"
	after.Items[0].Rendering = declarationRendering
	after.Items = append(after.Items, PacketItem{
		Kind: "source", Path: "server/new.go", Reason: "symbol_match", IncludedBytes: 40,
	})
	before.Items = append(before.Items, PacketItem{
		Kind: "source", Path: "server/gone.go", Reason: "task_match", IncludedBytes: 10,
	})
	after.Exclusions = []PacketDecision{{Path: "build/x.go", Reason: "default_excluded"}}

	diff := DiffPackets(before, after)
	if diff.Identical {
		t.Fatal("packets with different hashes reported identical")
	}
	if len(diff.Repository) != 1 || diff.Repository[0].Field != "head_revision" {
		t.Fatalf("repository changes = %#v", diff.Repository)
	}
	if len(diff.Limits) != 1 || diff.Limits[0].After != "20" {
		t.Fatalf("limit changes = %#v", diff.Limits)
	}
	if len(diff.Items.Added) != 1 || diff.Items.Added[0].Path != "server/new.go" {
		t.Fatalf("added = %#v", diff.Items.Added)
	}
	if len(diff.Items.Removed) != 1 || diff.Items.Removed[0].Path != "server/gone.go" {
		t.Fatalf("removed = %#v", diff.Items.Removed)
	}
	if len(diff.Items.Changed) != 1 || diff.Items.Changed[0].Path != "server/wrap.go" {
		t.Fatalf("changed = %#v", diff.Items.Changed)
	}
	fields := map[string]string{}
	for _, change := range diff.Items.Changed[0].Changes {
		fields[change.Field] = change.After
	}
	if fields["content_hash"] != "sha256:bbbb" || fields["rendering"] != declarationRendering {
		t.Fatalf("changed fields = %#v", fields)
	}
	if len(diff.RequiredChecks.Added) != 1 || len(diff.RequiredChecks.Removed) != 1 {
		t.Fatalf("check changes = %#v", diff.RequiredChecks)
	}
	if len(diff.Exclusions.Added) != 1 || diff.Exclusions.Added[0].Path != "build/x.go" {
		t.Fatalf("exclusion changes = %#v", diff.Exclusions)
	}
}

// A diff names what was selected and why. It must never reproduce content, or
// comparing two packets would disclose what reading either of them would not —
// and a diff is the artifact most likely to be pasted somewhere else.
func TestDiffNeverCarriesFileContent(t *testing.T) {
	const marker = "content-must-not-appear-in-a-diff"
	before := samplePacket()
	after := samplePacket()
	after.PacketHash = "sha256:after"
	after.Items[0].Content = "package server\n// " + marker + "\n"
	after.Items = append(after.Items, PacketItem{
		Kind: "source", Path: "server/new.go", Reason: "symbol_match",
		Content: marker, IncludedBytes: len(marker),
	})

	encoded, err := json.Marshal(DiffPackets(before, after))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), marker) {
		t.Fatalf("diff serialized file content: %s", encoded)
	}
}

func TestLoadPacketRejectsDocumentsThatAreNotPackets(t *testing.T) {
	root := t.TempDir()
	valid := filepath.Join(root, "valid.json")
	data, err := json.Marshal(samplePacket())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(valid, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPacket(valid); err != nil {
		t.Fatalf("LoadPacket rejected a valid packet: %v", err)
	}

	for name, content := range map[string]string{
		"not json":        "{",
		"wrong schema":    `{"schema":"struktly/snapshot/v1"}`,
		"no schema field": `{"task":"x"}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(root, "bad.json")
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := LoadPacket(path)
			if err == nil {
				t.Fatal("LoadPacket accepted a document that is not a packet")
			}
			if !strings.Contains(err.Error(), ErrInvalidPacket.Error()) {
				t.Fatalf("error does not identify the problem: %v", err)
			}
		})
	}

	if _, err := LoadPacket(filepath.Join(root, "absent.json")); err == nil {
		t.Fatal("LoadPacket accepted a missing file")
	}
}
