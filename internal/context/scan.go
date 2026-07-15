package context

import (
	stdcontext "context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/struktly/struktly/internal/files"
	"github.com/struktly/struktly/internal/runs"
)

const (
	maxSummaryFileBytes = 256 * 1024
	maxDocsListed       = 40
	maxCommandsListed   = 30
	maxSourceRefsListed = 40
)

type repositoryScan struct {
	root             string
	name             string
	topDirs          map[string]struct{}
	languages        map[string]struct{}
	commands         map[string]struct{}
	docs             map[string]struct{}
	adrs             map[string]struct{}
	agentFiles       map[string]struct{}
	ignored          map[string]struct{}
	stale            map[string]struct{}
	sourceRefs       map[string]struct{}
	direction        string
	directionSource  string
	openQuestions    []string
	prov             map[string][]Provenance
	filesScanned     int
	gitAuthoritative bool
}

func Scan(opts ScanOptions) (ScanResult, error) {
	root, err := files.CleanRoot(opts.Root)
	if err != nil {
		return ScanResult{}, err
	}
	if opts.NoWrite && strings.TrimSpace(opts.RunID) != "" {
		return ScanResult{}, fmt.Errorf("--no-write cannot be used with --run")
	}

	now := opts.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	now = now.UTC()

	start := time.Now()
	scan := newRepositoryScan(root)
	if err := scan.collect(); err != nil {
		return ScanResult{}, err
	}
	scan.finalizeOpenQuestions()

	snap := scan.snapshot(now, time.Since(start))
	if opts.NoWrite {
		return ScanResult{Snapshot: snap}, nil
	}

	outputPath := filepath.Join(root, ".struktly", "project-context.md")
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return ScanResult{}, fmt.Errorf("create .struktly dir: %w", err)
	}
	content := files.OKFFrontmatter("project-context", "Repository context: "+scan.name, "Local summary of repository files, commands, and guidance.", now) + scan.renderMarkdown()
	if err := os.WriteFile(outputPath, []byte(content), 0o644); err != nil {
		return ScanResult{}, fmt.Errorf("write project context: %w", err)
	}

	snapshotPath := filepath.Join(root, ".struktly", "scans", "latest.json")
	if err := os.MkdirAll(filepath.Dir(snapshotPath), 0o755); err != nil {
		return ScanResult{}, fmt.Errorf("create .struktly/scans dir: %w", err)
	}
	snapshotData, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return ScanResult{}, fmt.Errorf("marshal scan snapshot: %w", err)
	}
	if err := os.WriteFile(snapshotPath, append(snapshotData, '\n'), 0o644); err != nil {
		return ScanResult{}, fmt.Errorf("write scan snapshot: %w", err)
	}

	if strings.TrimSpace(opts.RunID) != "" {
		if _, err := runs.AttachRunArtifact(runs.AttachRunArtifactOptions{
			Root:         root,
			RunID:        opts.RunID,
			ArtifactType: "scan",
			Path:         outputPath,
			Message:      "Attached repository scan output.",
		}); err != nil {
			return ScanResult{}, err
		}
	}

	return ScanResult{OutputPath: outputPath, Snapshot: snap, SnapshotPath: snapshotPath}, nil
}

func newRepositoryScan(root string) *repositoryScan {
	return &repositoryScan{
		root:       root,
		name:       filepath.Base(root),
		topDirs:    map[string]struct{}{},
		languages:  map[string]struct{}{},
		commands:   map[string]struct{}{},
		docs:       map[string]struct{}{},
		adrs:       map[string]struct{}{},
		agentFiles: map[string]struct{}{},
		ignored:    map[string]struct{}{},
		stale:      map[string]struct{}{},
		sourceRefs: map[string]struct{}{},
		prov:       map[string][]Provenance{},
	}
}

