package context

import (
	stdcontext "context"
	"strings"
	"testing"
)

// importRepo is a repository where the dependency is reachable only through a
// call: nothing in internal/clock's path, identifiers or package name matches
// the request.
func importRepo(t *testing.T) string {
	t.Helper()
	root := initSelectionRepo(t)
	writeFile(t, root, "go.mod", "module example.com/svc\n\ngo 1.24.0\n")
	writeFile(t, root, "middleware/timeout.go", `package middleware

import "example.com/svc/internal/clock"

func Timeout() int { return clock.Grace }
`)
	writeFile(t, root, "internal/clock/clock.go", `package clock

const Grace = 30

func Wall() int { return 0 }
`)
	writeFile(t, root, "internal/clock/unused.go", `package clock

func Nap() {}
`)
	return root
}

func TestImportExpansionFollowsUseNotReachability(t *testing.T) {
	root := importRepo(t)
	selection, err := selectPacketContext(stdcontext.Background(), selectionRequest{
		root: root, task: "add request timeout middleware", limits: DefaultPacketLimits(),
	})
	if err != nil {
		t.Fatalf("selectPacketContext returned error: %v", err)
	}

	var clock *PacketItem
	for i := range selection.items {
		switch selection.items[i].Path {
		case "internal/clock/clock.go":
			clock = &selection.items[i]
		case "internal/clock/unused.go":
			// The whole point: a package is a directory, and importing it must
			// not drag in siblings nothing selected actually calls.
			t.Error("an unused file from an imported package was selected")
		}
	}
	if clock == nil {
		t.Fatalf("the imported dependency was not selected: %v", selectedPaths(selection.items))
	}
	if clock.Reason != "import_neighbor" {
		t.Fatalf("reason = %q, want import_neighbor", clock.Reason)
	}
	if !strings.Contains(clock.Provenance.Location, "Grace") {
		t.Fatalf("provenance does not name what the file supplies: %q", clock.Provenance.Location)
	}
}

// Import expansion is a reason to look, not evidence about the request, so it
// must never take a place a direct match would have had.
func TestImportExpansionNeverDisplacesADirectMatch(t *testing.T) {
	root := importRepo(t)
	limits := DefaultPacketLimits()
	limits.MaxItems = 2
	selection, err := selectPacketContext(stdcontext.Background(), selectionRequest{
		root: root, task: "add request timeout middleware", limits: limits,
	})
	if err != nil {
		t.Fatalf("selectPacketContext returned error: %v", err)
	}
	for _, item := range selection.items {
		if item.Reason == "import_neighbor" {
			t.Fatalf("an import neighbour took a slot under a tight limit: %v", selectedPaths(selection.items))
		}
	}
}

// A file the request reaches only through expansion still has to be
// explainable, or `explain` reports not_selected for a file the packet carries.
func TestExplainAnswersForImportNeighbors(t *testing.T) {
	root := importRepo(t)
	explanation, err := ExplainSelection(stdcontext.Background(), root, "internal/clock/clock.go", "add request timeout middleware", "")
	if err != nil {
		t.Fatalf("ExplainSelection returned error: %v", err)
	}
	if explanation.Decision != "included" || explanation.Reason != "import_neighbor" {
		t.Fatalf("explain = %s (%s), want included (import_neighbor)", explanation.Decision, explanation.Reason)
	}
	if !strings.Contains(explanation.Detail, "Grace") {
		t.Fatalf("explain does not say what the file supplies: %q", explanation.Detail)
	}

	unused, err := ExplainSelection(stdcontext.Background(), root, "internal/clock/unused.go", "add request timeout middleware", "")
	if err != nil {
		t.Fatalf("ExplainSelection returned error: %v", err)
	}
	if unused.Decision != "excluded" || unused.Reason != "not_selected" {
		t.Fatalf("explain = %s (%s), want excluded (not_selected)", unused.Decision, unused.Reason)
	}
}

// Expansion must respect every boundary the first pass respects.
func TestImportExpansionRespectsScopeAndExclusions(t *testing.T) {
	root := importRepo(t)
	selection, err := selectPacketContext(stdcontext.Background(), selectionRequest{
		root: root, task: "add request timeout middleware", scope: "middleware",
		limits: DefaultPacketLimits(),
	})
	if err != nil {
		t.Fatalf("selectPacketContext returned error: %v", err)
	}
	for _, item := range selection.items {
		if strings.HasPrefix(item.Path, "internal/") {
			t.Fatalf("expansion reached outside the requested scope: %s", item.Path)
		}
	}
}

func TestModulePathAndLocalNames(t *testing.T) {
	root := importRepo(t)
	if got := modulePath(root); got != "example.com/svc" {
		t.Fatalf("modulePath = %q", got)
	}
	// An aliased import is known by its alias; an unaliased one by the imported
	// package's name, which is not always its directory.
	writeFile(t, root, "aliased.go", `package svc

import (
	renamed "example.com/svc/internal/clock"
	_ "example.com/svc/internal/clock"
	"net/http"
)

var _ = renamed.Grace
var _ = http.StatusOK
`)
	file, ok := parseGoFile(root, "aliased.go", 0)
	if !ok {
		t.Fatal("could not parse the fixture")
	}
	resolver := packageResolver{root: root, byDirectory: map[string][]string{
		"internal/clock": {"internal/clock/clock.go"},
	}, names: map[string]string{}}
	local := resolver.localNames(file, "example.com/svc")
	if local["renamed"] != "internal/clock" {
		t.Fatalf("alias not resolved: %#v", local)
	}
	if _, ok := local["_"]; ok {
		t.Fatalf("a blank import produced a qualifier: %#v", local)
	}
	if _, ok := local["http"]; ok {
		t.Fatalf("an out-of-repository import was resolved: %#v", local)
	}
}
