package context

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/struktly/struktly/internal/files"
)

const (
	TaskSchema   = "struktly/task/v1"
	TasksSchema  = "struktly/tasks/v1"
	tasksDir     = ".struktly/tasks"
	maxTaskBytes = 512 * 1024
)

var (
	ErrInvalidTask      = errors.New("invalid task")
	taskIDPattern       = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
	legacyTaskIDPattern = regexp.MustCompile(`^[a-z0-9]+(?:[.-][a-z0-9]+)*$`)
	taskCodeSpan        = regexp.MustCompile("`([^`\n]+)`")
)

type TaskContract struct {
	Outcome        string   `json:"outcome"`
	DoneWhen       []string `json:"done_when"`
	NonGoals       []string `json:"non_goals"`
	RequiredChecks []string `json:"required_checks"`
}

type Task struct {
	Path               string       `json:"path"`
	ID                 string       `json:"id"`
	Title              string       `json:"title"`
	Status             string       `json:"status"`
	Priority           string       `json:"priority"`
	Created            string       `json:"created"`
	Updated            string       `json:"updated,omitempty"`
	Agent              string       `json:"agent"`
	AgentModel         string       `json:"agent_model,omitempty"`
	Reasoning          string       `json:"reasoning_effort,omitempty"`
	AgentSession       string       `json:"agent_session,omitempty"`
	ResumeCommand      string       `json:"resume_command,omitempty"`
	Contract           TaskContract `json:"contract"`
	SHA256             string       `json:"sha256"`
	CompatibilityNotes []string     `json:"compatibility_notes,omitempty"`
	// Extensions carries frontmatter keys this parser does not define. OKF v0.2
	// §4.1 requires consumers to tolerate unrecognized fields and asks them to
	// preserve those fields when round-tripping; dropping them silently would
	// make a producer's own annotations disappear on every read.
	Extensions map[string]string `json:"extensions,omitempty"`
}

type InvalidTaskFile struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type TasksDocument struct {
	Schema  string            `json:"schema"`
	Tasks   []Task            `json:"tasks"`
	Invalid []InvalidTaskFile `json:"invalid"`
}

// LoadTasks validates and returns the portable task declarations in canonical
// path order. Runtime session state is deliberately not loaded here.
func LoadTasks(root string) ([]Task, error) {
	pattern := filepath.Join(root, filepath.FromSlash(tasksDir), "*.md")
	paths, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	sort.Strings(paths)
	tasks := make([]Task, 0, len(paths))
	for _, path := range paths {
		task, err := loadTask(root, path)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, task)
	}
	return tasks, nil
}

// DiscoverTasks returns every safely readable task declaration in canonical
// path order. Unlike LoadTasks, body headings are interpreted when present but
// are not required: historical repository task prose remains discoverable, and
// malformed files are reported without hiding valid siblings.
func DiscoverTasks(root string) (TasksDocument, error) {
	root, err := files.CleanRoot(root)
	if err != nil {
		return TasksDocument{}, err
	}
	dir := filepath.Join(root, filepath.FromSlash(tasksDir))
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return emptyTasksDocument(), nil
	}
	if err != nil {
		return TasksDocument{}, fmt.Errorf("inspect %s: %w", tasksDir, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return TasksDocument{}, fmt.Errorf("%s must be a directory, not a symlink", tasksDir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return TasksDocument{}, fmt.Errorf("read %s: %w", tasksDir, err)
	}

	document := emptyTasksDocument()
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			continue
		}
		// OKF v0.2 §8 and §9 reserve index.md and log.md: a directory listing
		// and an update history. They are not concept documents, so they carry
		// no `type` and conformance does not ask them to.
		//
		// Deliberately a fixed list rather than "skip anything without
		// frontmatter": an author who forgets frontmatter on a real task must
		// still be told, because a task that silently fails to appear is the
		// worse failure of the two.
		if isReservedOKFName(entry.Name()) {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		rel := files.RelPath(root, path)
		task, err := loadTaskFile(root, path, false)
		if err != nil {
			document.Invalid = append(document.Invalid, InvalidTaskFile{Path: rel, Reason: taskErrorReason(rel, err)})
			continue
		}
		document.Tasks = append(document.Tasks, task)
	}
	sort.Slice(document.Tasks, func(i, j int) bool { return document.Tasks[i].Path < document.Tasks[j].Path })
	sort.Slice(document.Invalid, func(i, j int) bool { return document.Invalid[i].Path < document.Invalid[j].Path })
	return document, nil
}