func (s *repositoryScan) collect() error {
	ignores := files.NewIgnoreMatcher(s.root)
	if _, err := os.Stat(filepath.Join(s.root, ".gitignore")); err == nil {
		files.AddString(s.sourceRefs, ".gitignore")
	}

	entries, err := os.ReadDir(s.root)
	if err != nil {
		return fmt.Errorf("read root: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if ignores.ShouldSkip(name, true) {
			files.AddString(s.ignored, name)
			continue
		}
		files.AddString(s.topDirs, name)
		s.note("top_dir", name, Provenance{Source: name, Method: "dir-listing", Confidence: "detected"})
	}
	if gitRevision(s.root) != "" {
		s.gitAuthoritative = true
		return s.collectGitFiles()
	}

	err = filepath.WalkDir(s.root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			s.addOpenQuestion(fmt.Sprintf("Unable to inspect `%s`.", files.RelPath(s.root, path)))
			if entry != nil && entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		rel := files.RelPath(s.root, path)
		if rel == "." {
			return nil
		}
		if ignores.ShouldSkip(rel, entry.IsDir()) {
			files.AddString(s.ignored, rel)
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if isStaleDirName(entry.Name()) {
				files.AddString(s.stale, rel)
				return filepath.SkipDir
			}
			return nil
		}

		s.inspectFile(rel, path)
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk repository: %w", err)
	}
	return nil
}

func (s *repositoryScan) collectGitFiles() error {
	paths, err := gitContextFiles(stdcontext.Background(), s.root)
	if err != nil {
		return fmt.Errorf("enumerate Git files: %w", err)
	}
	if files.FileExists(filepath.Join(s.root, ".git")) {
		files.AddString(s.ignored, ".git")
	}
	for _, rel := range paths {
		if stale := stalePathAncestor(rel); stale != "" {
			files.AddString(s.stale, stale)
			continue
		}
		info, err := os.Lstat(filepath.Join(s.root, filepath.FromSlash(rel)))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			s.addOpenQuestion(fmt.Sprintf("Unable to inspect `%s`.", rel))
			continue
		}
		if !info.Mode().IsRegular() {
			files.AddString(s.ignored, rel)
			continue
		}
		s.inspectFile(rel, filepath.Join(s.root, filepath.FromSlash(rel)))
	}
	return nil
}

func stalePathAncestor(rel string) string {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	for i := 0; i < len(parts)-1; i++ {
		if isStaleDirName(parts[i]) {
			return strings.Join(parts[:i+1], "/")
		}
	}
	return ""
}

