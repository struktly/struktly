package context

// Task lifecycle on disk. The task format states that frontmatter `status` is
// the single source of truth and that location is an invariant derived from
// it: the live .struktly/tasks/ directory must not contain done or canceled
// tasks, which live under archive/ instead. Keeping that true by hand is what
// stopped happening in the repository this was ported from — sixty-five
// finished contracts piled up beside two live ones — and the move is only half
// the work, because it changes every relative link that crosses it.
//
// CompleteTask is the transition: one invocation sets the status, files the
// task under archive/, and repairs links. ArchiveTasks is the sweep: it files
// tasks that are already finished but misfiled, which is the migration and
// cleanup case, and in check mode it is the conformance gate for the invariant.
//
// Link repair is one rule applied uniformly: resolve every Markdown link
// target against the file's directory before the move, then re-express it from
// its directory after the move. That covers links out of a moved task, links
// into it from live tasks, from already-archived tasks, and from docs — the
// first hand-run archive sweep did neither direction and left an
// implementation plan pointing at a file that was no longer there.

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/struktly/struktly/internal/files"
)

const (
	TaskArchiveSchema    = "struktly/task-archive/v1"
	TaskTransitionSchema = "struktly/task-transition/v1"
	// TaskArchiveDir is where the task format files finished tasks.
	TaskArchiveDir = tasksDir + "/archive"
)

var (
	// ErrTaskNotFound reports that no live task declares the requested id.
	ErrTaskNotFound = errors.New("task not found")
	// ErrTaskAlreadyArchived reports a transition on a task that is already
	// filed under archive/, or whose archive slot is occupied.
	ErrTaskAlreadyArchived = errors.New("task already archived")
)

// finishedTaskStatuses lists the statuses whose place is archive/. It is
// deliberately this narrow: struktly/task/v1 admits no other finished
// spelling, and a file using one is invalid rather than done.
var finishedTaskStatuses = map[string]bool{"done": true, "canceled": true}

// taskLinkPattern matches an inline Markdown link target, with an optional
// title.
var taskLinkPattern = regexp.MustCompile(`\]\(([^)\s]+)((?:\s+"[^"]*")?)\)`)

type ArchiveTasksOptions struct {
	Root string
	// Check reports without touching the tree.
	Check bool
}

type CompleteTaskOptions struct {
	Root string
	// ID is resolved against the frontmatter id of live tasks.
	ID string
	// Now stamps the task's updated field; zero means the current time.
	Now time.Time
}

type TaskArchiveMove struct {
	From string `json:"from"`
	To   string `json:"to"`
}

type TaskArchiveRewrite struct {
	Path  string `json:"path"`
	Links int    `json:"links"`
}

// TaskArchiveDocument is the machine contract for the archive sweep. In check
// mode it reports what a mutating run would do, without writing any of it.
type TaskArchiveDocument struct {
	Schema    string               `json:"schema"`
	Root      string               `json:"root"`
	Archived  []TaskArchiveMove    `json:"archived"`
	Rewritten []TaskArchiveRewrite `json:"rewritten"`
	// Clean reports that no finished task sat outside archive/ when the
	// command ran: the location invariant held.
	Clean bool `json:"clean"`
}

// TaskTransitionDocument is the machine contract for a lifecycle transition.
// The transition field names which one, so future transitions can share the
// schema instead of each minting a new document.
type TaskTransitionDocument struct {
	Schema     string               `json:"schema"`
	Root       string               `json:"root"`
	Transition string               `json:"transition"`
	ID         string               `json:"id"`
	From       string               `json:"from"`
	To         string               `json:"to"`
	Status     string               `json:"status"`
	Updated    string               `json:"updated"`
	Rewritten  []TaskArchiveRewrite `json:"rewritten"`
}

