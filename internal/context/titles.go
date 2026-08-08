package context

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/struktly/struktly/internal/files"
)

// maxTitleScanBytes bounds the read for a title. A document's first heading is
// near the top, so there is no reason to read a large file to find it.
const maxTitleScanBytes = 8 << 10

// minTitleWords is how many distinct request words a title must carry.
//
// Deliberately not the specificity ratio identifiers use. An identifier is
// short and cohesive, so "at least half its tokens" is a fair test of what it
// is about; a prose title is longer, and "ADR 0001: Record architecture
// decisions" is plainly about architecture decisions while matching only two of
// its five tokens. Two distinct request words in a title is the evidence; one
// is a word the document happens to contain.
const minTitleWords = 2

// fileTitleMatch reports whether a document's title names what the request
// names.
//
// A filename is a guess at a document's subject and often a bad one: an ADR is
// called 0001-record.md and a design note ages out of its own name. The title
// is the document's own claim about itself, which is both better evidence and
// something `explain` can quote back.
func fileTitleMatch(root, rel string, words map[string]struct{}) (contentMatch, bool) {
	title, ok := documentTitle(filepath.Join(root, filepath.FromSlash(rel)))
	if !ok {
		return contentMatch{}, false
	}
	matched := map[string]struct{}{}
	for token := range pathTokens(title) {
		if _, ok := words[token]; ok {
			matched[token] = struct{}{}
		}
	}
	if len(matched) < minTitleWords {
		return contentMatch{}, true
	}
	return contentMatch{score: len(matched), words: matched, reason: "title_match", names: []string{title}}, true
}

// documentTitle returns a Markdown document's first level-one heading, with any
// OKF frontmatter stripped first so a generated file reports its heading rather
// than its metadata.
func documentTitle(path string) (string, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return "", false
	}
	handle, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer handle.Close()

	prefix := make([]byte, maxTitleScanBytes)
	n, err := handle.Read(prefix)
	if n == 0 && err != nil {
		return "", false
	}
	for _, line := range strings.Split(files.StripFrontmatter(string(prefix[:n])), "\n") {
		heading, ok := strings.CutPrefix(strings.TrimSpace(line), "# ")
		if !ok {
			continue
		}
		if heading = strings.TrimSpace(heading); heading != "" {
			return heading, true
		}
	}
	return "", false
}
