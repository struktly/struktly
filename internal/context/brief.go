package context

import (
	stdcontext "context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/struktly/struktly/internal/files"
	"github.com/struktly/struktly/internal/memory"
	"github.com/struktly/struktly/internal/runs"
)

func Brief(opts BriefOptions) (BriefResult, error) {
	root, err := files.CleanRoot(opts.Root)
	if err != nil {
		return BriefResult{}, err
	}
	ctx := opts.Context
	if ctx == nil {
		ctx = stdcontext.Background()
	}
	repository, err := ResolveRepository(ctx, root)
	if err != nil {
		return BriefResult{}, err
	}
	root = repository.absoluteRoot
	task := strings.TrimSpace(opts.Task)
	if task == "" {
		return BriefResult{}, fmt.Errorf("task is required")
	}

	scan := newRepositoryScan(root)
	if err := scan.collect(); err != nil {
		return BriefResult{}, err
	}
	scan.finalizeOpenQuestions()

	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()

	packet := contextPacket{
		root:           root,
		task:           task,
		generatedAt:    now,
		projectContext: scan.renderMarkdown(),
		sourceRefs:     make(map[string]struct{}, len(scan.sourceRefs)),
	}
	for source := range scan.sourceRefs {
		packet.sourceRefs[source] = struct{}{}
	}
	packet.readOptionalInputs()
	pkt, err := packet.toPacket(ctx)
	if err != nil {
		return BriefResult{}, err
	}

	basename := now.Format("20060102-150405") + "-" + files.Slugify(task, 72)
	outputPath := filepath.Join(root, ".struktly", "context-packets", basename+".md")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return BriefResult{}, fmt.Errorf("create context packet dir: %w", err)
	}
	if err := os.WriteFile(outputPath, []byte(packet.renderMarkdown(pkt)), 0o644); err != nil {
		return BriefResult{}, fmt.Errorf("write context packet: %w", err)
	}

	packetJSON, err := json.MarshalIndent(pkt, "", "  ")
	if err != nil {
		return BriefResult{}, fmt.Errorf("encode context packet json: %w", err)
	}
	jsonOutputPath := filepath.Join(root, ".struktly", "context-packets", basename+".json")
	if err := os.WriteFile(jsonOutputPath, append(packetJSON, '\n'), 0o644); err != nil {
		return BriefResult{}, fmt.Errorf("write context packet json: %w", err)
	}

	if strings.TrimSpace(opts.RunID) != "" {
		if _, err := runs.AttachRunArtifact(runs.AttachRunArtifactOptions{
			Root:         root,
			RunID:        opts.RunID,
			ArtifactType: "brief",
			Path:         outputPath,
			Message:      "Attached context packet.",
			Now:          now,
		}); err != nil {
			return BriefResult{}, err
		}
	}

	return BriefResult{OutputPath: outputPath, PacketPath: jsonOutputPath, Packet: pkt}, nil
}

type contextPacket struct {
	root           string
	task           string
	generatedAt    time.Time
	projectContext string

	currentDirection string
	constraints      string
	decisions        string
	evidence         string
	approvedMemory   []memory.Record
	readWarnings     []string
	sourceRefs       map[string]struct{}
}

func (p *contextPacket) readOptionalInputs() {
	for _, input := range []struct {
		rel    string
		assign func(string)
	}{
		{rel: ".struktly/direction.md", assign: func(text string) { p.currentDirection = text }},
		{rel: ".struktly/constraints.md", assign: func(text string) { p.constraints = text }},
		{rel: ".struktly/decisions.md", assign: func(text string) { p.decisions = text }},
		{rel: ".struktly/evidence.md", assign: func(text string) { p.evidence = text }},
	} {
		text, err := files.ReadSmallTextFile(filepath.Join(p.root, filepath.FromSlash(input.rel)), 512*1024)
		if err != nil {
			if !os.IsNotExist(err) {
				p.readWarnings = append(p.readWarnings, fmt.Sprintf("Unable to read `%s`.", input.rel))
			}
			continue
		}
		input.assign(files.StripFrontmatter(text))
		files.AddString(p.sourceRefs, input.rel)
	}
	memories, err := memory.ReadApprovedForBrief(p.root, 8)
	if err != nil {
		p.readWarnings = append(p.readWarnings, "Unable to read approved memory.")
	} else {
		p.approvedMemory = memories
		if len(memories) > 0 {
			files.AddString(p.sourceRefs, ".struktly/memory/approved/")
		}
	}
	sort.Strings(p.readWarnings)
}

