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
	expectedRevision := strings.TrimSpace(opts.ExpectedBaseRevision)
	if expectedRevision != "" && repository.HeadRevision != expectedRevision {
		return BriefResult{}, repositoryChangedError(expectedRevision, repository.HeadRevision)
	}
	task := strings.TrimSpace(opts.Task)
	if task == "" {
		return BriefResult{}, fmt.Errorf("task is required")
	}
	scope, err := cleanScope(root, opts.Scope)
	if err != nil {
		return BriefResult{}, err
	}
	limits, err := resolvePacketLimits(PacketLimits{
		MaxItems:      opts.MaxItems,
		MaxFileBytes:  opts.MaxFileBytes,
		MaxTotalBytes: opts.MaxTotalBytes,
	})
	if err != nil {
		return BriefResult{}, err
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
		scope:          scope,
		generatedAt:    now,
		projectContext: scan.renderMarkdown(),
		sourceRefs:     make(map[string]struct{}, len(scan.sourceRefs)),
	}
	for source := range scan.sourceRefs {
		packet.sourceRefs[source] = struct{}{}
	}
	packet.readOptionalInputs()
	pkt, err := packet.toPacket(ctx, limits)
	if err != nil {
		return BriefResult{}, err
	}
	currentRepository, err := ResolveRepository(ctx, root)
	if err != nil {
		return BriefResult{}, err
	}
	if currentRepository.HeadRevision != repository.HeadRevision {
		return BriefResult{}, repositoryChangedError(repository.HeadRevision, currentRepository.HeadRevision)
	}
	if opts.NoWrite {
		return BriefResult{Packet: pkt}, nil
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

	return BriefResult{OutputPath: outputPath, PacketPath: jsonOutputPath, Packet: pkt}, nil
}

func repositoryChangedError(expected, actual string) error {
	return fmt.Errorf("%w: expected HEAD %s, found %s", ErrRepositoryChanged, expected, actual)
}

type contextPacket struct {
	root           string
	task           string
	scope          string
	generatedAt    time.Time
	projectContext string

	currentDirection string
	constraints      string
	decisions        string
	readWarnings     []string
	sourceRefs       map[string]struct{}
}

// readOptionalInputs records which repository guidance files exist and which
// could not be read. It deliberately does not retain their text: the content
// that reaches the packet comes from the selection, which scans for secrets and
// counts bytes against the packet budget. See sanitizeLegacyFields.
func (p *contextPacket) readOptionalInputs() {
	for _, rel := range []string{
		".struktly/direction.md",
		".struktly/constraints.md",
		".struktly/decisions.md",
	} {
		if _, err := os.Stat(filepath.Join(p.root, filepath.FromSlash(rel))); err != nil {
			if !os.IsNotExist(err) {
				p.readWarnings = append(p.readWarnings, fmt.Sprintf("Unable to read `%s`.", rel))
			}
			continue
		}
		files.AddString(p.sourceRefs, rel)
	}
	sort.Strings(p.readWarnings)
}

// derivedFields holds the values renderMarkdown and toPacket both need, so
// they are computed once from projectContext instead of twice.
type derivedFields struct {
	docs           []string
	suggestedFiles []string
	detectedChecks []string
}

func (p *contextPacket) derive() derivedFields {
	commands := rankCommands(extractBullets(sectionContent(p.projectContext, "## Build and test commands")))
	docs := extractBullets(sectionContent(p.projectContext, "## Documentation"))
	adrs := extractBullets(sectionContent(p.projectContext, "## Decision records"))
	agentFiles := extractBullets(sectionContent(p.projectContext, "## Agent instruction files"))
	topDirs := extractBullets(sectionContent(p.projectContext, "## Top-level directories"))
	return derivedFields{
		docs:           docs,
		suggestedFiles: p.suggestedFiles(docs, adrs, agentFiles, topDirs),
		detectedChecks: suggestedChecks(commands),
	}
}

