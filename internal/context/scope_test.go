package context

import (
	stdcontext "context"
	"strings"
	"testing"
)

func scopedRepo(t *testing.T) string {
	t.Helper()
	root := initSelectionRepo(t)
	writeFile(t, root, "AGENTS.md", "# Repository rules\n")
	writeFile(t, root, "services/api/handler.go", "package api\n\nfunc Timeout() {}\n")
	writeFile(t, root, "services/api/AGENTS.md", "# API rules\n")
	writeFile(t, root, "services/web/handler.go", "package web\n\nfunc Timeout() {}\n")
	return root
}

func selectedPaths(items []PacketItem) []string {
	paths := make([]string, 0, len(items))
	for _, item := range items {
		paths = append(paths, item.Path)
	}
	return paths
}

func TestScopeNarrowsSelectionToTheNamedSubtree(t *testing.T) {
	root := scopedRepo(t)
	selection, err := selectPacketContext(stdcontext.Background(), selectionRequest{root: root, task: "request timeout", scope: "services/api", limits: DefaultPacketLimits()})
	if err != nil {
		t.Fatalf("selectPacketContext returned error: %v", err)
	}
	paths := selectedPaths(selection.items)
	for _, want := range []string{"services/api/handler.go", "services/api/AGENTS.md"} {
		if !containsString(paths, want) {
			t.Errorf("scope dropped %q from its own subtree: %v", want, paths)
		}
	}
	for _, unwanted := range []string{"services/web/handler.go"} {
		if containsString(paths, unwanted) {
			t.Errorf("scope admitted %q from a sibling subtree: %v", unwanted, paths)
		}
	}
}

// A scoped packet that dropped the repository's own AGENTS.md would be worse
// context than an unscoped one rather than narrower, so governance in an
// ancestor directory survives the narrowing.
func TestScopeKeepsRepositoryGovernanceFromAncestors(t *testing.T) {
	root := scopedRepo(t)
	writeFile(t, root, ".struktly/constraints.md", "# Constraints\n\n- Keep it small.\n")

	selection, err := selectPacketContext(stdcontext.Background(), selectionRequest{root: root, task: "request timeout", scope: "services/api", limits: DefaultPacketLimits()})
	if err != nil {
		t.Fatalf("selectPacketContext returned error: %v", err)
	}
	paths := selectedPaths(selection.items)
	for _, want := range []string{"AGENTS.md", ".struktly/constraints.md"} {
		if !containsString(paths, want) {
			t.Errorf("scope dropped repository governance %q: %v", want, paths)
		}
	}
	// README.md at the root is not governance, so it goes.
	if containsString(paths, "README.md") {
		t.Errorf("scope admitted an ordinary root file: %v", paths)
	}
}

// Scope narrows what a request considers. It must not change what the packet is
// about: a service inside a monorepo is not a separate repository, and a packet
// claiming otherwise would make two scopes look like two projects.
func TestScopeDoesNotWeakenRepositoryIdentity(t *testing.T) {
	root := scopedRepo(t)
	whole, err := Brief(BriefOptions{Root: root, Task: "request timeout", NoWrite: true})
	if err != nil {
		t.Fatalf("Brief returned error: %v", err)
	}
	scoped, err := Brief(BriefOptions{Root: root, Task: "request timeout", Scope: "services/api", NoWrite: true})
	if err != nil {
		t.Fatalf("Brief returned error: %v", err)
	}
	if scoped.Packet.Repository.Identity != whole.Packet.Repository.Identity {
		t.Fatalf("scope changed repository identity: %q vs %q",
			scoped.Packet.Repository.Identity, whole.Packet.Repository.Identity)
	}
	if scoped.Packet.Repository.HeadRevision != whole.Packet.Repository.HeadRevision {
		t.Fatal("scope changed the recorded HEAD revision")
	}
	if scoped.Packet.Repository.Root != "." {
		t.Fatalf("scope changed the portable repository root to %q", scoped.Packet.Repository.Root)
	}
	if scoped.Packet.Scope != "services/api" {
		t.Fatalf("packet does not record its scope: %q", scoped.Packet.Scope)
	}
	// The same request at two scopes is two different contexts.
	if scoped.Packet.PacketHash == whole.Packet.PacketHash {
		t.Fatal("scoped and unscoped packets share an identity")
	}
}

