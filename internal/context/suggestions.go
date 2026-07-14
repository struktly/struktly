package context

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/struktly/struktly/internal/files"
)

const agentInstructionsDir = ".struktly/agent-instructions"

var agentInstructionTargets = []struct {
	filename string
	target   string
}{
	{filename: "AGENTS.suggested.md", target: "generic"},
	{filename: "CLAUDE.suggested.md", target: "claude"},
	{filename: "CURSOR.suggested.md", target: "cursor"},
}

func SuggestInstructions(opts SuggestInstructionsOptions) (SuggestInstructionsResult, error) {
	root, err := files.CleanRoot(opts.Root)
	if err != nil {
		return SuggestInstructionsResult{}, err
	}

	projectContextPath := filepath.Join(root, ".struktly", "project-context.md")
	projectContext, err := files.ReadSmallTextFile(projectContextPath, 512*1024)
	if err != nil {
		return SuggestInstructionsResult{}, fmt.Errorf("read .struktly/project-context.md: %w; run `struktly scan` first", err)
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()

	draft := instructionDraft{
		root:           root,
		generatedAt:    now,
		projectContext: files.StripFrontmatter(projectContext),
		sourceRefs:     map[string]struct{}{".struktly/project-context.md": {}},
	}
	draft.readOptionalInputs()

	outputDir := filepath.Join(root, agentInstructionsDir)
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return SuggestInstructionsResult{}, fmt.Errorf("create agent instructions dir: %w", err)
	}

	result := SuggestInstructionsResult{OutputPaths: make([]string, 0, len(agentInstructionTargets))}
	for _, target := range agentInstructionTargets {
		outputPath := filepath.Join(outputDir, target.filename)
		content := draft.renderMarkdown(target.target)
		if err := os.WriteFile(outputPath, []byte(content), 0o644); err != nil {
			return SuggestInstructionsResult{}, fmt.Errorf("write %s: %w", target.filename, err)
		}
		result.OutputPaths = append(result.OutputPaths, outputPath)
	}

	return result, nil
}

type instructionDraft struct {
	root           string
	generatedAt    time.Time
	projectContext string

	currentDirection string
	constraints      string
	decisions        string
	verification     []string
	readWarnings     []string
	sourceRefs       map[string]struct{}
}

func (d *instructionDraft) readOptionalInputs() {
	for _, input := range []struct {
		rel    string
		assign func(string)
	}{
		{rel: ".struktly/direction.md", assign: func(text string) { d.currentDirection = text }},
		{rel: ".struktly/constraints.md", assign: func(text string) { d.constraints = text }},
		{rel: ".struktly/decisions.md", assign: func(text string) { d.decisions = text }},
	} {
		text, err := files.ReadSmallTextFile(filepath.Join(d.root, filepath.FromSlash(input.rel)), 512*1024)
		if err != nil {
			if !os.IsNotExist(err) {
				d.readWarnings = append(d.readWarnings, fmt.Sprintf("Unable to read `%s`: %v.", input.rel, err))
			}
			continue
		}
		input.assign(files.StripFrontmatter(text))
		files.AddString(d.sourceRefs, input.rel)
	}

	commands := rankCommands(extractBullets(sectionContent(d.projectContext, "## Build and test commands")))
	d.verification = suggestedChecks(commands)
}

// extractNonGoals collects bullets from direction-doc sections whose headings
// mark explicit non-goals, preserving their negative framing under a
// dedicated Non-goals heading in the rendered draft.
func extractNonGoals(markdown string) []string {
	nonGoals := map[string]struct{}{}
	for _, heading := range []string{
		"Non-goals",
		"Non-Goals",
		"What This Is Not",
		"Out of Scope",
	} {
		// Not extractBullets: its placeholder filter drops bullets containing
		// "no ", which legitimate non-goals often start with.
		for _, line := range strings.Split(sectionContent(markdown, "## "+heading), "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "- ") {
				continue
			}
			value := strings.TrimSpace(strings.TrimPrefix(line, "- "))
			if value != "" {
				files.AddString(nonGoals, value)
			}
		}
	}
	return files.SortedStrings(nonGoals)
}

