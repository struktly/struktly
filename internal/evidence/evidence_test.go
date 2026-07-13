package evidence

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRecordEvidenceAppendsStructuredEntry(t *testing.T) {
	root := t.TempDir()
	packetPath := filepath.Join(root, ".struktly", "context-packets", "packet.md")
	writeFile(t, root, ".struktly/context-packets/packet.md", "# Packet\n\nTask context.\n")

	result, err := RecordEvidence(EvidenceOptions{
		Root:          root,
		Task:          "Complete MVP evidence command",
		Agent:         "Cursor agent",
		Outcome:       "Implemented append-only evidence writer with tests.",
		ContextPacket: packetPath,
		Checks:        []string{"cd product && go test ./internal/struktly/localcontext/..."},
		CheckResult:   "pass",
		FilesTouched:  []string{"product/internal/struktly/localcontext/evidence.go"},
		Reviewer:      "pending",
		Now:           time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RecordEvidence returned error: %v", err)
	}

	if result.OutputPath != filepath.Join(root, ".struktly", "evidence.md") {
		t.Fatalf("unexpected output path: %s", result.OutputPath)
	}

	data, err := os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatalf("read evidence ledger: %v", err)
	}
	content := string(data)

	if !strings.HasPrefix(content, "---\ntype: evidence-log\n") {
		t.Fatalf("fresh evidence ledger should start with OKF frontmatter:\n%s", content)
	}
	assertContains(t, content, "# Struktly Evidence Ledger")
	assertContains(t, content, "## 2026-07-05T12:00:00Z — Complete MVP evidence command")
	assertContains(t, content, "| **Task** | Complete MVP evidence command |")
	assertContains(t, content, "| **Agent/tool** | Cursor agent |")
	assertContains(t, content, ".struktly/context-packets/packet.md")
	assertContains(t, content, "sha256:")
	assertContains(t, content, "### Checks run")
	assertContains(t, content, "cd product && go test ./internal/struktly/localcontext/...")
	assertContains(t, content, "### Files touched")
	assertContains(t, content, "product/internal/struktly/localcontext/evidence.go")
	assertContains(t, content, "### Reviewer")
	assertContains(t, content, "pending")

	second, err := RecordEvidence(EvidenceOptions{
		Root:    root,
		Task:    "Second entry",
		Agent:   "human",
		Outcome: "Append-only behavior verified.",
		Now:     time.Date(2026, 7, 5, 13, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("second RecordEvidence returned error: %v", err)
	}
	if second.OutputPath != result.OutputPath {
		t.Fatalf("unexpected second output path: %s", second.OutputPath)
	}

	data, err = os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatalf("read evidence ledger after second append: %v", err)
	}
	content = string(data)
	if strings.Count(content, "## 2026-07-05T12:00:00Z — Complete MVP evidence command") != 1 {
		t.Fatalf("expected first entry once, got:\n%s", content)
	}
	assertContains(t, content, "## 2026-07-05T13:00:00Z — Second entry")
	assertContains(t, content, "| **Context packet** | _(none)_ |")
	if got := strings.Count(content, "---\ntype:"); got != 1 {
		t.Fatalf("expected exactly one frontmatter block after second append, got %d:\n%s", got, content)
	}
}

func TestRecordEvidenceRunChecksDerivesPass(t *testing.T) {
	root := t.TempDir()
	result, err := RecordEvidence(EvidenceOptions{
		Root:        root,
		Task:        "Verified pass",
		Agent:       "struktly",
		Outcome:     "Checks executed.",
		Checks:      []string{"true"},
		CheckResult: "pass",
		RunChecks:   true,
		Now:         time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RecordEvidence returned error: %v", err)
	}

	data, err := os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatalf("read evidence ledger: %v", err)
	}
	content := string(data)
	assertContains(t, content, "| **Verification** | checks executed by struktly |")
	assertContains(t, content, "| **Checks result** | pass |")
	assertContains(t, content, "- `true` — pass (exit 0, ")
	if strings.Contains(content, "```") {
		t.Fatalf("passing check should not include output block:\n%s", content)
	}
}

func TestRecordEvidenceRunChecksDerivesFailWithOutputTail(t *testing.T) {
	root := t.TempDir()
	result, err := RecordEvidence(EvidenceOptions{
		Root:      root,
		Task:      "Verified fail",
		Agent:     "struktly",
		Outcome:   "Checks executed.",
		Checks:    []string{"echo boom; exit 3"},
		RunChecks: true,
		Now:       time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RecordEvidence returned error: %v", err)
	}

	data, err := os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatalf("read evidence ledger: %v", err)
	}
	content := string(data)
	assertContains(t, content, "| **Checks result** | fail |")
	assertContains(t, content, "- `echo boom; exit 3` — fail (exit 3, ")
	assertContains(t, content, "  ```\n  boom\n  ```\n")
}

func TestRecordEvidenceRunChecksTruncatesOutputTail(t *testing.T) {
	root := t.TempDir()
	result, err := RecordEvidence(EvidenceOptions{
		Root:      root,
		Task:      "Long output",
		Agent:     "struktly",
		Outcome:   "Checks executed.",
		Checks:    []string{"for i in $(seq 1 15); do echo line$i; done; exit 1"},
		RunChecks: true,
		Now:       time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RecordEvidence returned error: %v", err)
	}

	data, err := os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatalf("read evidence ledger: %v", err)
	}
	content := string(data)
	assertContains(t, content, "  line6\n")
	assertContains(t, content, "  line15\n")
	if strings.Contains(content, "line5") {
		t.Fatalf("expected only last 10 lines of output:\n%s", content)
	}
}

func TestRecordEvidenceRunChecksRejectsConflictingResult(t *testing.T) {
	root := t.TempDir()
	_, err := RecordEvidence(EvidenceOptions{
		Root:        root,
		Task:        "Conflict",
		Agent:       "struktly",
		Outcome:     "Checks executed.",
		Checks:      []string{"false"},
		CheckResult: "pass",
		RunChecks:   true,
	})
	if err == nil || !strings.Contains(err.Error(), "--result conflicts with --run-checks; drop --result") {
		t.Fatalf("expected conflicting result error, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(root, ".struktly", "evidence.md")); !os.IsNotExist(statErr) {
		t.Fatalf("expected no ledger written on conflict, got: %v", statErr)
	}
}

func TestRecordEvidenceRunChecksRequiresChecks(t *testing.T) {
	root := t.TempDir()
	_, err := RecordEvidence(EvidenceOptions{
		Root:      root,
		Task:      "No checks",
		Agent:     "struktly",
		Outcome:   "Nothing to run.",
		RunChecks: true,
	})
	if err == nil || !strings.Contains(err.Error(), "--run-checks requires --checks") {
		t.Fatalf("expected missing checks error, got: %v", err)
	}
}

func TestRecordEvidenceRunChecksTimesOut(t *testing.T) {
	original := checkTimeout
	checkTimeout = 50 * time.Millisecond
	defer func() { checkTimeout = original }()

	root := t.TempDir()
	result, err := RecordEvidence(EvidenceOptions{
		Root:      root,
		Task:      "Timeout",
		Agent:     "struktly",
		Outcome:   "Checks executed.",
		Checks:    []string{"sleep 5"},
		RunChecks: true,
	})
	if err != nil {
		t.Fatalf("RecordEvidence returned error: %v", err)
	}

	data, err := os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatalf("read evidence ledger: %v", err)
	}
	content := string(data)
	assertContains(t, content, "- `sleep 5` — fail (timeout after 50ms)")
	assertContains(t, content, "| **Checks result** | fail |")
}

func TestRecordEvidenceSelfReportedRenderingUnchanged(t *testing.T) {
	root := t.TempDir()
	result, err := RecordEvidence(EvidenceOptions{
		Root:        root,
		Task:        "Ship feature",
		Agent:       "human",
		Outcome:     "done",
		Checks:      []string{"go test ./..."},
		CheckResult: "pass",
		Now:         time.Date(2026, 7, 5, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("RecordEvidence returned error: %v", err)
	}

	data, err := os.ReadFile(result.OutputPath)
	if err != nil {
		t.Fatalf("read evidence ledger: %v", err)
	}
	content := string(data)
	want := "\n## 2026-07-05T12:00:00Z — Ship feature\n\n" +
		"| Field | Value |\n" +
		"|-------|-------|\n" +
		"| **Task** | Ship feature |\n" +
		"| **Agent/tool** | human |\n" +
		"| **Context packet** | _(none)_ |\n" +
		"| **Outcome** | done |\n" +
		"\n### Checks run\n\n" +
		"- `go test ./...` — pass\n" +
		"\n---\n"
	if !strings.HasSuffix(content, want) {
		t.Fatalf("self-reported entry rendering changed:\n%s", content)
	}
	if strings.Contains(content, "Verification") {
		t.Fatalf("self-reported entry should not carry verification row:\n%s", content)
	}
}

func TestRecordEvidenceRequiresCoreFields(t *testing.T) {
	root := t.TempDir()
	if _, err := RecordEvidence(EvidenceOptions{Root: root}); err == nil {
		t.Fatalf("expected missing task error")
	}
	if _, err := RecordEvidence(EvidenceOptions{Root: root, Task: "x"}); err == nil {
		t.Fatalf("expected missing agent error")
	}
	if _, err := RecordEvidence(EvidenceOptions{Root: root, Task: "x", Agent: "y"}); err == nil {
		t.Fatalf("expected missing outcome error")
	}
}