// derivedFields holds the values renderMarkdown and toPacket both need, so
// they are computed once from projectContext instead of twice.
type derivedFields struct {
	docs           []string
	suggestedFiles []string
	checks         []string
}

func (p *contextPacket) derive() derivedFields {
	commands := rankCommands(extractBullets(sectionContent(p.projectContext, "## Build / Test Commands")))
	docs := extractBullets(sectionContent(p.projectContext, "## Existing Docs"))
	adrs := extractBullets(sectionContent(p.projectContext, "## Existing ADRs / Decision Docs"))
	agentFiles := extractBullets(sectionContent(p.projectContext, "## Existing Agent Instruction Files"))
	topDirs := extractBullets(sectionContent(p.projectContext, "## Top-Level Directories"))
	return derivedFields{
		docs:           docs,
		suggestedFiles: p.suggestedFiles(docs, adrs, agentFiles, topDirs),
		checks:         suggestedChecks(commands),
	}
}

func (p *contextPacket) renderMarkdown(pkt Packet) string {
	d := p.derive()
	docs, suggestedFiles, checks := d.docs, d.suggestedFiles, d.checks

	var b strings.Builder
	b.WriteString(files.OKFFrontmatter("context-packet", "Context Packet: "+p.task, "Task-scoped repository context generated by struktly brief.", p.generatedAt))
	b.WriteString("# Struktly Context Packet\n\n")
	b.WriteString("Deterministic, live repository context for one task. No model calls.\n\n")

	b.WriteString("## Task\n\n")
	b.WriteString(p.task + "\n\n")
	b.WriteString("## Packet Identity\n\n")
	writeBullets(&b, []string{
		"Schema: `" + pkt.Schema + "`",
		"Packet hash: `" + pkt.PacketHash + "`",
		"Repository: `" + pkt.Repository.Identity + "`",
		"Branch: `" + emptyFallback(pkt.Repository.Branch, "detached HEAD") + "`",
		"HEAD revision: `" + pkt.Repository.HeadRevision + "`",
	})
	b.WriteString("\n")

	writeSectionExcerpt(&b, p.projectContext, []string{
		"## Repository",
		"## Top-Level Directories",
		"## Detected Languages / Frameworks",
	}, 1800)
	b.WriteString("\n")

	p.writeProductDirection(&b)

	if strings.TrimSpace(p.constraints) != "" {
		b.WriteString("## Constraints\n\n")
		b.WriteString("From `.struktly/constraints.md`:\n\n")
		b.WriteString(excerptMarkdown(p.constraints, 1600))
		b.WriteString("\n\n")
	}

	if len(p.approvedMemory) > 0 {
		b.WriteString("## Approved Memory\n\n")
		p.writeApprovedMemory(&b)
	}

	b.WriteString("## Verification Commands\n\n")
	writeCodeBullets(&b, checks, "No verification commands detected; run `struktly scan` to refresh.")
	b.WriteString("\n")

	if len(docs) > 0 {
		b.WriteString("## Relevant Docs\n\n")
		writePathBullets(&b, files.LimitStrings(docs, 15), "No docs listed.")
		b.WriteString("\n")
	}

	b.WriteString("## Suggested Files To Inspect\n\n")
	writePathBullets(&b, suggestedFiles, "No suggested files available.")
	b.WriteString("\n")

	p.writeSelectedContext(&b, pkt)

	if len(p.readWarnings) > 0 {
		b.WriteString("## Read Warnings\n\n")
		writeBullets(&b, p.readWarnings)
		b.WriteString("\n")
	}

	p.writeStruktlySetup(&b)

	b.WriteString("## Source References\n\n")
	writePathBullets(&b, files.SortedStrings(p.sourceRefs), "No source references recorded.")

	return b.String()
}