func extractActiveDecisions(markdown string) []string {
	lines := strings.Split(markdown, "\n")
	decisions := make([]string, 0, 4)

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "## ") {
			continue
		}
		title := strings.TrimSpace(strings.TrimPrefix(line, "## "))
		if title == "" {
			continue
		}

		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if strings.HasPrefix(strings.TrimSpace(lines[j]), "## ") {
				end = j
				break
			}
		}
		section := strings.Join(lines[i:end], "\n")
		if !strings.Contains(section, "**Status:** accepted") {
			continue
		}

		summary := decisionSummary(section)
		if summary == "" {
			summary = title
		}
		decisions = append(decisions, title+": "+summary)
	}
	return decisions
}

func decisionSummary(section string) string {
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "**Decision:**") {
			return strings.TrimSpace(strings.TrimPrefix(line, "**Decision:**"))
		}
	}
	return excerptMarkdown(strings.TrimSpace(section), 240)
}

func (d *instructionDraft) renderMarkdown(target string) string {
	nonGoals := extractNonGoals(d.currentDirection)
	activeDecisions := extractActiveDecisions(d.decisions)
	checks := d.verification
	if len(checks) == 0 {
		checks = []string{"struktly scan"}
	}

	var b strings.Builder
	b.WriteString(files.OKFFrontmatter("agent-instructions", "Agent instructions draft ("+target+")", "Repository guidance prepared for human review.", d.generatedAt))
	b.WriteString("# Agent instructions draft\n\n")

	d.writePromotionNote(&b, target)

	b.WriteString("## Constraints\n\n")
	if strings.TrimSpace(d.constraints) != "" {
		b.WriteString("From `.struktly/constraints.md`:\n\n")
		b.WriteString(excerptMarkdown(d.constraints, 1600))
		b.WriteString("\n\n")
	} else {
		b.WriteString("No `.struktly/constraints.md` file was found.\n\n")
	}

	if len(nonGoals) > 0 {
		b.WriteString("## Non-goals\n\n")
		b.WriteString("From `.struktly/direction.md`:\n\n")
		writeBullets(&b, nonGoals)
		b.WriteString("\n")
	}

	if len(activeDecisions) > 0 {
		b.WriteString("## Active decisions\n\n")
		writeBullets(&b, activeDecisions)
		b.WriteString("\n")
	}

	b.WriteString("## Checks\n\n")
	writeCodeBullets(&b, checks, "Run `struktly scan` to refresh detected commands.")
	b.WriteString("\n")

	b.WriteString("## Using Struktly\n\n")
	writeBullets(&b, []string{
		"Run `struktly brief \"<task>\"` when task-specific repository context is needed.",
		"After work, record checks with `struktly evidence`.",
	})
	b.WriteString("\n")

	if len(d.readWarnings) > 0 {
		b.WriteString("## Warnings\n\n")
		writeBullets(&b, d.readWarnings)
		b.WriteString("\n")
	}

	b.WriteString("## Sources\n\n")
	writePathBullets(&b, files.SortedStrings(d.sourceRefs), "No source references recorded.")

	return b.String()
}

func (d *instructionDraft) writePromotionNote(b *strings.Builder, target string) {
	switch target {
	case "claude":
		b.WriteString("## Promotion\n\n")
		b.WriteString("Copy reviewed content into repo-root `CLAUDE.md`. Struktly never overwrites active agent instruction files automatically.\n\n")
	case "cursor":
		b.WriteString("## Promotion\n\n")
		b.WriteString("Copy reviewed content into `.cursor/rules/` or your active Cursor rules. Struktly never overwrites active agent instruction files automatically.\n\n")
	default:
		b.WriteString("## Promotion\n\n")
		b.WriteString("Copy reviewed content into repo-root `AGENTS.md`. Struktly never overwrites active agent instruction files automatically.\n\n")
	}
}