// isReservedOKFName reports whether a filename is reserved by OKF v0.2 rather
// than being a concept document. §8 defines index.md, §9 defines log.md.
func isReservedOKFName(name string) bool {
	return strings.EqualFold(name, "index.md") || strings.EqualFold(name, "log.md")
}

func emptyTasksDocument() TasksDocument {
	return TasksDocument{Schema: TasksSchema, Tasks: []Task{}, Invalid: []InvalidTaskFile{}}
}

func loadTask(root, path string) (Task, error) {
	return loadTaskFile(root, path, true)
}

func loadTaskFile(root, path string, validateBody bool) (Task, error) {
	rel := files.RelPath(root, path)
	info, err := os.Lstat(path)
	if err != nil {
		return Task{}, invalidTask(rel, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Task{}, invalidTask(rel, errors.New("must be a regular file, not a symlink"))
	}
	if info.Size() > maxTaskBytes {
		return Task{}, invalidTask(rel, fmt.Errorf("exceeds %d-byte limit", maxTaskBytes))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Task{}, invalidTask(rel, errors.New("cannot read file"))
	}
	metadata, body, err := parseTaskFrontmatter(string(data))
	if err != nil {
		return Task{}, invalidTask(rel, err)
	}

	// Unknown frontmatter keys are carried, not rejected. A task file is an OKF
	// concept document, and OKF v0.2 §4.1 is explicit: producers may add keys,
	// and consumers "MUST NOT reject documents with unrecognized fields" and
	// SHOULD preserve them when round-tripping.
	//
	// This used to be an allowlist. It meant a repository could not annotate its
	// own tasks with anything this parser had not been taught, and the failure
	// was the worst kind: the file vanished from the task list rather than
	// keeping the fields it did understand.
	known := map[string]struct{}{
		"type": {}, "schema": {}, "id": {}, "title": {}, "status": {},
		"priority": {}, "created": {}, "updated": {}, "agent": {},
		"agent_model": {}, "reasoning_effort": {},
		"agent_session": {}, "resume_command": {},
	}
	extensions := map[string]string{}
	for key, value := range metadata {
		if _, ok := known[key]; !ok {
			extensions[key] = value
		}
	}
	// Required is what makes a file a task at all: what it is, which contract it
	// speaks, its identity, what it is called, and where it stands. Everything
	// else describes a task rather than constituting one.
	//
	// priority, created and agent used to be required. That forced every author
	// to answer questions a fresh contract cannot honestly answer — an unstarted
	// task has no agent, and "unassigned" is a magic string standing in for a
	// field that should simply be absent. The cost was not worse metadata but
	// invisible tasks: a file missing one is not a task at all, so a repository
	// can carry a shelf of carefully written contracts that no tool will show.
	for _, key := range []string{"type", "schema", "id", "title", "status"} {
		if strings.TrimSpace(metadata[key]) == "" {
			return Task{}, invalidTask(rel, fmt.Errorf("frontmatter field %q is required", key))
		}
	}
	if metadata["type"] != "task" {
		return Task{}, invalidTask(rel, errors.New(`type must be "task"`))
	}
	if metadata["schema"] != TaskSchema {
		return Task{}, invalidTask(rel, fmt.Errorf("schema must be %q", TaskSchema))
	}
	compatibilityNotes := []string{}
	if !taskIDPattern.MatchString(metadata["id"]) {
		if validateBody || !legacyTaskIDPattern.MatchString(metadata["id"]) {
			return Task{}, invalidTask(rel, errors.New("id must contain lowercase letters, digits, and single hyphens"))
		}
		compatibilityNotes = append(compatibilityNotes, "Compatibility import: canonical task/v1 validation rejects this historical dotted task ID.")
	}
	filenameID := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if metadata["id"] != filenameID {
		return Task{}, invalidTask(rel, fmt.Errorf("id %q must match filename %q", metadata["id"], filenameID+".md"))
	}
	if !oneOf(metadata["status"], "draft", "ready", "in-progress", "blocked", "done", "canceled") {
		return Task{}, invalidTask(rel, fmt.Errorf("unsupported status %q", metadata["status"]))
	}
	// Absent priority is a task nobody has ranked, which is an ordinary state and
	// not an error. "medium" replaces "normal": every author who reached for a
	// middle rung unprompted wrote "medium", and low/medium/high is the ladder
	// people already think in.
	if metadata["priority"] != "" && !oneOf(metadata["priority"], "low", "medium", "high", "critical") {
		return Task{}, invalidTask(rel, fmt.Errorf("unsupported priority %q", metadata["priority"]))
	}
	for _, key := range []string{"created", "updated"} {
		if metadata[key] == "" {
			continue
		}
		if _, err := time.Parse(time.DateOnly, metadata[key]); err != nil {
			return Task{}, invalidTask(rel, fmt.Errorf("%s must be YYYY-MM-DD", key))
		}
	}
	if strings.ContainsAny(metadata["resume_command"], "\r\n") {
		return Task{}, invalidTask(rel, errors.New("resume_command must be a single line"))
	}
	if (metadata["agent_session"] == "") != (metadata["resume_command"] == "") {
		return Task{}, invalidTask(rel, errors.New("agent_session and resume_command must be declared together"))
	}
	bodyValidationErr := validateTaskBody(body)
	if validateBody {
		if bodyValidationErr != nil {
			return Task{}, invalidTask(rel, bodyValidationErr)
		}
	}
	contract, bodyCompatibilityNotes := parseTaskContract(body)
	compatibilityNotes = append(compatibilityNotes, bodyCompatibilityNotes...)
	if !validateBody && bodyValidationErr != nil {
		compatibilityNotes = append(compatibilityNotes, "Compatibility import: canonical task/v1 validation would reject this body: "+bodyValidationErr.Error())
	}
	digest := sha256.Sum256(data)

	return Task{
		Path:               rel,
		ID:                 metadata["id"],
		Title:              metadata["title"],
		Status:             metadata["status"],
		Priority:           metadata["priority"],
		Created:            metadata["created"],
		Updated:            metadata["updated"],
		Agent:              metadata["agent"],
		AgentModel:         metadata["agent_model"],
		Reasoning:          metadata["reasoning_effort"],
		AgentSession:       metadata["agent_session"],
		ResumeCommand:      metadata["resume_command"],
		Contract:           contract,
		SHA256:             hex.EncodeToString(digest[:]),
		CompatibilityNotes: compatibilityNotes,
		Extensions:         extensions,
	}, nil
}

func parseTaskContract(body string) (TaskContract, []string) {
	sections := parseTaskSections(body)
	notes := []string{}
	contract := TaskContract{
		DoneWhen:       []string{},
		NonGoals:       []string{},
		RequiredChecks: []string{},
	}
	contract.Outcome = firstTaskSection(sections, "objective")
	if contract.Outcome == "" {
		if mission := firstTaskSection(sections, "mission"); mission != "" {
			contract.Outcome = mission
			notes = append(notes, "Mapped historical Mission heading to Objective.")
		} else if outcome := firstTaskSection(sections, "outcome"); outcome != "" {
			contract.Outcome = outcome
			notes = append(notes, "Mapped historical Outcome heading to Objective.")
		}
	}
	done := firstTaskSection(sections, "required outcomes")
	if done == "" {
		for _, historical := range []string{"success", "success criteria", "acceptance criteria", "done when", "definition of done", "requirements"} {
			if value := firstTaskSection(sections, historical); value != "" {
				done = value
				notes = append(notes, "Mapped historical "+taskHeading(historical)+" heading to Required outcomes.")
				break
			}
		}
	}
	if done != "" {
		contract.DoneWhen = taskItemsOrText(done)
	}
	if nonGoals := firstTaskSection(sections, "non-goals"); nonGoals != "" {
		contract.NonGoals = taskItemsOrText(nonGoals)
	}
	checkText := strings.Join([]string{
		sections["definition of done"],
		sections["verification"],
		sections["required checks"],
	}, "\n")
	seen := map[string]bool{}
	for _, match := range taskCodeSpan.FindAllStringSubmatch(checkText, -1) {
		value := strings.TrimSpace(match[1])
		if value != "" && !seen[value] {
			seen[value] = true
			contract.RequiredChecks = append(contract.RequiredChecks, value)
		}
	}
	return contract, notes
}

func taskHeading(value string) string {
	words := strings.Fields(value)
	for i := range words {
		words[i] = strings.ToUpper(words[i][:1]) + words[i][1:]
	}
	return strings.Join(words, " ")
}

func parseTaskSections(body string) map[string]string {
	sections := map[string]string{}
	current := ""
	lines := []string{}
	flush := func() {
		if current != "" {
			sections[current] = strings.TrimSpace(strings.Join(lines, "\n"))
		}
	}
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "## ") {
			flush()
			current = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "## ")))
			lines = nil
			continue
		}
		if current != "" {
			lines = append(lines, line)
		}
	}
	flush()
	return sections
}

