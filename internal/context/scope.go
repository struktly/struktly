package context

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ErrInvalidScope marks a scope that does not name a directory inside the
// repository. It is a bad argument rather than a repository problem, so it
// carries the invocation exit code.
var ErrInvalidScope = errors.New("invalid scope")

// cleanScope resolves a requested scope to a repository-relative directory.
//
// Scope narrows which files a request considers. It deliberately does not
// change what the packet is *about*: repository identity, branch and revisions
// stay the repository's, because a service inside a monorepo is not a separate
// repository and a packet that claimed otherwise would make two different
// scopes look like two different projects.
func cleanScope(root, requested string) (string, error) {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return "", nil
	}
	rel, err := cleanRequestedPath(root, requested)
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrInvalidScope, err)
	}
	if rel == "." {
		// Scoping to the repository root is the unscoped case, not an error.
		return "", nil
	}
	info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		return "", fmt.Errorf("%w: %s is not a directory in this repository", ErrInvalidScope, rel)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%w: %s is a symlink", ErrInvalidScope, rel)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: %s is not a directory", ErrInvalidScope, rel)
	}
	return rel, nil
}

// withinScope reports whether a repository-relative path is eligible under a
// scope. An empty scope admits everything.
//
// Files under the scope are eligible. So is repository governance in an
// ancestor directory — agent instruction files and `.struktly/` declarations —
// because those govern the whole repository including the scoped subtree, and a
// scoped packet that dropped the root AGENTS.md would be worse context than an
// unscoped one rather than narrower. They keep their own selection reasons, so
// the packet already says why a file above the scope is present.
//
// Scope only ever narrows: it cannot admit a file that the exclusion rules
// reject, so no security rule is weakened by naming one.
func withinScope(rel, scope string) bool {
	if scope == "" {
		return true
	}
	if rel == scope || strings.HasPrefix(rel, scope+"/") {
		return true
	}
	// Repository declarations live in `.struktly/` at the repository root and
	// govern every scope. The directory is not an ancestor of the scope in the
	// path sense, so it cannot go through the test below.
	if strings.HasPrefix(rel, ".struktly/") {
		return true
	}
	// An instruction file governs the tree it sits in, so services/CLAUDE.md
	// reaches services/api while other/AGENTS.md does not.
	return isAgentInstructionPath(rel) && isAncestorPath(rel, scope)
}

// isAncestorPath reports whether rel sits in a directory at or above scope.
func isAncestorPath(rel, scope string) bool {
	dir := path.Dir(rel)
	if dir == "." {
		return true
	}
	return scope == dir || strings.HasPrefix(scope, dir+"/")
}