func (s *repositoryScan) inspectFile(rel, path string) {
	s.filesScanned++
	base := files.PathBase(rel)
	lowerBase := strings.ToLower(base)
	lowerRel := strings.ToLower(rel)
	ext := strings.ToLower(filepath.Ext(base))

	switch {
	case base == "go.mod":
		s.noteLanguage("Go", rel, "go-mod", "detected")
		files.AddString(s.sourceRefs, rel)
		s.addGoCommands(rel)
	case base == "package.json":
		s.noteLanguage("JavaScript/TypeScript", rel, "package-json", "detected")
		files.AddString(s.sourceRefs, rel)
		s.inspectPackageJSON(rel, path)
	case base == "pyproject.toml" || base == "requirements.txt":
		method := "pyproject"
		if base == "requirements.txt" {
			method = "requirements-txt"
		}
		s.noteLanguage("Python", rel, method, "detected")
		files.AddString(s.sourceRefs, rel)
		s.addPythonCommands(rel, method)
	case base == "Cargo.toml":
		s.noteLanguage("Rust", rel, "cargo-toml", "detected")
		files.AddString(s.sourceRefs, rel)
	case base == "Dockerfile" || strings.HasPrefix(lowerBase, "docker-compose"):
		s.noteLanguage("Docker", rel, "dockerfile", "detected")
		files.AddString(s.sourceRefs, rel)
	case ext == ".astro":
		s.noteLanguage("Astro", rel, "file-extension", "inferred")
	case ext == ".ts" || ext == ".tsx" || ext == ".js" || ext == ".jsx":
		s.noteLanguage("JavaScript/TypeScript", rel, "file-extension", "inferred")
	case ext == ".go":
		s.noteLanguage("Go", rel, "file-extension", "inferred")
	case ext == ".py":
		s.noteLanguage("Python", rel, "file-extension", "inferred")
	case ext == ".yaml" || ext == ".yml":
		s.noteLanguage("YAML", rel, "file-extension", "inferred")
	}

	if base == "Makefile" || strings.HasSuffix(base, ".mk") {
		files.AddString(s.sourceRefs, rel)
		s.inspectMakefile(rel, path)
	}

	if isDocPath(rel) {
		files.AddString(s.docs, rel)
		s.note("doc", rel, Provenance{Source: rel, Method: "doc-path", Confidence: "detected"})
		files.AddString(s.sourceRefs, rel)
	}
	if isADRPath(rel) {
		files.AddString(s.adrs, rel)
		s.note("adr", rel, Provenance{Source: rel, Method: "adr-path", Confidence: "detected"})
		files.AddString(s.sourceRefs, rel)
	}
	if isAgentInstructionPath(rel) {
		files.AddString(s.agentFiles, rel)
		s.note("instruction_file", rel, Provenance{Source: rel, Method: "instruction-file", Confidence: "detected"})
		files.AddString(s.sourceRefs, rel)
	}
	if rel == ".struktly/direction.md" {
		s.directionSource = rel
		if text, err := files.ReadSmallTextFile(path, maxSummaryFileBytes); err == nil {
			if containsSecret(text) {
				s.addOpenQuestion("Excluded `.struktly/direction.md` because it contains a detected secret.")
			} else {
				s.direction = excerptMarkdown(files.StripFrontmatter(text), 1200)
			}
		} else {
			s.addOpenQuestion(fmt.Sprintf("Unable to read `%s`: %v.", rel, err))
		}
	}
	if strings.Contains(lowerRel, "playwright") {
		command := commandForDir(filepath.Dir(rel), "npx playwright test")
		files.AddString(s.commands, command)
		s.note("command", command, Provenance{Source: rel, Method: "playwright-config", Confidence: "inferred"})
	}
}

// noteLanguage adds a language with its provenance in one step.
func (s *repositoryScan) noteLanguage(name, source, method, confidence string) {
	files.AddString(s.languages, name)
	s.note("language", name, Provenance{Source: source, Method: method, Confidence: confidence})
}

func (s *repositoryScan) addGoCommands(rel string) {
	prefix := commandPrefix(filepath.Dir(rel))
	for _, command := range []string{prefix + "go test ./...", prefix + "go vet ./...", prefix + "go build ./..."} {
		files.AddString(s.commands, command)
		s.note("command", command, Provenance{Source: rel, Method: "go-mod", Confidence: "detected"})
	}
}

func (s *repositoryScan) addPythonCommands(rel, method string) {
	command := commandPrefix(filepath.Dir(rel)) + "python -m pytest"
	files.AddString(s.commands, command)
	s.note("command", command, Provenance{Source: rel, Method: method, Confidence: "detected"})
}

func (s *repositoryScan) inspectMakefile(rel, path string) {
	text, err := files.ReadSmallTextFile(path, maxSummaryFileBytes)
	if err != nil {
		s.addOpenQuestion(fmt.Sprintf("Unable to read `%s`: %v.", rel, err))
		return
	}
	if containsSecret(text) {
		s.addOpenQuestion(fmt.Sprintf("Excluded `%s` command detection because it contains a detected secret.", rel))
		return
	}

	dir := filepath.Dir(rel)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, ".") || strings.HasPrefix(line, "#") || strings.Contains(line, "=") {
			continue
		}
		target, ok := strings.CutSuffix(strings.Fields(line)[0], ":")
		if !ok || !isCommonTarget(target) {
			continue
		}
		command := "make " + target
		if dir != "." {
			command = "make -C " + filepath.ToSlash(dir) + " " + target
		}
		files.AddString(s.commands, command)
		s.note("command", command, Provenance{Source: rel, Location: "target:" + target, Method: "makefile-target", Confidence: "detected"})
	}
}

