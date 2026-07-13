package context

import (
	"errors"
	"fmt"
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
	maxTaskBytes = 512 * 1024
)

var (
	ErrInvalidTask = errors.New("invalid task")
	taskIDPattern  = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
)

type Task struct {
	Path          string `json:"path"`
	ID            string `json:"id"`
	Title         string `json:"title"`
	Status        string `json:"status"`
	Priority      string `json:"priority"`
	Created       string `json:"created"`
	Updated       string `json:"updated,omitempty"`
	Agent         string `json:"agent"`
	AgentModel    string `json:"agent_model,omitempty"`
	Reasoning     string `json:"reasoning_effort,omitempty"`
	AgentSession  string `json:"agent_session,omitempty"`
	ResumeCommand string `json:"resume_command,omitempty"`
}

// LoadTasks validates and returns the portable task declarations in canonical
// path order. Runtime session state is deliberately not loaded here.
func LoadTasks(root string) ([]Task, error) {
	pattern := filepath.Join(root, ".struktly", "tasks", "*.md")
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

func loadTask(root, path string) (Task, error) {
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

	allowed := map[string]struct{}{
		"type": {}, "schema": {}, "id": {}, "title": {}, "status": {},
		"priority": {}, "created": {}, "updated": {}, "agent": {},
		"agent_model": {}, "reasoning_effort": {},
		"agent_session": {}, "resume_command": {},
	}
	for key := range metadata {
		if _, ok := allowed[key]; !ok {
			return Task{}, invalidTask(rel, fmt.Errorf("unknown frontmatter field %q", key))
		}
	}
	for _, key := range []string{"type", "schema", "id", "title", "status", "priority", "created", "agent"} {
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
	if !taskIDPattern.MatchString(metadata["id"]) {
		return Task{}, invalidTask(rel, errors.New("id must contain lowercase letters, digits, and single hyphens"))
	}
	filenameID := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if metadata["id"] != filenameID {
		return Task{}, invalidTask(rel, fmt.Errorf("id %q must match filename %q", metadata["id"], filenameID+".md"))
	}
	if !oneOf(metadata["status"], "draft", "ready", "in-progress", "blocked", "done", "canceled") {
		return Task{}, invalidTask(rel, fmt.Errorf("unsupported status %q", metadata["status"]))
	}
	if !oneOf(metadata["priority"], "low", "normal", "high", "critical") {
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
	if err := validateTaskBody(body); err != nil {
		return Task{}, invalidTask(rel, err)
	}

	return Task{
		Path:          rel,
		ID:            metadata["id"],
		Title:         metadata["title"],
		Status:        metadata["status"],
		Priority:      metadata["priority"],
		Created:       metadata["created"],
		Updated:       metadata["updated"],
		Agent:         metadata["agent"],
		AgentModel:    metadata["agent_model"],
		Reasoning:     metadata["reasoning_effort"],
		AgentSession:  metadata["agent_session"],
		ResumeCommand: metadata["resume_command"],
	}, nil
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
