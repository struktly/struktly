package context

import (
	"strings"
	"testing"
)

const validTaskDocument = `---
type: task
schema: struktly/task/v1
id: add-timeout
title: "Add request timeout"
status: ready
priority: normal
created: 2026-07-13
agent: unassigned
---

# Add request timeout

## Pick up this task

Start here.

## Objective

Add the timeout.

## Constraints

- Keep compatibility.

## Required outcomes

- [ ] Tests pass.

## Execution plan

1. Implement it.

## Definition of done

The timeout works.
`

func TestLoadTasksValidatesCanonicalTask(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".struktly/tasks/add-timeout.md", validTaskDocument)

	tasks, err := LoadTasks(root)
	if err != nil {
		t.Fatalf("LoadTasks returned error: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("tasks = %#v, want one", tasks)
	}
	task := tasks[0]
	if task.Path != ".struktly/tasks/add-timeout.md" || task.ID != "add-timeout" || task.Status != "ready" || task.Agent != "unassigned" {
		t.Fatalf("unexpected task: %#v", task)
	}
}

func TestLoadTasksRejectsMalformedTasks(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		content string
		want    string
	}{
		{name: "unknown field", path: "add-timeout.md", content: strings.Replace(validTaskDocument, "agent: unassigned", "agent: unassigned\nowner: team", 1), want: "unknown frontmatter field"},
		{name: "noncanonical status", path: "add-timeout.md", content: strings.Replace(validTaskDocument, "status: ready", "status: completed", 1), want: "unsupported status"},
		{name: "filename mismatch", path: "different.md", content: validTaskDocument, want: "must match filename"},
		{name: "missing heading", path: "add-timeout.md", content: strings.Replace(validTaskDocument, "## Constraints", "## Notes", 1), want: "required heading"},
		{name: "partial handoff", path: "add-timeout.md", content: strings.Replace(validTaskDocument, "agent: unassigned", "agent: codex\nagent_session: session-1", 1), want: "declared together"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, root, ".struktly/tasks/"+test.path, test.content)
			_, err := LoadTasks(root)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("LoadTasks error = %v, want %q", err, test.want)
			}
		})
	}
}