func (p *contextPacket) writeSelectedContext(b *strings.Builder, pkt Packet) {
	b.WriteString("## Selected Context\n\n")
	if len(pkt.Items) == 0 {
		b.WriteString("- No repository context items were selected.\n\n")
	} else {
		for _, item := range pkt.Items {
			b.WriteString("### `" + item.Path + "`\n\n")
			b.WriteString("- Kind: `" + item.Kind + "`\n")
			b.WriteString("- Included because: `" + item.Reason + "`\n")
			b.WriteString("- Content hash: `" + item.ContentHash + "`\n")
			fmt.Fprintf(b, "- Bytes: `%d/%d`\n\n", item.IncludedBytes, item.OriginalBytes)
			fence := markdownFence(item.Content)
			b.WriteString(fence + "text\n")
			b.WriteString(item.Content)
			if !strings.HasSuffix(item.Content, "\n") {
				b.WriteString("\n")
			}
			b.WriteString(fence + "\n\n")
		}
	}
	if len(pkt.Exclusions) == 0 && len(pkt.Truncations) == 0 {
		return
	}
	b.WriteString("## Context Decisions\n\n")
	for _, decision := range pkt.Exclusions {
		b.WriteString("- Excluded `" + decision.Path + "`: `" + decision.Reason + "`")
		if decision.Detail != "" {
			b.WriteString(" — " + decision.Detail)
		}
		b.WriteString("\n")
	}
	for _, decision := range pkt.Truncations {
		b.WriteString("- Truncated `" + decision.Path + "`: " + decision.Detail + "\n")
	}
	b.WriteString("\n")
}

func markdownFence(content string) string {
	fence := "```"
	for strings.Contains(content, fence) {
		fence += "`"
	}
	return fence
}

