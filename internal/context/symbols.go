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

// symbolIndex maps repository-relative paths to the identifiers they declare
// that the request also names.
//
// Filename matching cannot see inside a file, so a request like "add request
// timeout middleware" finds middleware/timeout.go by its name and misses
// `func WithTimeout` in server/wrap.go entirely. The declarations are ground
// truth for what a file offers, and go/ast reads them exactly rather than
// guessing — which is the property that lets `explain` justify the selection in
// one line instead of asserting relevance.
//
// Matching only ever adds candidates. A repository in another language, or one
// where nothing parses, selects exactly what it selected before.
type symbolIndex struct {
	matches  map[string]symbolMatch
	indexed  int
	skipped  int
	attempts int
}

type symbolMatch struct {
	// score is the number of distinct request words the file's declarations
	// match, so a file answering three words outranks one answering one.
	score int
	// names are the declarations responsible, for the audit trail.
	names []string
}

// buildSymbolIndex parses the eligible Go sources among paths and records which
// declare identifiers the request names. Paths already excluded by directory
// convention or repository configuration are not read: they cannot be selected,
// so indexing them would cost I/O to reach a foregone conclusion.
func buildSymbolIndex(root string, paths []string, words map[string]struct{}, excluded func(string) bool) symbolIndex {
	index := symbolIndex{matches: map[string]symbolMatch{}}
	if len(words) == 0 {
		return index
	}
	for _, rel := range paths {
		if !isGoSource(rel) || excluded(rel) {
			continue
		}
		if index.attempts >= maxIndexedSymbolFiles {
			index.skipped++
			continue
		}
		index.attempts++
		match, ok := fileSymbolMatch(root, rel, words)
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

func (i symbolIndex) match(rel string) symbolMatch {
	return i.matches[rel]
}

// fileSymbolMatch reports which of words the declarations in rel match. The
// second result is false when the file could not be read or parsed, which is
// not an error: a file mid-edit simply contributes no symbols.
func fileSymbolMatch(root, rel string, words map[string]struct{}) (symbolMatch, bool) {
	full := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Lstat(full)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxDeclarationParseBytes {
		return symbolMatch{}, false
	}
	src, err := os.ReadFile(full)
	if err != nil {
		return symbolMatch{}, false
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.SkipObjectResolution)
	if err != nil {
		return symbolMatch{}, false
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
	return symbolMatch{score: len(matchedWords), names: names}, true
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
