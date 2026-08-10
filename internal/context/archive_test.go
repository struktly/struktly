package context

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func readFile(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func archiveTask(id, status, body string) string {
	return "---\ntype: task\nschema: struktly/task/v1\nid: " + id + "\ntitle: \"X\"\nstatus: " + status + "\n---\n\n" + body
}

// archiveFixture builds a bundle with every direction a link can cross the
// move: out of a moved task, from a live task, from an already-archived task,
// and from a doc two directories away. It also plants the traps: a reserved
// name carrying a finished status, and a draft whose body quotes the status
// vocabulary.
func archiveFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFile(t, root, ".struktly/tasks/index.md", "# Tasks\n\n- [Live tasks](.)\n- [Archive](archive/)\n")
	writeFile(t, root, ".struktly/tasks/log.md", archiveTask("log", "done", "Reserved by OKF, not a task.\n"))
	writeFile(t, root, ".struktly/tasks/finished.md", archiveTask("finished", "done",
		"Depends on [live](live.md) and [earlier](archive/earlier.md).\n"+
			"Read [the plan](../../docs/plan.md) first.\n"))
	writeFile(t, root, ".struktly/tasks/quit.md", archiveTask("quit", "canceled", "Superseded by [live](live.md).\n"))
	writeFile(t, root, ".struktly/tasks/live.md", archiveTask("live", "in-progress",
		"Blocked by [finished](finished.md#done-when), and `.struktly/tasks/finished.md` says why.\n"))
	writeFile(t, root, ".struktly/tasks/draft.md", archiveTask("draft", "draft", "| `done` | Finished. |\nstatus: done\n"))
	writeFile(t, root, ".struktly/tasks/archive/earlier.md", archiveTask("earlier", "done", "Preceded [finished](../finished.md).\n"))
	writeFile(t, root, "docs/plan.md", "See [finished](../.struktly/tasks/finished.md).\n")
	writeFile(t, root, "internal/tasks/tasks.go", "package tasks\n\n// See .struktly/tasks/finished.md for why.\n")
	return root
}

func TestArchiveTasksFilesMisfiledTasksAndRepairsEveryDirection(t *testing.T) {
	root := archiveFixture(t)

	document, err := ArchiveTasks(ArchiveTasksOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if document.Schema != TaskArchiveSchema || document.Clean {
		t.Fatalf("unexpected document header: %+v", document)
	}
	wantMoves := []TaskArchiveMove{
		{From: ".struktly/tasks/finished.md", To: ".struktly/tasks/archive/finished.md"},
		{From: ".struktly/tasks/quit.md", To: ".struktly/tasks/archive/quit.md"},
	}
	if !reflect.DeepEqual(document.Archived, wantMoves) {
		t.Fatalf("archived = %+v, want %+v", document.Archived, wantMoves)
	}
	wantRewrites := []TaskArchiveRewrite{
		{Path: ".struktly/tasks/archive/earlier.md", Links: 1},
		{Path: ".struktly/tasks/finished.md", Links: 3},
		{Path: ".struktly/tasks/live.md", Links: 2},
		{Path: ".struktly/tasks/quit.md", Links: 1},
		{Path: "docs/plan.md", Links: 1},
		{Path: "internal/tasks/tasks.go", Links: 1},
	}
	if !reflect.DeepEqual(document.Rewritten, wantRewrites) {
		t.Fatalf("rewritten = %+v, want %+v", document.Rewritten, wantRewrites)
	}
	assertConforms(t, "task-archive.v1.json", document)

	for _, rel := range []string{".struktly/tasks/finished.md", ".struktly/tasks/quit.md"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); !os.IsNotExist(err) {
			t.Errorf("%s still in the live directory", rel)
		}
	}
	for _, rel := range []string{".struktly/tasks/live.md", ".struktly/tasks/draft.md", ".struktly/tasks/index.md", ".struktly/tasks/log.md"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("%s should not have moved: %v", rel, err)
		}
	}

	cases := []struct {
		name string
		file string
		want []string
	}{
		{"links out of a moved task gain a level", ".struktly/tasks/archive/finished.md",
			[]string{"](../live.md)", "](earlier.md)", "](../../../docs/plan.md)"}},
		{"links into it from a live task gain archive/", ".struktly/tasks/live.md",
			[]string{"](archive/finished.md#done-when)", "`.struktly/tasks/archive/finished.md`"}},
		{"links from an already-archived task lose a level", ".struktly/tasks/archive/earlier.md",
			[]string{"](finished.md)"}},
		{"links from a doc outside the bundle follow", "docs/plan.md",
			[]string{"](../.struktly/tasks/archive/finished.md)"}},
		{"a Go comment citing the path follows", "internal/tasks/tasks.go",
			[]string{".struktly/tasks/archive/finished.md"}},
		{"directory links are left alone", ".struktly/tasks/index.md",
			[]string{"](.)", "](archive/)"}},
	}
	for _, c := range cases {
		content := readFile(t, root, c.file)
		for _, want := range c.want {
			if !strings.Contains(content, want) {
				t.Errorf("%s: %s missing %q in:\n%s", c.name, c.file, want, content)
			}
		}
	}
}

