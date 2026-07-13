package context

import (
	stdcontext "context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestSelectionHonorsNestedAnchoredGitIgnore(t *testing.T) {
	root := initSelectionRepo(t)
	writeFile(t, root, "nested/.gitignore", "/private.md\n")
	writeFile(t, root, "nested/private.md", "must stay ignored\n")
	runGit(t, root, "add", "nested/.gitignore")
	runGit(t, root, "commit", "-qm", "add nested ignore")

	selection, err := selectPacketContext(stdcontext.Background(), root, "inspect private docs", nil)
	if err != nil {
		t.Fatalf("selectPacketContext returned error: %v", err)
	}
	assertItemAbsent(t, selection.items, "nested/private.md")

	explanation, err := ExplainSelection(stdcontext.Background(), root, "nested/private.md", "inspect private docs")
	if err != nil {
		t.Fatalf("ExplainSelection returned error: %v", err)
	}
	if explanation.Decision != "excluded" || explanation.Reason != "git_ignored" {
		t.Fatalf("unexpected explanation: %+v", explanation)
	}
	brief, err := Brief(BriefOptions{Root: root, Task: "inspect private docs"})
	if err != nil {
		t.Fatalf("Brief returned error: %v", err)
	}
	if containsString(brief.Packet.SuggestedFiles, "nested/private.md") {
		t.Fatalf("ignored path leaked into suggestions: %v", brief.Packet.SuggestedFiles)
	}
}

func TestSelectionExcludesSecretContentWithoutSerializingIt(t *testing.T) {
	root := initSelectionRepo(t)
	const secret = "private-material-must-not-leak"
	writeFile(t, root, "src/auth.go", "-----BEGIN PRIVATE KEY-----\n"+secret+"\n")

	selection, err := selectPacketContext(stdcontext.Background(), root, "review auth", nil)
	if err != nil {
		t.Fatalf("selectPacketContext returned error: %v", err)
	}
	assertDecision(t, selection.exclusions, "src/auth.go", "secret_detected")
	assertItemAbsent(t, selection.items, "src/auth.go")

	encoded, err := json.Marshal(struct {
		Items      []PacketItem     `json:"items"`
		Exclusions []PacketDecision `json:"exclusions"`
	}{selection.items, selection.exclusions})
	if err != nil {
		t.Fatalf("marshal selection: %v", err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("serialized selection leaked secret content: %s", encoded)
	}
}

func TestBriefDoesNotLeakExcludedPortableSecret(t *testing.T) {
	root := initSelectionRepo(t)
	const secret = "private-material-must-not-leak"
	writeFile(t, root, ".struktly/project-context.md", "# Project Context\n")
	writeFile(t, root, ".struktly/constraints.md", "-----BEGIN PRIVATE KEY-----\n"+secret+"\n")

	result, err := Brief(BriefOptions{Root: root, Task: "review constraints"})
	if err != nil {
		t.Fatalf("Brief returned error: %v", err)
	}
	if result.Packet.Constraints != "" {
		t.Fatalf("excluded constraints leaked into legacy packet field: %q", result.Packet.Constraints)
	}
	assertDecision(t, result.Packet.Exclusions, ".struktly/constraints.md", "secret_detected")
	for _, outputPath := range []string{result.OutputPath, result.PacketPath} {
		data, err := os.ReadFile(outputPath)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), secret) {
			t.Fatalf("%s leaked excluded secret", outputPath)
		}
	}
}

func TestSelectionExcludesNULBinary(t *testing.T) {
	root := initSelectionRepo(t)
	writeFile(t, root, "binary.dat", "text\x00binary")

	selection, err := selectPacketContext(stdcontext.Background(), root, "inspect binary", nil)
	if err != nil {
		t.Fatalf("selectPacketContext returned error: %v", err)
	}
	assertDecision(t, selection.exclusions, "binary.dat", "binary")
	assertItemAbsent(t, selection.items, "binary.dat")
}

func TestSelectionExcludesProviderSessionState(t *testing.T) {
	root := initSelectionRepo(t)
	writeFile(t, root, ".codex/sessions/session.json", `{"transcript":"private"}`)

	explanation, err := ExplainSelection(stdcontext.Background(), root, ".codex/sessions/session.json", "inspect session")
	if err != nil {
		t.Fatal(err)
	}
	if explanation.Decision != "excluded" || explanation.Reason != "default_excluded" {
		t.Fatalf("unexpected provider-session explanation: %+v", explanation)
	}
}

func TestSelectionClassifiesMatchingPortableTask(t *testing.T) {
	root := initSelectionRepo(t)
	writeFile(t, root, ".struktly/tasks/add-timeout.md", validTaskDocument)

	selection, err := selectPacketContext(stdcontext.Background(), root, "add timeout", nil)
	if err != nil {
		t.Fatal(err)
	}
	item := requireItem(t, selection.items, ".struktly/tasks/add-timeout.md")
	if item.Kind != "task" || item.Reason != "task_match" {
		t.Fatalf("unexpected task item: %#v", item)
	}
}

