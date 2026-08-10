package context

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrInvalidSeed marks a seed that does not name a file inside the repository.
var ErrInvalidSeed = errors.New("invalid seed")

// maxSeeds bounds what one request may name explicitly. Seeds outrank every
// other reason, so an unbounded list would let a caller fill the packet with
// them and leave the request itself no room.
const maxSeeds = 40

// cleanSeeds resolves caller-supplied starting paths to repository-relative
// files, in canonical order and without duplicates.
//
// A seed says "this file is relevant, I already know". It is the one selection
// reason the CLI does not derive, which is exactly why it is checked hardest:
// naming a file gets it considered, never included. Every exclusion still
// applies afterwards, so a seed pointing at a detected secret is refused and
// recorded like any other candidate. "Reviewed" describes the caller's
// judgement about relevance, not a claim that the file is safe to disclose.
func cleanSeeds(root, scope string, requested []string) ([]string, error) {
	if len(requested) == 0 {
		return nil, nil
	}
	unique := map[string]struct{}{}
	for _, raw := range requested {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		rel, err := cleanRequestedPath(root, raw)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInvalidSeed, err)
		}
		info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return nil, fmt.Errorf("%w: %s is not a file in this repository", ErrInvalidSeed, rel)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("%w: %s is a directory; use --scope to narrow to a subtree", ErrInvalidSeed, rel)
		}
		// A seed outside the scope is refused rather than quietly overriding it.
		// Two mechanisms that silently disagree about the candidate set is the
		// surprise this avoids, and the error says which one to change. It also
		// keeps "scope narrows and never widens" literally true, which is a
		// promise worth more than the convenience of relaxing it here.
		if !withinScope(rel, scope) {
			return nil, fmt.Errorf("%w: %s is outside the requested scope %s", ErrInvalidSeed, rel, scope)
		}
		unique[rel] = struct{}{}
	}
	if len(unique) > maxSeeds {
		return nil, fmt.Errorf("%w: %d seeds exceeds the limit of %d", ErrInvalidSeed, len(unique), maxSeeds)
	}
	seeds := make([]string, 0, len(unique))
	for rel := range unique {
		seeds = append(seeds, rel)
	}
	sort.Strings(seeds)
	return seeds, nil
}