func TestArchiveTasksIsIdempotentAndCheckDoesNotWrite(t *testing.T) {
	root := archiveFixture(t)
	if _, err := ArchiveTasks(ArchiveTasksOptions{Root: root}); err != nil {
		t.Fatal(err)
	}
	before := readFile(t, root, ".struktly/tasks/archive/finished.md")

	// A second sweep has nothing to move, so no link may shift again — the
	// failure mode of a rewrite that re-resolves its own output.
	document, err := ArchiveTasks(ArchiveTasksOptions{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if !document.Clean || len(document.Archived) != 0 || len(document.Rewritten) != 0 {
		t.Errorf("second sweep was not clean: %+v", document)
	}
	assertConforms(t, "task-archive.v1.json", document)
	if after := readFile(t, root, ".struktly/tasks/archive/finished.md"); after != before {
		t.Errorf("second sweep rewrote links again:\n%s", after)
	}

	// Check mode names the misfiled file and the repairs a mutating run would
	// write, without touching either.
	writeFile(t, root, ".struktly/tasks/late.md", archiveTask("late", "done", "See [live](live.md).\n"))
	liveBefore := readFile(t, root, ".struktly/tasks/live.md")
	document, err = ArchiveTasks(ArchiveTasksOptions{Root: root, Check: true})
	if err != nil {
		t.Fatal(err)
	}
	if document.Clean || len(document.Archived) != 1 || document.Archived[0].From != ".struktly/tasks/late.md" {
		t.Errorf("check did not name the misfiled task: %+v", document)
	}
	if len(document.Rewritten) != 1 || document.Rewritten[0] != (TaskArchiveRewrite{Path: ".struktly/tasks/late.md", Links: 1}) {
		t.Errorf("check did not report the repairs a run would write: %+v", document.Rewritten)
	}
	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(".struktly/tasks/late.md"))); err != nil {
		t.Errorf("check mode moved the file: %v", err)
	}
	if readFile(t, root, ".struktly/tasks/late.md") != archiveTask("late", "done", "See [live](live.md).\n") {
		t.Error("check mode rewrote the misfiled task in place")
	}
	if readFile(t, root, ".struktly/tasks/live.md") != liveBefore {
		t.Error("check mode wrote a link repair")
	}
}

func TestArchiveTasksRefusesToOverwriteAnArchivedName(t *testing.T) {
	root := archiveFixture(t)
	writeFile(t, root, ".struktly/tasks/archive/finished.md", archiveTask("finished", "done", "Already filed.\n"))

	_, err := ArchiveTasks(ArchiveTasksOptions{Root: root})
	if !errors.Is(err, ErrTaskAlreadyArchived) {
		t.Fatalf("err = %v, want ErrTaskAlreadyArchived", err)
	}
	// Refused before anything was written: no repair may precede the refusal.
	if !strings.Contains(readFile(t, root, ".struktly/tasks/live.md"), "](finished.md#done-when)") {
		t.Error("a refused sweep still rewrote links")
	}
}