// ArchiveTasks files every finished task that is misfiled in the live
// directory, repairing links in both directions, and reports what it did. With
// Check set it computes the same report and writes nothing.
func ArchiveTasks(opts ArchiveTasksOptions) (TaskArchiveDocument, error) {
	root, err := files.CleanRoot(opts.Root)
	if err != nil {
		return TaskArchiveDocument{}, err
	}
	document := TaskArchiveDocument{
		Schema:    TaskArchiveSchema,
		Root:      root,
		Archived:  []TaskArchiveMove{},
		Rewritten: []TaskArchiveRewrite{},
	}
	misfiled, err := misfiledTasks(root)
	if err != nil {
		return TaskArchiveDocument{}, err
	}
	if len(misfiled) == 0 {
		document.Clean = true
		return document, nil
	}

	moves := make(map[string]string, len(misfiled))
	for _, rel := range misfiled {
		moves[rel] = TaskArchiveDir + "/" + path.Base(rel)
		document.Archived = append(document.Archived, TaskArchiveMove{From: rel, To: moves[rel]})
	}
	if !opts.Check {
		// A rename onto an occupied archive slot would overwrite it, so refuse
		// before anything — link repairs included — is written.
		for _, rel := range misfiled {
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(moves[rel]))); err == nil {
				return TaskArchiveDocument{}, fmt.Errorf("%w: %s already exists", ErrTaskAlreadyArchived, moves[rel])
			}
		}
	}

	// Link targets are resolved against the tree as it stands, before any file
	// moves, so check mode computes exactly the repairs a mutating run writes.
	document.Rewritten, err = repairTaskLinks(root, moves, !opts.Check, "")
	if err != nil {
		return TaskArchiveDocument{}, err
	}
	if opts.Check {
		return document, nil
	}

	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(TaskArchiveDir)), 0o755); err != nil {
		return TaskArchiveDocument{}, err
	}
	for _, rel := range misfiled {
		from := filepath.Join(root, filepath.FromSlash(rel))
		to := filepath.Join(root, filepath.FromSlash(moves[rel]))
		if err := os.Rename(from, to); err != nil {
			return TaskArchiveDocument{}, err
		}
	}
	return document, nil
}

// CompleteTask is the atomic done transition: it sets `status: done` and
// today's `updated` date in the task's frontmatter, files the task under
// archive/, and repairs links in both directions, in one invocation.
//
// Ordering, so that a failure at any step leaves the original live file
// intact: (1) resolve the id and compute the completed content, (2) ensure the
// archive slot exists and is free, (3) repair inbound links across the
// repository, (4) write the completed content at the archive path, (5) remove
// the live file. The live file is never rewritten in place. A run interrupted
// after (3) has re-pointed links at an archive path that does not exist yet;
// rerunning the command converges, because the repairs are recognised as
// already made and the move still happens.
func CompleteTask(opts CompleteTaskOptions) (TaskTransitionDocument, error) {
	root, err := files.CleanRoot(opts.Root)
	if err != nil {
		return TaskTransitionDocument{}, err
	}
	from, content, err := resolveLiveTask(root, opts.ID)
	if err != nil {
		return TaskTransitionDocument{}, err
	}
	to := TaskArchiveDir + "/" + path.Base(from)
	updated := files.NormalizeNow(opts.Now).Format(time.DateOnly)
	completed, err := completedTaskContent(content, updated)
	if err != nil {
		return TaskTransitionDocument{}, invalidTask(from, err)
	}
	document := TaskTransitionDocument{
		Schema:     TaskTransitionSchema,
		Root:       root,
		Transition: "complete",
		ID:         opts.ID,
		From:       from,
		To:         to,
		Status:     "done",
		Updated:    updated,
		Rewritten:  []TaskArchiveRewrite{},
	}

	if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(TaskArchiveDir)), 0o755); err != nil {
		return TaskTransitionDocument{}, err
	}
	absTo := filepath.Join(root, filepath.FromSlash(to))
	if _, err := os.Stat(absTo); err == nil {
		return TaskTransitionDocument{}, fmt.Errorf("%w: %s already exists", ErrTaskAlreadyArchived, to)
	}

	// The task's own outbound links are re-expressed in memory and land in the
	// single destination write; the walk below skips the live file so it is
	// never rewritten in place.
	moves := map[string]string{from: to}
	exists := func(rel string) bool {
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		return err == nil
	}
	completed, outbound := rewriteTaskLinks(completed, from, moves, exists)
	document.Rewritten, err = repairTaskLinks(root, moves, true, from)
	if err != nil {
		return TaskTransitionDocument{}, err
	}
	if outbound > 0 {
		document.Rewritten = append(document.Rewritten, TaskArchiveRewrite{Path: from, Links: outbound})
		sort.Slice(document.Rewritten, func(i, j int) bool {
			return document.Rewritten[i].Path < document.Rewritten[j].Path
		})
	}

	if err := os.WriteFile(absTo, []byte(completed), 0o644); err != nil {
		return TaskTransitionDocument{}, err
	}
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(from))); err != nil {
		return TaskTransitionDocument{}, err
	}
	return document, nil
}

