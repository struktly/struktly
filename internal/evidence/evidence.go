package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/struktly/struktly/internal/files"
	"github.com/struktly/struktly/internal/runs"
)

const evidenceLedgerHeader = `# Struktly Evidence Ledger

Append-only record of inspections, extractions, and verification outcomes. Audit-grade entries require human review.

---
`

type EvidenceOptions struct {
	Root          string
	Task          string
	Agent         string
	Outcome       string
	ContextPacket string
	Checks        []string
	CheckResult   string
	RunChecks     bool
	FilesTouched  []string
	Reviewer      string
	RunID         string
	Now           time.Time
}

// checkTimeout bounds each executed check; a variable so tests can shorten it.
var checkTimeout = 10 * time.Minute

const checkOutputTailLines = 10

type EvidenceResult struct {
	OutputPath string
}

func RecordEvidence(opts EvidenceOptions) (EvidenceResult, error) {
	root, err := files.CleanRoot(opts.Root)
	if err != nil {
		return EvidenceResult{}, err
	}

	task := strings.TrimSpace(opts.Task)
	if task == "" {
		return EvidenceResult{}, fmt.Errorf("task is required")
	}
	agent := strings.TrimSpace(opts.Agent)
	if agent == "" {
		return EvidenceResult{}, fmt.Errorf("agent is required")
	}
	outcome := strings.TrimSpace(opts.Outcome)
	if outcome == "" {
		return EvidenceResult{}, fmt.Errorf("outcome is required")
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()

	contextPath, contextHash, err := resolveContextPacket(root, opts.ContextPacket)
	if err != nil {
		return EvidenceResult{}, err
	}

	checks := normalizeList(opts.Checks)
	checkResult := strings.TrimSpace(opts.CheckResult)
	var checkRuns []checkRun
	if opts.RunChecks {
		if len(checks) == 0 {
			return EvidenceResult{}, fmt.Errorf("--run-checks requires --checks")
		}
		checkRuns = executeChecks(root, checks)
		derived := "pass"
		for _, run := range checkRuns {
			if !run.Passed {
				derived = "fail"
				break
			}
		}
		if checkResult != "" && checkResult != derived {
			return EvidenceResult{}, fmt.Errorf("--result conflicts with --run-checks; drop --result")
		}
		checkResult = derived
	}

	outputPath := filepath.Join(root, ".struktly", "evidence.md")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return EvidenceResult{}, fmt.Errorf("create .struktly dir: %w", err)
	}

	existing, err := readEvidenceLedger(outputPath, now)
	if err != nil {
		return EvidenceResult{}, err
	}

	entry := renderEvidenceEntry(evidenceEntry{
		Timestamp:    now,
		Task:         task,
		Agent:        agent,
		Outcome:      outcome,
		ContextPath:  contextPath,
		ContextHash:  contextHash,
		Checks:       checks,
		CheckResult:  checkResult,
		RunChecks:    opts.RunChecks,
		CheckRuns:    checkRuns,
		FilesTouched: normalizeList(opts.FilesTouched),
		Reviewer:     strings.TrimSpace(opts.Reviewer),
	})
	if err := os.WriteFile(outputPath, []byte(existing+entry), 0o644); err != nil {
		return EvidenceResult{}, fmt.Errorf("write evidence ledger: %w", err)
	}
	if strings.TrimSpace(opts.RunID) != "" {
		if _, err := runs.AttachRunArtifact(runs.AttachRunArtifactOptions{
			Root:         root,
			RunID:        opts.RunID,
			ArtifactType: "evidence",
			Path:         outputPath,
			Message:      "Attached evidence ledger.",
			Now:          now,
		}); err != nil {
			return EvidenceResult{}, err
		}
	}

	return EvidenceResult{OutputPath: outputPath}, nil
}

type evidenceEntry struct {
	Timestamp    time.Time
	Task         string
	Agent        string
	Outcome      string
	ContextPath  string
	ContextHash  string
	Checks       []string
	CheckResult  string
	RunChecks    bool
	CheckRuns    []checkRun
	FilesTouched []string
	Reviewer     string
}

type checkRun struct {
	Command string
	Status  string
	Passed  bool
	Tail    []string
}

func executeChecks(root string, checks []string) []checkRun {
	runs := make([]checkRun, 0, len(checks))
	for _, check := range checks {
		runs = append(runs, executeCheck(root, check))
	}
	return runs
}