func TestArchiveTasksMissingDirectoryIsClean(t *testing.T) {
	document, err := ArchiveTasks(ArchiveTasksOptions{Root: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if !document.Clean || len(document.Archived) != 0 || len(document.Rewritten) != 0 {
		t.Fatalf("empty repository is not clean: %+v", document)
	}
}

func TestCompleteTaskIsOneAtomicTransition(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".struktly/tasks/ship-it.md",
		"---\ntype: task\nschema: struktly/task/v1\nid: ship-it\ntitle: \"Ship\"\nstatus: in-progress\npriority: high\ncreated: 2026-08-01\nowner_team: core\n---\n\nRead [the plan](../../docs/plan.md) first.\n")
	writeFile(t, root, ".struktly/tasks/live.md", archiveTask("live", "ready",
		"Waits on [ship](ship-it.md#done-when), and `.struktly/tasks/ship-it.md` says why.\n"))
	writeFile(t, root, "docs/plan.md", "See [ship](../.struktly/tasks/ship-it.md).\n")

	document, err := CompleteTask(CompleteTaskOptions{Root: root, ID: "ship-it", Now: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if document.Schema != TaskTransitionSchema || document.Transition != "complete" || document.ID != "ship-it" ||
		document.From != ".struktly/tasks/ship-it.md" || document.To != ".struktly/tasks/archive/ship-it.md" ||
		document.Status != "done" || document.Updated != "2026-08-10" {
		t.Fatalf("unexpected transition document: %+v", document)
	}
	wantRewrites := []TaskArchiveRewrite{
		{Path: ".struktly/tasks/live.md", Links: 2},
		{Path: ".struktly/tasks/ship-it.md", Links: 1},
		{Path: "docs/plan.md", Links: 1},
	}
	if !reflect.DeepEqual(document.Rewritten, wantRewrites) {
		t.Fatalf("rewritten = %+v, want %+v", document.Rewritten, wantRewrites)
	}
	assertConforms(t, "task-transition.v1.json", document)

	if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(".struktly/tasks/ship-it.md"))); !os.IsNotExist(err) {
		t.Error("the live file survived the transition")
	}
	archived := readFile(t, root, ".struktly/tasks/archive/ship-it.md")
	for _, want := range []string{
		"status: done",
		"updated: 2026-08-10",
		"created: 2026-08-01",
		// Fields this parser does not define are preserved (OKF v0.2 §4.1).
		"owner_team: core",
		// Outbound links were re-expressed for the archive location.
		"](../../../docs/plan.md)",
	} {
		if !strings.Contains(archived, want) {
			t.Errorf("archived task missing %q:\n%s", want, archived)
		}
	}
	if !strings.Contains(readFile(t, root, ".struktly/tasks/live.md"), "](archive/ship-it.md#done-when)") {
		t.Error("inbound link from a live task was not repaired")
	}
	if !strings.Contains(readFile(t, root, "docs/plan.md"), "](../.struktly/tasks/archive/ship-it.md)") {
		t.Error("inbound link from a doc was not repaired")
	}
}

func TestCompleteTaskValidationFailureLeavesEverythingUntouched(t *testing.T) {
	root := t.TempDir()
	task := archiveTask("ship-it", "in-progress", "Nothing links here.\n")
	live := archiveTask("live", "ready", "Waits on [ship](ship-it.md).\n")
	writeFile(t, root, ".struktly/tasks/ship-it.md", task)
	writeFile(t, root, ".struktly/tasks/live.md", live)
	// A file occupying the archive path makes the destination directory
	// impossible, before any repair is written.
	writeFile(t, root, ".struktly/tasks/archive", "not a directory")

	if _, err := CompleteTask(CompleteTaskOptions{Root: root, ID: "ship-it"}); err == nil {
		t.Fatal("expected an error when archive/ cannot exist")
	}
	if readFile(t, root, ".struktly/tasks/ship-it.md") != task {
		t.Error("a failed transition modified the live task")
	}
	if readFile(t, root, ".struktly/tasks/live.md") != live {
		t.Error("a failed transition wrote a link repair")
	}
}

func TestCompleteTaskInterruptedRunLeavesTheLiveTaskAndConverges(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses file permissions")
	}
	root := t.TempDir()
	task := archiveTask("ship-it", "in-progress", "Nothing links here.\n")
	writeFile(t, root, ".struktly/tasks/ship-it.md", task)
	writeFile(t, root, ".struktly/tasks/live.md", archiveTask("live", "ready", "Waits on [ship](ship-it.md).\n"))
	archiveDir := filepath.Join(root, filepath.FromSlash(TaskArchiveDir))
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(archiveDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(archiveDir, 0o755) })

	// The destination write fails after inbound repairs. The documented
	// ordering promises the live file is intact and a rerun converges.
	_, err := CompleteTask(CompleteTaskOptions{Root: root, ID: "ship-it"})
	if err == nil {
		t.Skip("filesystem does not enforce the unwritable mode")
	}
	if readFile(t, root, ".struktly/tasks/ship-it.md") != task {
		t.Fatal("an interrupted transition modified the live task")
	}

	if err := os.Chmod(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	document, err := CompleteTask(CompleteTaskOptions{Root: root, ID: "ship-it"})
	if err != nil {
		t.Fatalf("rerun did not converge: %v", err)
	}
	if document.To != ".struktly/tasks/archive/ship-it.md" {
		t.Fatalf("unexpected rerun document: %+v", document)
	}
	if !strings.Contains(readFile(t, root, ".struktly/tasks/archive/ship-it.md"), "status: done") {
		t.Error("rerun did not complete the task")
	}
	if !strings.Contains(readFile(t, root, ".struktly/tasks/live.md"), "](archive/ship-it.md)") {
		t.Error("rerun lost the inbound repair")
	}
}