func (s *repositoryScan) inspectPackageJSON(rel, path string) {
	text, err := files.ReadSmallTextFile(path, maxSummaryFileBytes)
	if err != nil {
		s.addOpenQuestion(fmt.Sprintf("Unable to read `%s`: %v.", rel, err))
		return
	}
	if containsSecret(text) {
		s.addOpenQuestion(fmt.Sprintf("Excluded `%s` command detection because it contains a detected secret.", rel))
		return
	}

	var pkg struct {
		Scripts      map[string]string `json:"scripts"`
		Dependencies map[string]string `json:"dependencies"`
		DevDeps      map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal([]byte(text), &pkg); err != nil {
		s.addOpenQuestion(fmt.Sprintf("Unable to parse `%s`: %v.", rel, err))
		return
	}

	for dep := range pkg.Dependencies {
		s.detectFramework(rel, dep)
	}
	for dep := range pkg.DevDeps {
		s.detectFramework(rel, dep)
	}

	dir := filepath.Dir(rel)
	manager := packageManagerForDir(s.root, dir)
	for name := range pkg.Scripts {
		if !isCommonScript(name) {
			continue
		}
		command := packageCommand(dir, manager, name)
		files.AddString(s.commands, command)
		s.note("command", command, Provenance{Source: rel, Location: "scripts." + name, Method: "package-json-script", Confidence: "detected"})
	}
}

func (s *repositoryScan) detectFramework(source, dep string) {
	framework := ""
	switch strings.ToLower(dep) {
	case "astro":
		framework = "Astro"
	case "next":
		framework = "Next.js"
	case "react":
		framework = "React"
	case "vue":
		framework = "Vue"
	case "svelte":
		framework = "Svelte"
	case "vite":
		framework = "Vite"
	case "typescript":
		framework = "TypeScript"
	}
	if framework != "" {
		s.noteLanguage(framework, source, "package-json-dependency", "detected")
	}
}

func (s *repositoryScan) addOpenQuestion(question string) {
	question = strings.TrimSpace(question)
	if question == "" {
		return
	}
	s.openQuestions = append(s.openQuestions, question)
}

func (s *repositoryScan) finalizeOpenQuestions() {
	if len(s.commands) == 0 {
		s.openQuestions = append(s.openQuestions, "No build or test commands were detected from common project files.")
	}
	sort.Strings(s.openQuestions)
}

func (s *repositoryScan) renderMarkdown() string {
	var b strings.Builder
	ignoreDescription := "Repository files were enumerated with Git's tracked and non-ignored file set."
	if !s.gitAuthoritative {
		ignoreDescription = "Outside Git, root-level exact, directory, and glob .gitignore patterns are applied; negation and full Git semantics require a Git repository."
	}
	b.WriteString("# Repository context\n\n")
	b.WriteString("Generated locally from repository files and Git metadata.\n\n")

	b.WriteString("## Repository\n\n")
	writeBullets(&b, []string{
		"Repository name: " + s.name,
		"Repository root: `.`",
	})
	b.WriteString("\n")

	b.WriteString("## Top-level directories\n\n")
	writePathBullets(&b, files.SortedStrings(s.topDirs), "No top-level directories detected.")
	b.WriteString("\n")

	b.WriteString("## Languages and frameworks\n\n")
	writeBullets(&b, files.SortedStrings(s.languages), "No languages or frameworks detected.")
	b.WriteString("\n")

	commands := rankCommands(files.SortedStrings(s.commands))
	b.WriteString("## Build and test commands\n\n")
	writeCodeBullets(&b, files.LimitStrings(commands, maxCommandsListed), "No common build or test commands detected.")
	if extra := len(commands) - maxCommandsListed; extra > 0 {
		fmt.Fprintf(&b, "\n(%d more commands not listed.)\n", extra)
	}
	b.WriteString("\n")

	docs := rankPathsByDepth(files.SortedStrings(s.docs))
	b.WriteString("## Documentation\n\n")
	writePathBullets(&b, files.LimitStrings(docs, maxDocsListed), "No docs detected.")
	if extra := len(docs) - maxDocsListed; extra > 0 {
		fmt.Fprintf(&b, "\n(%d more docs not listed; shallow current docs are preferred.)\n", extra)
	}
	b.WriteString("\n")

	if len(s.adrs) > 0 {
		b.WriteString("## Decision records\n\n")
		writePathBullets(&b, files.SortedStrings(s.adrs), "")
		b.WriteString("\n")
	}

	if len(s.agentFiles) > 0 {
		b.WriteString("## Agent instruction files\n\n")
		writePathBullets(&b, files.SortedStrings(s.agentFiles), "")
		b.WriteString("\n")
	}

	b.WriteString("## Files excluded from context\n\n")
	writeBullets(&b, []string{
		"Git-ignored files and `.git` internals.",
		"Dependencies, build output, caches, and generated runtime state.",
		"Common credential files and secret-looking filenames.",
		"Symlinks, non-regular files, binaries, and invalid UTF-8 text.",
		"File discovery: " + ignoreDescription,
	})
	ignored := files.LimitStrings(files.SortedStrings(s.ignored), 60)
	if len(ignored) > 0 {
		b.WriteString("\nDetected skipped paths:\n\n")
		writePathBullets(&b, ignored, "")
	}
	b.WriteString("\n")

	if len(s.stale) > 0 {
		b.WriteString("## Deprioritized paths\n\n")
		b.WriteString("These directories look archived or fixture-only, so their docs and commands were not treated as current repository guidance:\n\n")
		writePathBullets(&b, files.LimitStrings(files.SortedStrings(s.stale), 20), "")
		b.WriteString("\n")
	}

	if s.direction != "" {
		b.WriteString("## Repository direction\n\n")
		b.WriteString("Source: `" + s.directionSource + "`\n\n")
		b.WriteString(s.direction)
		b.WriteString("\n\n")
	}

	if len(s.openQuestions) > 0 {
		b.WriteString("## Gaps found\n\n")
		writeBullets(&b, s.openQuestions)
		b.WriteString("\n")
	}

	refs := rankPathsByDepth(files.SortedStrings(s.sourceRefs))
	b.WriteString("## Sources\n\n")
	writePathBullets(&b, files.LimitStrings(refs, maxSourceRefsListed), "No source references recorded.")
	if extra := len(refs) - maxSourceRefsListed; extra > 0 {
		fmt.Fprintf(&b, "\n(%d more source references not listed.)\n", extra)
	}

	return b.String()
}

// isStaleDirName marks directories that hold archived or fixture content so
// docs and commands inside them stay out of agent-facing context. A leading
// underscore is a common repo-owned quarantine convention (e.g. `_legacy`)
// and is treated the same way regardless of the rest of the name.
func isStaleDirName(name string) bool {
	if strings.HasPrefix(name, "_") {
		return true
	}
	switch strings.ToLower(name) {
	case "legacy", "archive", "archived", "deprecated", "attic", "old", "testdata":
		return true
	default:
		return false
	}
}

func rankPathsByDepth(paths []string) []string {
	ranked := append([]string(nil), paths...)
	sort.SliceStable(ranked, func(i, j int) bool {
		di, dj := strings.Count(ranked[i], "/"), strings.Count(ranked[j], "/")
		if di != dj {
			return di < dj
		}
		return ranked[i] < ranked[j]
	})
	return ranked
}

func isDocPath(rel string) bool {
	if strings.HasPrefix(rel, ".struktly/") {
		return false
	}
	base := files.PathBase(rel)
	return strings.EqualFold(base, "README.md") ||
		(strings.HasPrefix(rel, "docs/") && strings.HasSuffix(strings.ToLower(rel), ".md")) ||
		strings.Contains(rel, "/docs/") && strings.HasSuffix(strings.ToLower(rel), ".md")
}

func isADRPath(rel string) bool {
	lower := strings.ToLower(rel)
	if strings.HasPrefix(lower, "docs/adr/") {
		return strings.HasSuffix(lower, ".md")
	}
	base := strings.ToLower(files.PathBase(rel))
	return strings.HasSuffix(lower, ".md") && (strings.Contains(base, "adr") || strings.Contains(base, "decision"))
}

func isAgentInstructionPath(rel string) bool {
	lower := strings.ToLower(rel)
	base := strings.ToLower(files.PathBase(rel))
	switch base {
	case "agents.md", "claude.md", "gemini.md", ".cursorrules", "codex.md":
		return true
	}
	return lower == ".github/copilot-instructions.md" || strings.HasPrefix(lower, ".cursor/rules/")
}

func isCommonTarget(target string) bool {
	switch target {
	case "test", "build", "lint", "fmt", "vet", "ci", "e2e", "play", "run", "run-mcp", "dev":
		return true
	default:
		return false
	}
}

func isCommonScript(script string) bool {
	switch script {
	case "test", "build", "lint", "typecheck", "check", "dev", "format", "fmt":
		return true
	default:
		return false
	}
}

func commandPrefix(dir string) string {
	dir = filepath.ToSlash(dir)
	if dir == "." || dir == "" {
		return ""
	}
	return "cd " + dir + " && "
}

func commandForDir(dir, command string) string {
	return commandPrefix(dir) + command
}

func packageManagerForDir(root, dir string) string {
	if dir == "." {
		dir = ""
	}
	fullDir := filepath.Join(root, filepath.FromSlash(dir))
	switch {
	case files.FileExists(filepath.Join(fullDir, "yarn.lock")):
		return "yarn"
	case files.FileExists(filepath.Join(fullDir, "pnpm-lock.yaml")):
		return "pnpm"
	default:
		return "npm"
	}
}

func packageCommand(dir, manager, script string) string {
	var cmd string
	switch manager {
	case "yarn":
		cmd = "yarn " + script
	case "pnpm":
		cmd = "pnpm " + script
	default:
		cmd = "npm run " + script
	}
	return commandPrefix(dir) + cmd
}

func excerptMarkdown(text string, maxChars int) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimRight(line, " \t")
		if strings.TrimSpace(line) == "" && len(out) == 0 {
			continue
		}
		out = append(out, line)
		joined := strings.Join(out, "\n")
		if len(joined) >= maxChars {
			return strings.TrimSpace(joined[:maxChars]) + "\n\n..."
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func writeBullets(b *strings.Builder, values []string, empty ...string) {
	if len(values) == 0 {
		msg := "None."
		if len(empty) > 0 && empty[0] != "" {
			msg = empty[0]
		}
		b.WriteString("- " + msg + "\n")
		return
	}
	for _, value := range values {
		b.WriteString("- " + value + "\n")
	}
}

func writeCodeBullets(b *strings.Builder, values []string, empty string) {
	if len(values) == 0 {
		b.WriteString("- " + empty + "\n")
		return
	}
	for _, value := range values {
		b.WriteString("- `" + value + "`\n")
	}
}

func writePathBullets(b *strings.Builder, values []string, empty string) {
	if len(values) == 0 {
		if empty != "" {
			b.WriteString("- " + empty + "\n")
		}
		return
	}
	for _, value := range values {
		b.WriteString("- `" + value + "`\n")
	}
}
