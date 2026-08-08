package context

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// maxIndexedSymbolFiles bounds how many files one request will parse. Reaching
// it means some file was never considered, which is a silent omission unless it
// is reported, so selectPacketContext records a warning when it happens.
const maxIndexedSymbolFiles = 5000

// maxRecordedSymbols bounds the declared names carried in an item's provenance.
// The record is there to justify the selection, not to reproduce the file.
const maxRecordedSymbols = 6

// contentIndex maps repository-relative paths to what a file says it is about,
// where that differs from what its path says.
//
// Filename matching cannot see inside a file. A request naming a timeout finds
// middleware/timeout.go by its name and misses `func WithTimeout` in
// server/wrap.go; a request naming architecture decisions misses
// docs/adr/0001-record.md, whose filename is a serial number and whose title is
// "ADR 0001: Record architecture decisions". Both are the same failure — the
// path is not the file — and both are read exactly rather than guessed, which
// is what lets `explain` justify a selection in one line.
//
// Matching only ever adds candidates. A repository in another language, or one
// where nothing parses, selects exactly what it selected before.
type contentIndex struct {
	matches  map[string]contentMatch
	indexed  int
	skipped  int
	attempts int
}

type contentMatch struct {
	// score is the number of distinct request words the file matches, so a file
	// answering three words outranks one answering one.
	score int
	// words are those request words. Relevance unions them with the words the
	// path already matched rather than adding: a document titled after its own
	// filename answers one question, not two, and summing let it outrank the
	// implementation it merely describes.
	words map[string]struct{}
	// reason is the selection reason this evidence supports.
	reason string
	// names are the declarations or the title responsible, for the audit trail.
	names []string
}

// buildSymbolIndex parses the eligible Go sources among paths and records which
// declare identifiers the request names. Paths already excluded by directory
// convention or repository configuration are not read: they cannot be selected,
// so indexing them would cost I/O to reach a foregone conclusion.
func buildContentIndex(root string, paths []string, words map[string]struct{}, excluded func(string) bool) contentIndex {
	index := contentIndex{matches: map[string]contentMatch{}}
	if len(words) == 0 {
		return index
	}
	for _, rel := range paths {
		if !isIndexableContent(rel) || excluded(rel) {
			continue
		}
		if index.attempts >= maxIndexedSymbolFiles {
			index.skipped++
			continue
		}
		index.attempts++
		match, ok := fileContentMatch(root, rel, words)
		if !ok {
			continue
		}
		index.indexed++
		if match.score > 0 {
			index.matches[rel] = match
		}
	}
	return index
}

func (i contentIndex) match(rel string) contentMatch {
	return i.matches[rel]
}

func isIndexableContent(rel string) bool {
	return isGoSource(rel) || isMarkdown(rel)
}

func isMarkdown(rel string) bool {
	return strings.HasSuffix(strings.ToLower(rel), ".md")
}

// fileContentMatch reads whichever kind of evidence the file carries.
func fileContentMatch(root, rel string, words map[string]struct{}) (contentMatch, bool) {
	if isGoSource(rel) {
		return fileSymbolMatch(root, rel, words)
	}
	if isMarkdown(rel) {
		return fileTitleMatch(root, rel, words)
	}
	return contentMatch{}, false
}

// fileSymbolMatch reports which of words the declarations in rel match. The
// second result is false when the file could not be read or parsed, which is
// not an error: a file mid-edit simply contributes no symbols.
func fileSymbolMatch(root, rel string, words map[string]struct{}) (contentMatch, bool) {
	full := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Lstat(full)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxDeclarationParseBytes {
		return contentMatch{}, false
	}
	src, err := os.ReadFile(full)
	if err != nil {
		return contentMatch{}, false
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.SkipObjectResolution)
	if err != nil {
		return contentMatch{}, false
	}

	matchedWords := map[string]struct{}{}
	matchedNames := map[string]struct{}{}
	for _, name := range declaredNames(file) {
		hit, tokens := map[string]struct{}{}, identifierTokens(name)
		for _, tokenText := range tokens {
			if _, ok := words[tokenText]; ok {
				hit[tokenText] = struct{}{}
			}
		}
		if !isSpecificMatch(len(hit), len(tokens)) {
			continue
		}
		for tokenText := range hit {
			matchedWords[tokenText] = struct{}{}
		}
		matchedNames[name] = struct{}{}
	}
	names := make([]string, 0, len(matchedNames))
	for name := range matchedNames {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) > maxRecordedSymbols {
		names = names[:maxRecordedSymbols]
	}
	return contentMatch{score: len(matchedWords), words: matchedWords, reason: "symbol_match", names: names}, true
}

// isSpecificMatch reports whether a declaration is about the words it matched,
// rather than merely containing one of them.
//
// A request word has to account for at least half of an identifier's tokens.
// `Validate` and `WithTimeout` are about validating and timeouts;
// `TestMCPSurvivesAnOversizeRequest` is not about requests, it just contains
// the word, and without this every long test name in a repository matched
// almost every request. Measured on this repository, the filter removes the
// test-name noise while keeping `truncationDetail` for a truncation request and
// `selectionTaskWords` for a task request.
func isSpecificMatch(matched, total int) bool {
	return matched > 0 && matched*2 >= total
}

// declaredNames returns the top-level identifiers a file declares: functions
// and methods, the receiver types of methods, and named types, constants and
// variables. Struct fields and locals are deliberately excluded — they are
// numerous, and a file is not about its local variables.
func declaredNames(file *ast.File) []string {
	names := []string{}
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			names = append(names, d.Name.Name)
			if d.Recv != nil {
				for _, field := range d.Recv.List {
					if receiver := receiverTypeName(field.Type); receiver != "" {
						names = append(names, receiver)
					}
				}
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					names = append(names, s.Name.Name)
				case *ast.ValueSpec:
					for _, ident := range s.Names {
						names = append(names, ident.Name)
					}
				}
			}
		}
	}
	return names
}

func receiverTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return receiverTypeName(t.X)
	case *ast.IndexExpr:
		return receiverTypeName(t.X)
	case *ast.IndexListExpr:
		return receiverTypeName(t.X)
	default:
		return ""
	}
}

// identifierTokens splits an identifier the same way paths are split, so
// `WithTimeout` matches a request naming "timeout" and `parseHTTPHeader`
// matches "header".
func identifierTokens(name string) []string {
	out := []string{}
	for _, part := range strings.FieldsFunc(name, func(r rune) bool { return r == '_' }) {
		for _, segment := range splitToken(part) {
			segment = strings.ToLower(segment)
			if len(segment) >= 3 {
				out = append(out, segment)
			}
		}
	}
	return out
}