func (p *contextPacket) renderMarkdown(pkt Packet) string {
	d := p.derive()
	docs, suggestedFiles := d.docs, d.suggestedFiles

	var b strings.Builder
	b.WriteString(files.OKFFrontmatter("context-packet", "Context: "+p.task, "Repository files and guidance selected for this task.", p.generatedAt))
	b.WriteString("# Context packet\n\n")
	b.WriteString("Generated locally from repository files and Git metadata.\n\n")

	b.WriteString("## Task\n\n")
	b.WriteString(p.task + "\n\n")
	b.WriteString("## Packet details\n\n")
	writeBullets(&b, []string{
		"Schema: `" + pkt.Schema + "`",
		"Packet hash: `" + pkt.PacketHash + "`",
		"Repository: `" + pkt.Repository.Identity + "`",
		"Branch: `" + emptyFallback(pkt.Repository.Branch, "detached HEAD") + "`",
		"HEAD revision: `" + pkt.Repository.HeadRevision + "`",
		"Scope: `" + emptyFallback(pkt.Scope, "whole repository") + "`",
	})
	b.WriteString("\n")

	writeSectionExcerpt(&b, p.projectContext, []string{
		"## Repository",
		"## Top-level directories",
		"## Languages and frameworks",
	}, 1800)
	b.WriteString("\n")

	p.writeDirection(&b)

	if strings.TrimSpace(p.constraints) != "" {
		b.WriteString("## Constraints\n\n")
		b.WriteString("From `.struktly/constraints.md`:\n\n")
		b.WriteString(excerptMarkdown(p.constraints, 1600))
		b.WriteString("\n\n")
	}

	b.WriteString("## Required checks\n\n")
	writeCodeBullets(&b, pkt.RequiredChecks, "No required checks are configured.")
	b.WriteString("\n")

	b.WriteString("## Suggested checks\n\n")
	writeCodeBullets(&b, pkt.SuggestedChecks, "No checks were detected from repository files.")
	b.WriteString("\n")

	if len(docs) > 0 {
		b.WriteString("## Relevant documentation\n\n")
		writePathBullets(&b, files.LimitStrings(docs, 15), "No docs listed.")
		b.WriteString("\n")
	}

	b.WriteString("## Files to inspect\n\n")
	writePathBullets(&b, suggestedFiles, "No suggested files available.")
	b.WriteString("\n")

	p.writeSelectedContext(&b, pkt)

	if len(p.readWarnings) > 0 {
		b.WriteString("## Warnings\n\n")
		writeBullets(&b, p.readWarnings)
		b.WriteString("\n")
	}

	b.WriteString("## Sources\n\n")
	writePathBullets(&b, files.SortedStrings(p.sourceRefs), "No source references recorded.")

	return b.String()
}