func TestCompleteTaskRefusesUnknownAndArchivedIDs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, ".struktly/tasks/live.md", archiveTask("live", "ready", "Live.\n"))
	writeFile(t, root, ".struktly/tasks/archive/old.md", archiveTask("old", "done", "Filed.\n"))

	if _, err := CompleteTask(CompleteTaskOptions{Root: root, ID: "nope"}); !errors.Is(err, ErrTaskNotFound) {
		t.Errorf("unknown id: err = %v, want ErrTaskNotFound", err)
	}
	if _, err := CompleteTask(CompleteTaskOptions{Root: root, ID: "old"}); !errors.Is(err, ErrTaskAlreadyArchived) {
		t.Errorf("archived id: err = %v, want ErrTaskAlreadyArchived", err)
	}
}

func TestCompletedTaskContentPreservesEverythingElse(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    []string
	}{
		{
			name:    "updated is replaced in place",
			content: "---\nstatus: ready\nupdated: 2026-01-01\n---\n\nBody.\n",
			want:    []string{"---\nstatus: done\nupdated: 2026-08-10\n---\n\nBody.\n"},
		},
		{
			name:    "updated lands after created",
			content: "---\nstatus: ready\ncreated: 2026-01-01\nnote: keep\n---\n\nBody.\n",
			want:    []string{"---\nstatus: done\ncreated: 2026-01-01\nupdated: 2026-08-10\nnote: keep\n---\n\nBody.\n"},
		},
		{
			name:    "updated lands after status when created is absent",
			content: "---\nstatus: ready\n---\n\nstatus: done in the body stays.\n",
			want:    []string{"---\nstatus: done\nupdated: 2026-08-10\n---\n\nstatus: done in the body stays.\n"},
		},
		{
			name:    "a quoted status is still one line",
			content: "---\nstatus: \"canceled\"\n---\n\nBody.\n",
			want:    []string{"---\nstatus: done\nupdated: 2026-08-10\n---\n\nBody.\n"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := completedTaskContent(c.content, "2026-08-10")
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range c.want {
				if got != want {
					t.Fatalf("completedTaskContent = %q, want %q", got, want)
				}
			}
		})
	}

	if _, err := completedTaskContent("# No frontmatter\n", "2026-08-10"); err == nil {
		t.Error("content without frontmatter did not error")
	}
	if _, err := completedTaskContent("---\ntitle: no status\n---\n\nBody.\n", "2026-08-10"); err == nil {
		t.Error("frontmatter without status did not error")
	}
}