// Scope only ever narrows, so it cannot admit a file the exclusion rules reject.
func TestScopeCannotAdmitExcludedFiles(t *testing.T) {
	root := scopedRepo(t)
	writeFile(t, root, "services/api/secrets/token.txt", "value\n")
	writeFile(t, root, "services/api/.env", "TOKEN=value\n")

	selection, err := selectPacketContext(stdcontext.Background(), selectionRequest{root: root, task: "secrets token env", scope: "services/api", limits: DefaultPacketLimits()})
	if err != nil {
		t.Fatalf("selectPacketContext returned error: %v", err)
	}
	for _, path := range selectedPaths(selection.items) {
		if strings.Contains(path, "secrets/") || strings.HasSuffix(path, ".env") {
			t.Fatalf("scope admitted a file the security rules exclude: %s", path)
		}
	}
}

func TestScopeRejectsPathsOutsideTheRepository(t *testing.T) {
	root := scopedRepo(t)
	for name, scope := range map[string]string{
		"parent":     "../elsewhere",
		"absolute":   "/etc",
		"not a dir":  "README.md",
		"nonexisten": "services/absent",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Brief(BriefOptions{Root: root, Task: "anything", Scope: scope, NoWrite: true})
			if err == nil {
				t.Fatalf("scope %q was accepted", scope)
			}
			if !strings.Contains(err.Error(), ErrInvalidScope.Error()) {
				t.Fatalf("error does not identify the problem: %v", err)
			}
		})
	}
}

// Scoping to the repository root is the unscoped case, not an error.
func TestScopeAtRepositoryRootIsUnscoped(t *testing.T) {
	root := scopedRepo(t)
	for _, scope := range []string{"", ".", "  "} {
		result, err := Brief(BriefOptions{Root: root, Task: "request timeout", Scope: scope, NoWrite: true})
		if err != nil {
			t.Fatalf("scope %q returned error: %v", scope, err)
		}
		if result.Packet.Scope != "" {
			t.Fatalf("scope %q recorded as %q", scope, result.Packet.Scope)
		}
	}
}

func TestExplainReportsOutOfScope(t *testing.T) {
	root := scopedRepo(t)
	explanation, err := ExplainSelection(stdcontext.Background(), root, "services/web/handler.go", "request timeout", "services/api")
	if err != nil {
		t.Fatalf("ExplainSelection returned error: %v", err)
	}
	if explanation.Decision != "excluded" || explanation.Reason != "out_of_scope" {
		t.Fatalf("explain = %s (%s), want excluded (out_of_scope)", explanation.Decision, explanation.Reason)
	}
	if !strings.Contains(explanation.Detail, "services/api") {
		t.Fatalf("explain does not name the scope: %q", explanation.Detail)
	}
}

func TestWithinScopeRules(t *testing.T) {
	for name, test := range map[string]struct {
		rel, scope string
		want       bool
	}{
		"inside":                   {rel: "services/api/a.go", scope: "services/api", want: true},
		"the scope itself":         {rel: "services/api", scope: "services/api", want: true},
		"sibling":                  {rel: "services/web/a.go", scope: "services/api", want: false},
		"prefix collision":         {rel: "services/apiv2/a.go", scope: "services/api", want: false},
		"root governance":          {rel: "AGENTS.md", scope: "services/api", want: true},
		"intermediate governance":  {rel: "services/CLAUDE.md", scope: "services/api", want: true},
		"unrelated governance":     {rel: "other/AGENTS.md", scope: "services/api", want: false},
		"root declaration":         {rel: ".struktly/direction.md", scope: "services/api", want: true},
		"ordinary root file":       {rel: "README.md", scope: "services/api", want: false},
		"everything when unscoped": {rel: "anything/at/all.go", scope: "", want: true},
	} {
		t.Run(name, func(t *testing.T) {
			if got := withinScope(test.rel, test.scope); got != test.want {
				t.Errorf("withinScope(%q, %q) = %v, want %v", test.rel, test.scope, got, test.want)
			}
		})
	}
}