func emptyFallback(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func (p *contextPacket) writeProductDirection(b *strings.Builder) {
	direction := strings.TrimSpace(p.currentDirection)
	known := ""
	if direction == "" {
		known = strings.TrimSpace(sectionContent(p.projectContext, "## Known Product Direction"))
	}
	if direction == "" && known == "" && strings.TrimSpace(p.decisions) == "" && strings.TrimSpace(p.evidence) == "" {
		return
	}

	b.WriteString("## Product Direction\n\n")
	if direction != "" {
		b.WriteString("From `.struktly/direction.md`:\n\n")
		b.WriteString(excerptMarkdown(p.currentDirection, 1600))
		b.WriteString("\n\n")
	} else if known != "" {
		b.WriteString(known + "\n\n")
	}

	if strings.TrimSpace(p.decisions) != "" {
		b.WriteString("Existing decision ledger excerpt:\n\n")
		b.WriteString(excerptMarkdown(p.decisions, 900))
		b.WriteString("\n\n")
	}

	if strings.TrimSpace(p.evidence) != "" {
		b.WriteString("Existing evidence ledger excerpt:\n\n")
		b.WriteString(excerptMarkdown(p.evidence, 900))
		b.WriteString("\n\n")
	}
}

// missingContextNotes lists absent repo-owned context, shared by the
// markdown Struktly Setup section and the JSON packet's MissingContext.
func (p *contextPacket) missingContextNotes() []string {
	missing := []string{}
	if strings.TrimSpace(p.currentDirection) == "" {
		missing = append(missing, "`.struktly/direction.md` — product direction and non-goals that bound agent work")
	}
	if strings.TrimSpace(p.constraints) == "" {
		missing = append(missing, "`.struktly/constraints.md` — hard constraints excerpted into every packet")
	}
	if len(p.approvedMemory) == 0 {
		missing = append(missing, "approved memory under `.struktly/memory/approved/` — durable, human-approved learnings")
	}
	return missing
}

// writeStruktlySetup lists absent repo-owned context so a packet on a fresh
// repo ends with a setup pointer instead of scattered missing-file notes.
func (p *contextPacket) writeStruktlySetup(b *strings.Builder) {
	missing := p.missingContextNotes()
	if len(missing) == 0 {
		return
	}
	b.WriteString("## Struktly Setup\n\n")
	b.WriteString("This repository does not yet have repo-owned Struktly context. Missing:\n\n")
	writeBullets(b, missing)
	b.WriteString("\nRun `struktly init` to scaffold these files.\n\n")
}

// toPacket builds the machine-readable counterpart to renderMarkdown from
// the same packet state.
func (p *contextPacket) toPacket(ctx stdcontext.Context) (Packet, error) {
	d := p.derive()
	selection, err := selectPacketContext(ctx, p.root, p.task, d.checks)
	if err != nil {
		return Packet{}, err
	}
	p.sanitizeLegacyFields(selection)
	var mem []PacketMemoryItem
	for _, item := range p.approvedMemory {
		mem = append(mem, PacketMemoryItem{
			Content:        item.Content,
			Scope:          item.Scope,
			SourceRunID:    item.SourceRunID,
			SourceArtifact: item.SourceArtifact,
			Tags:           item.Tags,
		})
	}
	verification := uniqueSorted(append(append([]string(nil), selection.requiredChecks...), selection.suggestedChecks...))
	pkt := Packet{
		Schema:      PacketSchema,
		GeneratedAt: p.generatedAt,
		Metadata: PacketMetadata{
			GeneratedAt:     p.generatedAt.Format(time.RFC3339),
			AbsoluteGitRoot: ".",
		},
		Repository:           selection.repository,
		Items:                selection.items,
		InstructionFiles:     selection.instructions,
		RequiredChecks:       selection.requiredChecks,
		SuggestedChecks:      selection.suggestedChecks,
		Exclusions:           selection.exclusions,
		Truncations:          selection.truncations,
		Limits:               selection.limits,
		Task:                 p.task,
		Direction:            strings.TrimSpace(p.currentDirection),
		Constraints:          strings.TrimSpace(p.constraints),
		Decisions:            strings.TrimSpace(p.decisions),
		Evidence:             strings.TrimSpace(p.evidence),
		ApprovedMemory:       mem,
		VerificationCommands: verification,
		Docs:                 files.LimitStrings(d.docs, 15),
		SuggestedFiles:       d.suggestedFiles,
		MissingContext:       p.missingContextNotes(),
		ReadWarnings:         p.readWarnings,
		SourceRefs:           files.SortedStrings(p.sourceRefs),
	}
	if err := pkt.setHash(); err != nil {
		return Packet{}, fmt.Errorf("hash context packet: %w", err)
	}
	return pkt, nil
}

func (p *contextPacket) sanitizeLegacyFields(selection packetSelection) {
	selected := make(map[string]struct{}, len(selection.items))
	for _, item := range selection.items {
		selected[item.Path] = struct{}{}
	}
	for _, input := range []struct {
		path  string
		clear func()
	}{
		{path: ".struktly/direction.md", clear: func() { p.currentDirection = "" }},
		{path: ".struktly/constraints.md", clear: func() { p.constraints = "" }},
		{path: ".struktly/decisions.md", clear: func() { p.decisions = "" }},
		{path: ".struktly/evidence.md", clear: func() { p.evidence = "" }},
	} {
		if _, ok := selected[input.path]; !ok {
			input.clear()
		}
	}
	memories := p.approvedMemory[:0]
	for _, item := range p.approvedMemory {
		path := ".struktly/memory/approved/" + item.ID + ".json"
		if _, ok := selected[path]; ok {
			memories = append(memories, item)
		}
	}
	p.approvedMemory = memories
}

func (p *contextPacket) writeApprovedMemory(b *strings.Builder) {
	for _, item := range p.approvedMemory {
		line := "- " + item.Content
		details := []string{}
		if item.Scope != "" {
			details = append(details, "scope: "+item.Scope)
		}
		if item.SourceRunID != "" {
			details = append(details, "source_run_id: "+item.SourceRunID)
		}
		if item.SourceArtifact != "" {
			details = append(details, "source_artifact: "+item.SourceArtifact)
		}
		if len(item.Tags) > 0 {
			details = append(details, "tags: "+strings.Join(item.Tags, ", "))
		}
		if len(details) > 0 {
			line += " (" + strings.Join(details, "; ") + ")"
		}
		b.WriteString(line + "\n")
	}
	b.WriteString("\n")
}

func (p *contextPacket) suggestedFiles(docs, adrs, agentFiles, topDirs []string) []string {
	suggested := map[string]struct{}{}
	if files.FileExists(filepath.Join(p.root, "README.md")) {
		files.AddString(suggested, "README.md")
	}
	for _, rel := range files.LimitStrings(adrs, 10) {
		files.AddString(suggested, rel)
	}
	for _, rel := range files.LimitStrings(agentFiles, 5) {
		files.AddString(suggested, rel)
	}
	for _, rel := range files.LimitStrings(rankByTaskOverlap(p.task, docs), 10) {
		files.AddString(suggested, rel)
	}
	for _, rel := range files.LimitStrings(rankByTaskOverlap(p.task, topDirs), 5) {
		files.AddString(suggested, rel+"/")
	}
	for _, rel := range p.taskMatchedFiles(8) {
		files.AddString(suggested, rel)
	}
	return files.LimitStrings(files.SortedStrings(suggested), 25)
}

// taskMatchedFiles walks the repo up to two directory levels deep and returns
// files whose base name shares a word with the task, ranked by overlap. This
// is what turns a task like "add request timeout middleware" into a pointer
// at middleware/timeout.go instead of just middleware/.
func (p *contextPacket) taskMatchedFiles(limit int) []string {
	words := taskWords(p.task)
	if len(words) == 0 {
		return nil
	}
	found := map[string]struct{}{}
	paths, err := gitContextFiles(stdcontext.Background(), p.root)
	if err != nil {
		return nil
	}
	for _, rel := range paths {
		if strings.Count(rel, "/") > 2 || stalePathAncestor(rel) != "" {
			continue
		}
		lower := strings.ToLower(files.PathBase(rel))
		for word := range words {
			if strings.Contains(lower, word) {
				files.AddString(found, rel)
				break
			}
		}
	}

	return files.LimitStrings(rankByTaskOverlap(p.task, files.SortedStrings(found)), limit)
}

// rankByTaskOverlap orders paths by how many task words appear in the path,
// dropping paths that share no words with the task.
func rankByTaskOverlap(task string, paths []string) []string {
	words := taskWords(task)
	type scored struct {
		path  string
		score int
	}
	ranked := make([]scored, 0, len(paths))
	for _, path := range paths {
		lower := strings.ToLower(path)
		score := 0
		for word := range words {
			if strings.Contains(lower, word) {
				score++
			}
		}
		if score > 0 {
			ranked = append(ranked, scored{path: path, score: score})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return ranked[i].path < ranked[j].path
	})
	out := make([]string, 0, len(ranked))
	for _, item := range ranked {
		out = append(out, item.path)
	}
	return out
}

func taskWords(task string) map[string]struct{} {
	words := map[string]struct{}{}
	for _, word := range strings.FieldsFunc(strings.ToLower(task), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len(word) < 4 {
			continue
		}
		words[word] = struct{}{}
	}
	return words
}

func sectionContent(markdown, heading string) string {
	lines := strings.Split(markdown, "\n")
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == heading {
			start = i + 1
			break
		}
	}
	if start == -1 {
		return ""
	}
	end := len(lines)
	for i := start; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if strings.HasPrefix(line, "## ") && line != heading {
			end = i
			break
		}
	}
	return strings.TrimSpace(strings.Join(lines[start:end], "\n"))
}

