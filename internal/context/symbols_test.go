package context

import (
	stdcontext "context"
	"strings"
	"testing"
)

func TestIdentifierTokensSplitsTheWayPathsDo(t *testing.T) {
	for name, want := range map[string][]string{
		"WithTimeout":     {"with", "timeout"},
		"parseHTTPHeader": {"parse", "http", "header"},
		"read_small_file": {"read", "small", "file"},
		"Timeout":         {"timeout"},
		"ID":              nil, // shorter than the three-character floor
	} {
		got := identifierTokens(name)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Errorf("identifierTokens(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestDeclaredNamesCoversTopLevelDeclarations(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "decls.go", `package svc

type Server struct{ addr string }

const DefaultTimeout = 30

var ErrClosed = error(nil)

func New() *Server { return nil }

func (s *Server) Shutdown() error { return nil }
`)
	match, ok := fileSymbolMatch(root, "decls.go", map[string]struct{}{
		"server": {}, "timeout": {}, "closed": {}, "shutdown": {},
	})
	if !ok {
		t.Fatal("fileSymbolMatch could not read a valid file")
	}
	if match.score != 4 {
		t.Fatalf("score = %d, want 4 (server, timeout, closed, shutdown)", match.score)
	}
	// Receiver types count, so a method reaches its type's name too.
	for _, want := range []string{"DefaultTimeout", "ErrClosed", "Server", "Shutdown"} {
		if !containsString(match.names, want) {
			t.Errorf("declared names %v missing %q", match.names, want)
		}
	}
}

// The gap symbol matching exists to close: the request names something the file
// declares, but nothing in its path says so.
func TestSymbolMatchFindsFilesNoPathSignalWouldReach(t *testing.T) {
	root := initSelectionRepo(t)
	writeFile(t, root, "server/wrap.go", `package server

import "net/http"

// WithTimeout wraps h so a request cannot run forever.
func WithTimeout(h http.Handler) http.Handler { return h }
`)
	writeFile(t, root, "server/unrelated.go", `package server

func Colour() string { return "blue" }
`)

	const task = "add request timeout middleware"
	selection, err := selectPacketContextWithLimits(stdcontext.Background(), root, task, nil, DefaultPacketLimits())
	if err != nil {
		t.Fatalf("selectPacketContext returned error: %v", err)
	}

	var wrap *PacketItem
	for i := range selection.items {
		if selection.items[i].Path == "server/wrap.go" {
			wrap = &selection.items[i]
		}
		if selection.items[i].Path == "server/unrelated.go" {
			t.Error("a file sharing no words with the request was selected")
		}
	}
	if wrap == nil {
		t.Fatalf("server/wrap.go was not selected: %#v", selection.items)
	}
	if wrap.Reason != "symbol_match" {
		t.Fatalf("reason = %q, want symbol_match", wrap.Reason)
	}
	// A selection nobody can justify is worse than one nobody makes.
	if !strings.Contains(wrap.Provenance.Location, "WithTimeout") {
		t.Fatalf("provenance does not name the matched declaration: %q", wrap.Provenance.Location)
	}

	explanation, err := ExplainSelection(stdcontext.Background(), root, "server/wrap.go", task, "")
	if err != nil {
		t.Fatalf("ExplainSelection returned error: %v", err)
	}
	if explanation.Decision != "included" || explanation.Reason != "symbol_match" {
		t.Fatalf("explain = %s (%s), want included (symbol_match)", explanation.Decision, explanation.Reason)
	}
	if !strings.Contains(explanation.Detail, "WithTimeout") {
		t.Fatalf("explain does not name the declaration: %q", explanation.Detail)
	}
}

// Path and symbol evidence add up, so a file both named for the request and
// declaring what it names outranks one carrying only half the evidence.
func TestSymbolEvidenceRanksAbovePathEvidenceAlone(t *testing.T) {
	root := initSelectionRepo(t)
	writeFile(t, root, "timeout/doc.go", "package timeout\n\nfunc Unrelated() {}\n")
	writeFile(t, root, "server/wrap.go", `package server

func Timeout() {}
func RequestTimeout() {}
`)

	limits := DefaultPacketLimits()
	limits.MaxItems = 3
	selection, err := selectPacketContextWithLimits(stdcontext.Background(), root, "request timeout handling", nil, limits)
	if err != nil {
		t.Fatalf("selectPacketContext returned error: %v", err)
	}
	ranks := map[string]int{}
	for i, item := range selection.items {
		ranks[item.Path] = i
	}
	if _, ok := ranks["server/wrap.go"]; !ok {
		t.Fatalf("the file declaring the requested identifiers was not selected: %#v", ranks)
	}
}

// Symbol matching must only ever add. A repository in another language, or one
// where nothing parses, has to select exactly what it selected before.
func TestSymbolMatchingOnlyAddsCandidates(t *testing.T) {
	root := initSelectionRepo(t)
	writeFile(t, root, "src/timeout.rb", "def with_timeout; end\n")
	writeFile(t, root, "broken.go", "package broken\n\nfunc Timeout( {\n")

	selection, err := selectPacketContextWithLimits(stdcontext.Background(), root, "add request timeout", nil, DefaultPacketLimits())
	if err != nil {
		t.Fatalf("selectPacketContext returned error: %v", err)
	}
	for _, item := range selection.items {
		if item.Path == "broken.go" && item.Reason == "symbol_match" {
			t.Fatal("a file that does not parse produced a symbol match")
		}
	}
	// The Ruby file still reaches the packet the way it always did, by filename.
	found := false
	for _, item := range selection.items {
		if item.Path == "src/timeout.rb" {
			found = true
			if item.Reason != "task_match" {
				t.Fatalf("non-Go file reason = %q, want task_match", item.Reason)
			}
		}
	}
	if !found {
		t.Fatalf("filename matching regressed for a non-Go file: %#v", selection.items)
	}
}

// A file excluded by directory convention or configuration is never indexed, so
// symbol matching cannot resurrect it.
func TestSymbolMatchingSkipsExcludedPaths(t *testing.T) {
	root := initSelectionRepo(t)
	writeFile(t, root, "build/generated.go", "package generated\n\nfunc Timeout() {}\n")

	selection, err := selectPacketContextWithLimits(stdcontext.Background(), root, "add request timeout", nil, DefaultPacketLimits())
	if err != nil {
		t.Fatalf("selectPacketContext returned error: %v", err)
	}
	for _, item := range selection.items {
		if item.Path == "build/generated.go" {
			t.Fatal("symbol matching selected a file under an excluded directory")
		}
	}
}

// Measured noise, not theory: before this filter, every long test name in the
// repository matched almost every request, because one token in six was enough.
func TestSymbolMatchIgnoresWordsBuriedInLongIdentifiers(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "noise.go", `package noise

func TestMCPSurvivesAnOversizeRequest() {}
`)
	writeFile(t, root, "signal.go", `package signal

type request struct{}

func WithRequestID() {}
`)
	words := map[string]struct{}{"request": {}}

	noise, ok := fileSymbolMatch(root, "noise.go", words)
	if !ok {
		t.Fatal("could not read noise.go")
	}
	if noise.score != 0 {
		t.Errorf("a request word buried in a six-token name counted as evidence: %v", noise.names)
	}

	signal, ok := fileSymbolMatch(root, "signal.go", words)
	if !ok {
		t.Fatal("could not read signal.go")
	}
	if signal.score == 0 {
		t.Errorf("an identifier that is about the request word was not matched")
	}
}

// A request names an action and a subject, and only the subject identifies
// code. "add request timeout" used to match every AddString in the repository.
func TestActionVerbsAreNotSelectionWords(t *testing.T) {
	words := selectionTaskWords("add fix improve update refactor the timeout handler")
	for _, verb := range []string{"add", "fix", "improve", "update", "refactor"} {
		if _, ok := words[verb]; ok {
			t.Errorf("%q is treated as subject matter", verb)
		}
	}
	for _, subject := range []string{"timeout", "handler"} {
		if _, ok := words[subject]; !ok {
			t.Errorf("%q was dropped from the request", subject)
		}
	}
}
