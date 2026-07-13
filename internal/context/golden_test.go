package context

import (
	"bytes"
	"encoding/json"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite golden files under testdata/golden")

// TestGoldenCorpus locks the byte-level shape of scan and brief outputs for a
// corpus of fixture repositories. Downstream consumers depend on output
// stability, so any diff here is an intentional format change: verify it,
// then regenerate with `go test ./internal/context -run TestGoldenCorpus -update`.
func TestGoldenCorpus(t *testing.T) {
	for _, fixture := range []string{"go-service", "flat-package", "noisy-legacy"} {
		t.Run(fixture, func(t *testing.T) {
			// Copy the fixture so Scan writes .struktly outputs into a
			// scratch root and the committed fixture stays pristine.
			root := filepath.Join(t.TempDir(), fixture)
			copyTree(t, filepath.Join("testdata", "fixtures", fixture), root)

			scanResult, err := Scan(ScanOptions{Root: root})
			if err != nil {
				t.Fatalf("Scan returned error: %v", err)
			}
			initGitRepo(t, root)
			briefResult, err := Brief(BriefOptions{
				Root: root,
				Task: "add request timeout middleware",
				Now:  time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC),
			})
			if err != nil {
				t.Fatalf("Brief returned error: %v", err)
			}

			goldenDir := filepath.Join("testdata", "golden", fixture)
			compareGolden(t, filepath.Join(goldenDir, "project-context.md"),
				normalizeMarkdown(readOutput(t, scanResult.OutputPath), root))
			compareGolden(t, filepath.Join(goldenDir, "packet.md"),
				normalizeMarkdown(readOutput(t, briefResult.OutputPath), root))
			compareGolden(t, filepath.Join(goldenDir, "snapshot.json"),
				normalizeSnapshot(t, readOutput(t, scanResult.SnapshotPath)))
			compareGolden(t, filepath.Join(goldenDir, "packet.json"),
				normalizePacket(t, readOutput(t, briefResult.PacketPath)))
		})
	}
}

func copyTree(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy fixture %s: %v", src, err)
	}
}

func readOutput(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output %s: %v", path, err)
	}
	return data
}

// normalizeMarkdown makes scan and brief markdown reproducible across runs:
// the scratch root becomes $ROOT and the OKF frontmatter timestamp becomes
// $TIMESTAMP.
func normalizeMarkdown(data []byte, root string) []byte {
	content := string(data)
	content = strings.ReplaceAll(content, filepath.ToSlash(root), "$ROOT")
	content = strings.ReplaceAll(content, root, "$ROOT")
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "timestamp: ") {
			lines[i] = "timestamp: $TIMESTAMP"
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

// normalizeSnapshot pins the run-dependent snapshot fields, then re-marshals
// so golden comparison sees stable bytes for everything else.
func normalizeSnapshot(t *testing.T, data []byte) []byte {
	t.Helper()
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	snap.GeneratedAt = time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	snap.Stats.DurationMS = 0
	out, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		t.Fatalf("marshal normalized snapshot: %v", err)
	}
	return append(out, '\n')
}

// normalizePacket pins the run-dependent packet field, then re-marshals so
// golden comparison sees stable bytes for everything else.
func normalizePacket(t *testing.T, data []byte) []byte {
	t.Helper()
	var pkt Packet
	if err := json.Unmarshal(data, &pkt); err != nil {
		t.Fatalf("unmarshal packet: %v", err)
	}
	pkt.GeneratedAt = time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	pkt.Metadata.GeneratedAt = "2026-07-11T12:00:00Z"
	out, err := json.MarshalIndent(pkt, "", "  ")
	if err != nil {
		t.Fatalf("marshal normalized packet: %v", err)
	}
	return append(out, '\n')
}

func compareGolden(t *testing.T, goldenPath string, got []byte) {
	t.Helper()
	if *update {
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatalf("create golden dir: %v", err)
		}
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatalf("write golden %s: %v", goldenPath, err)
		}
		return
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s (run with -update to create it): %v", goldenPath, err)
	}
	if bytes.Equal(got, want) {
		return
	}
	line, wantLine, gotLine := firstDiffLine(want, got)
	t.Fatalf("output differs from golden %s at line %d:\n-%s\n+%s\n(verify the change is intentional, then rerun with -update)", goldenPath, line, wantLine, gotLine)
}

// firstDiffLine returns the 1-based number of the first differing line and
// both sides of it, using "<missing>" when one output is shorter.
func firstDiffLine(want, got []byte) (int, string, string) {
	wantLines := strings.Split(string(want), "\n")
	gotLines := strings.Split(string(got), "\n")
	for i := 0; i < len(wantLines) || i < len(gotLines); i++ {
		wantLine, gotLine := "<missing>", "<missing>"
		if i < len(wantLines) {
			wantLine = wantLines[i]
		}
		if i < len(gotLines) {
			gotLine = gotLines[i]
		}
		if wantLine != gotLine {
			return i + 1, wantLine, gotLine
		}
	}
	return 0, "", ""
}