func writeSectionExcerpt(b *strings.Builder, markdown string, headings []string, maxChars int) {
	parts := make([]string, 0, len(headings))
	for _, heading := range headings {
		content := sectionContent(markdown, heading)
		if strings.TrimSpace(content) == "" {
			continue
		}
		parts = append(parts, heading+"\n\n"+content)
	}
	if len(parts) == 0 {
		b.WriteString("No repository summary was found in `.struktly/project-context.md`.\n")
		return
	}
	excerpt := strings.Join(parts, "\n\n")
	if len(excerpt) > maxChars {
		excerpt = excerpt[:maxChars] + "\n\n..."
	}
	b.WriteString(strings.TrimSpace(excerpt) + "\n")
}

func extractBullets(section string) []string {
	values := map[string]struct{}{}
	for _, line := range strings.Split(section, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "- "))
		value = strings.Trim(value, "`")
		if value == "" || strings.Contains(strings.ToLower(value), "no ") {
			continue
		}
		files.AddString(values, value)
	}
	return files.SortedStrings(values)
}

func suggestedChecks(commands []string) []string {
	checks := map[string]struct{}{}
	for _, command := range commands {
		lower := strings.ToLower(command)
		if strings.Contains(lower, "test") || strings.Contains(lower, "vet") || strings.Contains(lower, "lint") || strings.Contains(lower, "build") {
			files.AddString(checks, command)
		}
	}
	if len(checks) == 0 {
		files.AddString(checks, "struktly scan")
	}
	return files.LimitStrings(rankCommands(files.SortedStrings(checks)), 8)
}

func rankCommands(commands []string) []string {
	ranked := append([]string(nil), commands...)
	sort.SliceStable(ranked, func(i, j int) bool {
		leftScore := commandScore(ranked[i])
		rightScore := commandScore(ranked[j])
		if leftScore != rightScore {
			return leftScore < rightScore
		}
		return ranked[i] < ranked[j]
	})
	return ranked
}

// commandScore ranks repo-root test commands first, then builds and static
// checks, with nested-directory variants after their root equivalents.
func commandScore(command string) int {
	lower := strings.ToLower(command)
	category := 3
	switch {
	case strings.Contains(lower, "test"):
		category = 0
	case strings.Contains(lower, "build"):
		category = 1
	case strings.Contains(lower, "vet") || strings.Contains(lower, "lint"):
		category = 2
	}
	nested := 0
	if strings.HasPrefix(lower, "cd ") || strings.Contains(lower, " -c ") {
		nested = 1
	}
	return category*2 + nested
}