func firstTaskSection(sections map[string]string, names ...string) string {
	for _, name := range names {
		if value := strings.TrimSpace(sections[name]); value != "" {
			return value
		}
	}
	return ""
}

func taskItemsOrText(text string) []string {
	items := []string{}
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		item := ""
		switch {
		case strings.HasPrefix(trimmed, "- [ ] "), strings.HasPrefix(trimmed, "- [x] "), strings.HasPrefix(trimmed, "- [X] "):
			item = strings.TrimSpace(trimmed[6:])
		case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "):
			item = strings.TrimSpace(trimmed[2:])
		}
		if item != "" {
			items = append(items, item)
			continue
		}
		if len(items) > 0 && trimmed != "" {
			items[len(items)-1] += " " + trimmed
		}
	}
	if len(items) > 0 {
		return items
	}
	if text = strings.TrimSpace(text); text != "" {
		return []string{text}
	}
	return []string{}
}

func taskErrorReason(path string, err error) string {
	prefix := ErrInvalidTask.Error() + ": " + path + ": "
	return strings.TrimPrefix(err.Error(), prefix)
}

func parseTaskFrontmatter(content string) (map[string]string, string, error) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	rest, ok := strings.CutPrefix(content, "---\n")
	if !ok {
		return nil, "", errors.New("must start with YAML frontmatter")
	}
	frontmatter, body, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		return nil, "", errors.New("frontmatter is not closed")
	}
	metadata := make(map[string]string)
	for _, line := range strings.Split(frontmatter, "\n") {
		key, raw, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(key) != key || key == "" {
			return nil, "", fmt.Errorf("invalid frontmatter line %q", line)
		}
		if _, duplicate := metadata[key]; duplicate {
			return nil, "", fmt.Errorf("duplicate frontmatter field %q", key)
		}
		value, err := taskFrontmatterValue(strings.TrimSpace(raw))
		if err != nil {
			return nil, "", fmt.Errorf("frontmatter field %q: %w", key, err)
		}
		metadata[key] = value
	}
	return metadata, strings.TrimLeft(body, "\n"), nil
}

func taskFrontmatterValue(raw string) (string, error) {
	if !strings.HasPrefix(raw, `"`) {
		return raw, nil
	}
	value, err := strconv.Unquote(raw)
	if err != nil {
		return "", errors.New("invalid quoted value")
	}
	return value, nil
}

func validateTaskBody(body string) error {
	required := []string{
		"## Pick up this task",
		"## Objective",
		"## Constraints",
		"## Required outcomes",
		"## Execution plan",
		"## Definition of done",
	}
	positions := make(map[string]int, len(required))
	for lineNumber, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		for _, heading := range required {
			if line == heading {
				if _, duplicate := positions[heading]; duplicate {
					return fmt.Errorf("heading %q must appear once", heading)
				}
				positions[heading] = lineNumber
			}
		}
	}
	previous := -1
	for _, heading := range required {
		position, ok := positions[heading]
		if !ok {
			return fmt.Errorf("required heading %q is missing", heading)
		}
		if position < previous {
			return errors.New("required task headings are out of order")
		}
		previous = position
	}
	return nil
}

func invalidTask(path string, err error) error {
	return fmt.Errorf("%w: %s: %v", ErrInvalidTask, path, err)
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
