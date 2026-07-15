package context

import (
	stdcontext "context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/struktly/struktly/internal/files"
)

var (
	ErrNotGitRepository  = errors.New("not a Git repository")
	ErrRepositoryChanged = errors.New("repository changed while building context")
)

type Repository struct {
	Name         string `json:"name"`
	Identity     string `json:"identity"`
	VCS          string `json:"vcs"`
	Root         string `json:"root"`
	Branch       string `json:"branch,omitempty"`
	HeadRevision string `json:"head_revision"`
	BaseRevision string `json:"base_revision,omitempty"`

	absoluteRoot string
}

func (r Repository) AbsoluteRoot() string {
	return r.absoluteRoot
}

func ResolveRepository(ctx stdcontext.Context, requestedRoot string) (Repository, error) {
	root, err := files.CleanRoot(requestedRoot)
	if err != nil {
		return Repository{}, err
	}
	gitRoot, err := gitOutput(ctx, root, "rev-parse", "--show-toplevel")
	if err != nil {
		if ctx.Err() != nil {
			return Repository{}, ctx.Err()
		}
		return Repository{}, fmt.Errorf("%w: %s", ErrNotGitRepository, root)
	}
	gitRoot, err = filepath.EvalSymlinks(gitRoot)
	if err != nil {
		return Repository{}, fmt.Errorf("resolve Git root: %w", err)
	}
	head, err := gitOutput(ctx, gitRoot, "rev-parse", "HEAD")
	if err != nil {
		return Repository{}, fmt.Errorf("resolve Git HEAD: %w", err)
	}
	branch, _ := gitOutput(ctx, gitRoot, "symbolic-ref", "--quiet", "--short", "HEAD")
	roots, err := gitLines(ctx, gitRoot, "rev-list", "--max-parents=0", "HEAD")
	if err != nil || len(roots) == 0 {
		return Repository{}, fmt.Errorf("resolve repository identity: %w", err)
	}
	sort.Strings(roots)
	digest := sha256.Sum256([]byte(strings.Join(roots, "\n")))
	return Repository{
		Name:         filepath.Base(gitRoot),
		Identity:     "git:" + hex.EncodeToString(digest[:]),
		VCS:          "git",
		Root:         ".",
		Branch:       branch,
		HeadRevision: head,
		BaseRevision: head,
		absoluteRoot: gitRoot,
	}, nil
}

func gitOutput(ctx stdcontext.Context, root string, args ...string) (string, error) {
	cmdArgs := append([]string{"-C", root}, args...)
	out, err := exec.CommandContext(ctx, "git", cmdArgs...).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitLines(ctx stdcontext.Context, root string, args ...string) ([]string, error) {
	out, err := gitOutput(ctx, root, args...)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}