func TestSelectionExcludesSymlink(t *testing.T) {
	root := initSelectionRepo(t)
	if err := os.Symlink("README.md", filepath.Join(root, "linked.md")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	selection, err := selectPacketContext(stdcontext.Background(), root, "inspect linked docs", nil)
	if err != nil {
		t.Fatalf("selectPacketContext returned error: %v", err)
	}
	assertDecision(t, selection.exclusions, "linked.md", "symlink")
	assertItemAbsent(t, selection.items, "linked.md")
}

func TestSelectionTruncatesOversizedUTF8AtValidBoundary(t *testing.T) {
	root := initSelectionRepo(t)
	content := strings.Repeat("a", maxPacketFileBytes-1) + "€" + "tail"
	writeFile(t, root, "oversized.txt", content)

	selection, err := selectPacketContext(stdcontext.Background(), root, "inspect oversized file", nil)
	if err != nil {
		t.Fatalf("selectPacketContext returned error: %v", err)
	}
	item := requireItem(t, selection.items, "oversized.txt")
	if !item.Truncated || item.OriginalBytes != int64(len(content)) || item.IncludedBytes != maxPacketFileBytes-1 {
		t.Fatalf("unexpected byte accounting: original=%d included=%d truncated=%v", item.OriginalBytes, item.IncludedBytes, item.Truncated)
	}
	if len(item.Content) != item.IncludedBytes || !utf8.ValidString(item.Content) {
		t.Fatalf("truncated content is not valid UTF-8: bytes=%d valid=%v", len(item.Content), utf8.ValidString(item.Content))
	}
	wantDigest := sha256.Sum256([]byte(content))
	wantHash := "sha256:" + hex.EncodeToString(wantDigest[:])
	if item.ContentHash != wantHash {
		t.Fatalf("content hash = %q, want %q", item.ContentHash, wantHash)
	}
	assertDecision(t, selection.truncations, "oversized.txt", "content_limit")
}

func TestPacketHashIgnoresGenerationTimeAndTracksSelectedContent(t *testing.T) {
	root := initSelectionRepo(t)
	writeFile(t, root, ".struktly/project-context.md", "# Project Context\n")
	task := "review README context"

	first, err := Brief(BriefOptions{Root: root, Task: task, Now: time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("first Brief returned error: %v", err)
	}
	second, err := Brief(BriefOptions{Root: root, Task: task, Now: time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("second Brief returned error: %v", err)
	}
	if first.Packet.PacketHash != second.Packet.PacketHash {
		t.Fatalf("packet hash changed with generation time: %q != %q", first.Packet.PacketHash, second.Packet.PacketHash)
	}

	writeFile(t, root, "README.md", "# Changed repository context\n")
	changed, err := Brief(BriefOptions{Root: root, Task: task, Now: time.Date(2026, 7, 15, 10, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("changed Brief returned error: %v", err)
	}
	if changed.Packet.PacketHash == first.Packet.PacketHash {
		t.Fatalf("packet hash did not change after selected content changed: %q", changed.Packet.PacketHash)
	}
}

func TestPacketHashIsPortableAcrossEquivalentCheckouts(t *testing.T) {
	root := initSelectionRepo(t)
	clone := filepath.Join(t.TempDir(), "different-name")
	cmd := exec.Command("git", "clone", "-q", root, clone)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("clone fixture: %v\n%s", err, output)
	}

	first, err := Brief(BriefOptions{Root: root, Task: "review repository"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Brief(BriefOptions{Root: clone, Task: "review repository"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Packet.PacketHash != second.Packet.PacketHash {
		t.Fatalf("equivalent checkout hashes differ: %s != %s", first.Packet.PacketHash, second.Packet.PacketHash)
	}
}

func TestPacketHashTracksDeterministicCompatibilityFields(t *testing.T) {
	packet := Packet{
		Schema:               PacketSchema,
		Repository:           Repository{Identity: "git:repo", VCS: "git", Root: ".", HeadRevision: "abc", BaseRevision: "abc"},
		Items:                []PacketItem{},
		RequiredChecks:       []string{},
		SuggestedChecks:      []string{},
		Exclusions:           []PacketDecision{},
		Truncations:          []PacketDecision{},
		VerificationCommands: []string{},
		SuggestedFiles:       []string{"README.md"},
		SourceRefs:           []string{"README.md"},
	}
	if err := packet.setHash(); err != nil {
		t.Fatal(err)
	}
	first := packet.PacketHash
	packet.SuggestedFiles = append(packet.SuggestedFiles, "docs/architecture.md")
	if err := packet.setHash(); err != nil {
		t.Fatal(err)
	}
	if packet.PacketHash == first {
		t.Fatal("packet hash did not change with deterministic packet content")
	}
}

func initSelectionRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "-q", "-b", "main")
	runGit(t, root, "config", "user.email", "test@example.com")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "commit.gpgsign", "false")
	writeFile(t, root, "README.md", "# Repository\n")
	runGit(t, root, "add", "README.md")
	runGit(t, root, "commit", "-qm", "initial")
	return root
}

func assertDecision(t *testing.T, decisions []PacketDecision, path, reason string) {
	t.Helper()
	for _, decision := range decisions {
		if decision.Path == path {
			if decision.Reason != reason {
				t.Fatalf("decision for %s = %q, want %q", path, decision.Reason, reason)
			}
			return
		}
	}
	t.Fatalf("no decision for %s in %+v", path, decisions)
}

func assertItemAbsent(t *testing.T, items []PacketItem, path string) {
	t.Helper()
	for _, item := range items {
		if item.Path == path {
			t.Fatalf("unexpected selected item %s", path)
		}
	}
}

func requireItem(t *testing.T, items []PacketItem, path string) PacketItem {
	t.Helper()
	for _, item := range items {
		if item.Path == path {
			return item
		}
	}
	t.Fatalf("no selected item %s in %+v", path, items)
	return PacketItem{}
}