func (p *contextPacket) writeSelectedContext(b *strings.Builder, pkt Packet) {
	b.WriteString("## Included files\n\n")
	if len(pkt.Items) == 0 {
		b.WriteString("- No repository context items were selected.\n\n")
	} else {
		for _, item := range pkt.Items {
			b.WriteString("### `" + item.Path + "`\n\n")
			b.WriteString("- Type: " + humanLabel(item.Kind) + "\n")
			b.WriteString("- Why it was included: " + humanReason(item.Reason) + "\n")
			if item.Rendering == declarationRendering {
				b.WriteString("- Content: declarations only; function bodies are omitted\n")
			}
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
	b.WriteString("## Selection notes\n\n")
	for _, decision := range pkt.Exclusions {
		b.WriteString("- Excluded `" + decision.Path + "`: " + humanReason(decision.Reason))
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

func humanReason(value string) string {
	switch value {
	case "selection_rule":
		return "matched a repository context rule"
	case "repository_instruction":
		return "repository instruction file"
	case "task_match":
		return "its filename matched the task"
	case "symbol_match":
		return "it declares an identifier the request names"
	case "config_excluded":
		return "excluded by repository configuration"
	case "item_limit":
		return "packet file limit reached"
	case "content_limit":
		return "per-file size limit reached"
	case "total_limit":
		return "packet size limit reached"
	case "sensitive_path":
		return "sensitive filename"
	case "secret_detected":
		return "possible secret detected"
	case "non_regular":
		return "not a regular file"
	case "unreadable":
		return "file could not be read"
	default:
		return strings.ReplaceAll(value, "_", " ")
	}
}

func humanLabel(value string) string {
	label := strings.ReplaceAll(value, "_", " ")
	if label == "" {
		return ""
	}
	runes := []rune(label)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
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

// writeDirection renders repository direction from the selected guidance only.
//
// There used to be a fallback here that read a "## Repository Direction" section
// out of the scan summary when the guidance field was empty. It never ran: the
// scan writes that heading as "## Repository direction", and the lookup is an
// exact match. Correcting the case would have been the wrong repair. The scan
// populates that section from `.struktly/direction.md` and nothing else, so the
// fallback can differ from the guidance field in exactly one situation — when
// the selection excluded direction.md, for a detected secret, a sensitive name,
// or a packet limit. Those are the cases where the content must not be rendered,
// so the only times the fallback could have fired are the only times it must
// not. It is deleted rather than fixed.
func (p *contextPacket) writeDirection(b *strings.Builder) {
	direction := strings.TrimSpace(p.currentDirection)
	if direction == "" && strings.TrimSpace(p.decisions) == "" {
		return
	}

	b.WriteString("## Direction\n\n")
	if direction != "" {
		b.WriteString("From `.struktly/direction.md`:\n\n")
		b.WriteString(excerptMarkdown(p.currentDirection, 1600))
		b.WriteString("\n\n")
	}

	if strings.TrimSpace(p.decisions) != "" {
		b.WriteString("Existing decision ledger excerpt:\n\n")
		b.WriteString(excerptMarkdown(p.decisions, 900))
		b.WriteString("\n\n")
	}
}

// toPacket builds the machine-readable counterpart to renderMarkdown from
// the same packet state.
func (p *contextPacket) toPacket(ctx stdcontext.Context, limits PacketLimits) (Packet, error) {
	d := p.derive()
	selection, err := selectPacketContext(ctx, p.root, p.task, p.scope, d.detectedChecks, limits)
	if err != nil {
		return Packet{}, err
	}
	p.sanitizeLegacyFields(selection)
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
		Scope:                p.scope,
		Direction:            strings.TrimSpace(p.currentDirection),
		Constraints:          strings.TrimSpace(p.constraints),
		Decisions:            strings.TrimSpace(p.decisions),
		VerificationCommands: verification,
		Docs:                 files.LimitStrings(d.docs, 15),
		SuggestedFiles:       d.suggestedFiles,
		ReadWarnings:         append(append([]string{}, p.readWarnings...), selection.readWarnings...),
		SourceRefs:           files.SortedStrings(p.sourceRefs),
	}
	if err := pkt.setHash(); err != nil {
		return Packet{}, fmt.Errorf("hash context packet: %w", err)
	}
	return pkt, nil
}

// sanitizeLegacyFields makes the packet's guidance fields report exactly what
// the selection included.
//
// They used to be filled from readOptionalInputs' separate 512 KiB read, which
// no secret scanner ever saw, and cleared only when the file was absent from the
// selection entirely. A secret past the selector's 64 KiB per-file window was
// therefore not detected, not excluded, and shipped in full — while the packet's
// own truncation record for the same file claimed 64 KiB was all that went in.
//
// Deriving these fields from the selected item gives them every guarantee the
// selection already makes: those bytes were scanned for secrets, they were
// counted against the packet budget, and they are the bytes the item reports.
// The invariant is that the packet never emits content the scanner did not read.
func (p *contextPacket) sanitizeLegacyFields(selection packetSelection) {
	included := make(map[string]string, len(selection.items))
	for _, item := range selection.items {
		included[item.Path] = item.Content
	}
	for _, input := range []struct {
		path   string
		assign func(string)
	}{
		{path: ".struktly/direction.md", assign: func(text string) { p.currentDirection = text }},
		{path: ".struktly/constraints.md", assign: func(text string) { p.constraints = text }},
		{path: ".struktly/decisions.md", assign: func(text string) { p.decisions = text }},
	} {
		input.assign(files.StripFrontmatter(included[input.path]))
	}
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
	scoped := make([]string, 0, len(suggested))
	for _, rel := range files.SortedStrings(suggested) {
		// Directory suggestions carry a trailing slash; test the path itself.
		if withinScope(strings.TrimSuffix(rel, "/"), p.scope) {
			scoped = append(scoped, rel)
		}
	}
	return files.LimitStrings(scoped, 25)
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
		if strings.Count(rel, "/") > 2 || stalePathAncestor(rel) != "" || defaultRuntimePath(rel) {
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
		excerpt = truncateUTF8(excerpt, maxChars) + "\n\n..."
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