func executeCheck(root, command string) checkRun {
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	cmd.Dir = root
	start := time.Now()
	output, err := cmd.CombinedOutput()
	duration := time.Since(start)

	run := checkRun{Command: command}
	if ctx.Err() == context.DeadlineExceeded {
		run.Status = fmt.Sprintf("fail (timeout after %s)", checkTimeout)
		run.Tail = outputTail(output, checkOutputTailLines)
		return run
	}
	if err == nil {
		run.Passed = true
		run.Status = fmt.Sprintf("pass (exit 0, %.2fs)", duration.Seconds())
		return run
	}
	exitCode := -1
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if len(output) == 0 {
		output = []byte(err.Error() + "\n")
	}
	run.Status = fmt.Sprintf("fail (exit %d, %.2fs)", exitCode, duration.Seconds())
	run.Tail = outputTail(output, checkOutputTailLines)
	return run
}

func outputTail(output []byte, maxLines int) []string {
	trimmed := strings.TrimRight(string(output), "\n")
	if trimmed == "" {
		return nil
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return lines
}

func readEvidenceLedger(path string, now time.Time) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return files.OKFFrontmatter("evidence-log", "Evidence Log", "Append-only ledger of verified agent work outcomes.", now) + evidenceLedgerHeader + "\n", nil
		}
		return "", fmt.Errorf("read evidence ledger: %w", err)
	}
	content := string(data)
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content, nil
}

func resolveContextPacket(root, contextPacket string) (path string, hash string, err error) {
	contextPacket = strings.TrimSpace(contextPacket)
	if contextPacket == "" {
		return "_(none)_", "", nil
	}

	if !filepath.IsAbs(contextPacket) {
		contextPacket = filepath.Join(root, contextPacket)
	}
	data, err := os.ReadFile(contextPacket)
	if err != nil {
		return "", "", fmt.Errorf("read context packet %s: %w", contextPacket, err)
	}
	sum := sha256.Sum256(data)
	rel, relErr := filepath.Rel(root, contextPacket)
	if relErr != nil || strings.HasPrefix(rel, "..") {
		return filepath.ToSlash(contextPacket), hex.EncodeToString(sum[:]), nil
	}
	return filepath.ToSlash(rel), hex.EncodeToString(sum[:]), nil
}

func renderEvidenceEntry(entry evidenceEntry) string {
	var b strings.Builder
	title := evidenceTitle(entry.Task)
	fmt.Fprintf(&b, "\n## %s — %s\n\n", entry.Timestamp.Format(time.RFC3339), title)
	b.WriteString("| Field | Value |\n")
	b.WriteString("|-------|-------|\n")
	fmt.Fprintf(&b, "| **Task** | %s |\n", escapeTableCell(entry.Task))
	fmt.Fprintf(&b, "| **Agent/tool** | %s |\n", escapeTableCell(entry.Agent))
	if entry.ContextHash == "" {
		fmt.Fprintf(&b, "| **Context packet** | %s |\n", escapeTableCell(entry.ContextPath))
	} else {
		fmt.Fprintf(&b, "| **Context packet** | `%s` (sha256:%s) |\n", escapeTableCell(entry.ContextPath), entry.ContextHash)
	}
	fmt.Fprintf(&b, "| **Outcome** | %s |\n", escapeTableCell(entry.Outcome))
	if entry.RunChecks {
		b.WriteString("| **Verification** | checks executed by struktly |\n")
		fmt.Fprintf(&b, "| **Checks result** | %s |\n", escapeTableCell(entry.CheckResult))
		b.WriteString("\n### Checks run\n\n")
		for _, run := range entry.CheckRuns {
			fmt.Fprintf(&b, "- `%s` — %s\n", run.Command, run.Status)
			if !run.Passed && len(run.Tail) > 0 {
				b.WriteString("\n  ```\n")
				for _, line := range run.Tail {
					b.WriteString("  " + line + "\n")
				}
				b.WriteString("  ```\n")
			}
		}
	} else if len(entry.Checks) > 0 {
		b.WriteString("\n### Checks run\n\n")
		for _, check := range entry.Checks {
			line := fmt.Sprintf("- `%s`", check)
			if entry.CheckResult != "" {
				line += " — " + entry.CheckResult
			}
			b.WriteString(line + "\n")
		}
	}

	if len(entry.FilesTouched) > 0 {
		b.WriteString("\n### Files touched\n\n")
		for _, file := range entry.FilesTouched {
			fmt.Fprintf(&b, "- `%s`\n", file)
		}
	}

	if entry.Reviewer != "" {
		b.WriteString("\n### Reviewer\n\n")
		b.WriteString(entry.Reviewer + "\n")
	}

	b.WriteString("\n---\n")
	return b.String()
}

func evidenceTitle(task string) string {
	task = strings.TrimSpace(task)
	if task == "" {
		return "evidence entry"
	}
	if len(task) <= 80 {
		return task
	}
	return strings.TrimSpace(task[:80]) + "…"
}

func escapeTableCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	value = strings.ReplaceAll(value, "\n", " ")
	return value
}

func normalizeList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