// resolveLiveTask finds the live task declaring the requested id and returns
// its repository-relative path and content. The id is the task's declared
// identity, so lookup reads frontmatter rather than trusting filenames; an id
// already filed under archive/ is reported as such rather than as unknown.
func resolveLiveTask(root, id string) (string, string, error) {
	rel, content, err := taskByID(root, tasksDir, id)
	if err != nil {
		return "", "", err
	}
	if rel != "" {
		return rel, content, nil
	}
	archived, _, err := taskByID(root, TaskArchiveDir, id)
	if err != nil {
		return "", "", err
	}
	if archived != "" {
		return "", "", fmt.Errorf("%w: %q is already filed at %s", ErrTaskAlreadyArchived, id, archived)
	}
	return "", "", fmt.Errorf("%w: no live task under %s/ declares id %q", ErrTaskNotFound, tasksDir, id)
}

func taskByID(root, dir, id string) (string, string, error) {
	rels, err := taskFiles(root, dir)
	if err != nil {
		return "", "", err
	}
	for _, rel := range rels {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return "", "", err
		}
		metadata, _, err := parseTaskFrontmatter(string(data))
		if err != nil {
			// A file whose frontmatter does not parse declares no id; `tasks`
			// already reports it as invalid.
			continue
		}
		if metadata["id"] == id {
			return rel, string(data), nil
		}
	}
	return "", "", nil
}

// misfiledTasks lists repository-relative paths of finished tasks still in the
// live directory — the files violating the location invariant — in a stable
// order.
func misfiledTasks(root string) ([]string, error) {
	rels, err := taskFiles(root, tasksDir)
	if err != nil {
		return nil, err
	}
	var misfiled []string
	for _, rel := range rels {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, err
		}
		metadata, _, err := parseTaskFrontmatter(string(data))
		if err != nil {
			// No frontmatter means no status: the file is invalid rather than
			// finished, and moving it is not this sweep's call to make.
			continue
		}
		if finishedTaskStatuses[metadata["status"]] {
			misfiled = append(misfiled, rel)
		}
	}
	return misfiled, nil
}

// taskFiles lists the task-shaped files directly inside dir in name order:
// Markdown, not reserved by OKF v0.2 §8 or §9, and regular — the same refusal
// DiscoverTasks makes, because archiving must not move or rewrite through a
// symlink. A missing directory is simply empty; an unreadable one is an error,
// not an empty result, because "nothing to file" must not be claimed for a
// directory that was never examined.
func taskFiles(root, dir string) ([]string, error) {
	abs := filepath.Join(root, filepath.FromSlash(dir))
	info, err := os.Lstat(abs)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("inspect %s: %w", dir, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, fmt.Errorf("%s must be a directory, not a symlink", dir)
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	var rels []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || isReservedOKFName(name) || !strings.HasSuffix(name, ".md") || !entry.Type().IsRegular() {
			continue
		}
		rels = append(rels, dir+"/"+name)
	}
	return rels, nil
}

// completedTaskContent rewrites the leading frontmatter block only: status
// becomes done and updated becomes the given date, with every other line kept
// as written — OKF v0.2 §4.1 asks a consumer to preserve fields it does not
// define, and the body is not this function's to touch. Line endings are
// normalized the same way the parser reads them.
func completedTaskContent(content, updated string) (string, error) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	rest, ok := strings.CutPrefix(content, "---\n")
	if !ok {
		return "", errors.New("must start with YAML frontmatter")
	}
	block, body, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		return "", errors.New("frontmatter is not closed")
	}
	lines := strings.Split(block, "\n")
	statusAt, updatedAt, createdAt := -1, -1, -1
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "status:"):
			statusAt = i
		case strings.HasPrefix(line, "updated:"):
			updatedAt = i
		case strings.HasPrefix(line, "created:"):
			createdAt = i
		}
	}
	if statusAt < 0 {
		return "", errors.New("frontmatter declares no status")
	}
	lines[statusAt] = "status: done"
	updatedLine := "updated: " + updated
	switch {
	case updatedAt >= 0:
		lines[updatedAt] = updatedLine
	case createdAt >= 0:
		lines = slices.Insert(lines, createdAt+1, updatedLine)
	default:
		lines = slices.Insert(lines, statusAt+1, updatedLine)
	}
	return "---\n" + strings.Join(lines, "\n") + "\n---\n" + body, nil
}

