package context

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
)

// maxDeclarationParseBytes bounds the files this will parse. Rendering
// declarations means reading and scanning the whole file rather than the
// per-file prefix, so the work is capped; past this size a file falls back to
// byte truncation.
const maxDeclarationParseBytes = 1 << 20

// declarationRendering is the value PacketItem.Rendering carries when content
// is a declaration skeleton rather than verbatim source. A consumer that does
// not check it would otherwise read a function with no body as a function that
// does nothing.
const declarationRendering = "declarations"

func isGoSource(rel string) bool {
	return strings.HasSuffix(rel, ".go")
}

// goDeclarations renders a Go file's declarations without function bodies:
// package clause, imports, types, values, and every function signature with its
// doc comment.
//
// This exists because a byte-truncated file is a poor use of a byte budget. Cut
// at 64 KiB, a large file yields its imports and the first few functions, and
// the reader cannot even tell what else is in it. The same budget spent on
// signatures and doc comments describes the whole file — measured against this
// repository's own sources, the skeleton is 69% to 82% smaller than the file it
// summarizes, so files that previously arrived as a truncated fragment now
// arrive complete.
//
// Returns false when the file does not parse, which is the correct outcome for
// a file mid-edit: byte truncation is a worse summary but an honest one.
func goDeclarations(src []byte) (string, bool) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return "", false
	}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			fn.Body = nil
		}
	}
	// Printing a File with its Comments list set interleaves comments by source
	// position, which stops matching the tree once bodies are gone. Clearing it
	// makes the printer use the Doc and Comment fields still attached to the
	// surviving nodes, so declaration docs survive and comments from inside
	// discarded bodies do not reappear at arbitrary places.
	file.Comments = nil

	var b bytes.Buffer
	cfg := printer.Config{Mode: printer.UseSpaces | printer.TabIndent, Tabwidth: 8}
	if err := cfg.Fprint(&b, fset, file); err != nil {
		return "", false
	}
	rendered := strings.TrimRight(b.String(), "\n") + "\n"
	if len(rendered) >= len(src) {
		return "", false
	}
	return rendered, true
}