// repairTaskLinks rewrites every reference to a moved file across the
// repository's Markdown and Go sources, skipping the moved file named by skip,
// and reports the repository-relative paths it changed with how many
// references each rewrite touched. With write false it computes the same
// report and leaves the tree alone.
func repairTaskLinks(root string, moves map[string]string, write bool, skip string) ([]TaskArchiveRewrite, error) {
	rewritten := []TaskArchiveRewrite{}
	exists := func(rel string) bool {
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		return err == nil
	}
	err := filepath.WalkDir(root, func(abs string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			// The root itself may carry an ignored name; only descendants are
			// skipped.
			if abs != root && slices.Contains(files.DefaultIgnoredDirs, entry.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() {
			return nil
		}
		name := entry.Name()
		isMarkdown := strings.HasSuffix(name, ".md")
		if !isMarkdown && !strings.HasSuffix(name, ".go") {
			return nil
		}
		relOS, err := filepath.Rel(root, abs)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(relOS)
		if rel == skip {
			return nil
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return err
		}
		next := string(data)
		references := 0
		if isMarkdown {
			next, references = rewriteTaskLinks(next, rel, moves, exists)
		}
		// Prose and comments cite tasks as repository-root paths in backticks
		// rather than as links; those move too.
		for from, to := range moves {
			if count := strings.Count(next, from); count > 0 {
				next = strings.ReplaceAll(next, from, to)
				references += count
			}
		}
		if next == string(data) {
			return nil
		}
		if write {
			if err := os.WriteFile(abs, []byte(next), 0o644); err != nil {
				return err
			}
		}
		rewritten = append(rewritten, TaskArchiveRewrite{Path: rel, Links: references})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(rewritten, func(i, j int) bool { return rewritten[i].Path < rewritten[j].Path })
	return rewritten, nil
}

// rewriteTaskLinks re-expresses the Markdown links in a file that lives at
// rel, given that every path in moves is about to change location — including,
// possibly, rel itself — and reports how many it rewrote. exists reports
// whether a repository-relative path resolves today; links that do not are
// left exactly as written, because a link that is already broken is not this
// command's to guess at.
func rewriteTaskLinks(content, rel string, moves map[string]string, exists func(string) bool) (string, int) {
	fromDir := path.Dir(rel)
	toDir := path.Dir(movedTaskPath(rel, moves))
	rewritten := 0
	next := taskLinkPattern.ReplaceAllStringFunc(content, func(match string) string {
		groups := taskLinkPattern.FindStringSubmatch(match)
		target, title := groups[1], groups[2]
		targetPath, fragment, hasFragment := strings.Cut(target, "#")
		if hasFragment {
			fragment = "#" + fragment
		}
		// Only file references to task documents can cross this move, and
		// restricting to them keeps directory links such as `archive/` intact.
		if !strings.HasSuffix(targetPath, ".md") || strings.Contains(targetPath, "://") || path.IsAbs(targetPath) {
			return match
		}
		resolved := path.Join(fromDir, targetPath)
		if !exists(resolved) {
			return match
		}
		relTarget, err := filepath.Rel(toDir, movedTaskPath(resolved, moves))
		if err != nil {
			return match
		}
		if slash := filepath.ToSlash(relTarget); slash != targetPath {
			rewritten++
			return "](" + slash + fragment + title + ")"
		}
		return match
	})
	return next, rewritten
}

func movedTaskPath(rel string, moves map[string]string) string {
	if to, ok := moves[rel]; ok {
		return to
	}
	return rel
}
