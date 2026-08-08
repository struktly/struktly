package context

import (
	"strings"
	"testing"
)

func seededRepo(t *testing.T) string {
	t.Helper()
	root := initSelectionRepo(t)
	writeFile(t, root, "internal/obscure/helper.go", "package obscure\n\nfunc Nothing() {}\n")
	writeFile(t, root, "docs/history.md", "# History\n\nNothing relevant here.\n")
	return root
}

// A seed reaches a file that shares nothing with the request — no filename
// overlap, no declared identifier, no configured rule. That is the point.
func TestSeedIncludesAFileNoOtherReasonWouldReach(t *testing.T) {
	root := seededRepo(t)
	result, err := Brief(BriefOptions{
		Root: root, Task: "unrelated request about billing", NoWrite: true,
		Seeds: []string{"internal/obscure/helper.go", "docs/history.md"},
	})
	if err != nil {
		t.Fatalf("Brief returned error: %v", err)
	}
	found := map[string]PacketItem{}
	for _, item := range result.Packet.Items {
		found[item.Path] = item
	}
	for _, want := range []string{"internal/obscure/helper.go", "docs/history.md"} {
		item, ok := found[want]
		if !ok {
			t.Fatalf("seed %q was not selected: %v", want, selectedPaths(result.Packet.Items))
		}
		if item.Reason != "seed" {
			t.Errorf("seed %q reason = %q, want seed", want, item.Reason)
		}
		// A seed is the one reason the CLI does not derive, and the packet says so.
		if item.Provenance.Confidence != "declared" {
			t.Errorf("seed %q confidence = %q, want declared", want, item.Provenance.Confidence)
		}
	}
	if len(result.Packet.Seeds) != 2 {
		t.Fatalf("packet does not record the requested seeds: %#v", result.Packet.Seeds)
	}
}

// The property that matters most. "Reviewed" is the caller's judgement about
// relevance, not a claim that a file is safe to disclose: naming a file gets it
// considered, never included.
func TestSeedCannotBypassExclusionRules(t *testing.T) {
	root := seededRepo(t)
	const marker = "private-material-must-not-leak"
	writeFile(t, root, "config/app.key", "key material\n")
	writeFile(t, root, "internal/creds.go", "package internal\n\n// -----BEGIN PRIVATE KEY-----\n// "+marker+"\n")

	result, err := Brief(BriefOptions{
		Root: root, Task: "inspect configuration", NoWrite: true,
		Seeds: []string{"config/app.key", "internal/creds.go"},
	})
	if err != nil {
		t.Fatalf("Brief returned error: %v", err)
	}
	for _, item := range result.Packet.Items {
		if item.Path == "config/app.key" || item.Path == "internal/creds.go" {
			t.Fatalf("a seed bypassed an exclusion rule: %s", item.Path)
		}
	}
	assertDecision(t, result.Packet.Exclusions, "config/app.key", "sensitive_path")
	assertDecision(t, result.Packet.Exclusions, "internal/creds.go", "secret_detected")
	// The caller can still tell the seed was asked for and refused.
	if !containsString(result.Packet.Seeds, "config/app.key") {
		t.Fatalf("an excluded seed vanished from the record: %#v", result.Packet.Seeds)
	}
}

// Seeds outrank everything the CLI worked out, so a tight item limit spends
// itself on what the caller named before what the selector guessed.
func TestSeedsSurviveTheItemLimit(t *testing.T) {
	root := seededRepo(t)
	result, err := Brief(BriefOptions{
		Root: root, Task: "readme", NoWrite: true, MaxItems: 1,
		Seeds: []string{"docs/history.md"},
	})
	if err != nil {
		t.Fatalf("Brief returned error: %v", err)
	}
	if len(result.Packet.Items) != 1 || result.Packet.Items[0].Path != "docs/history.md" {
		t.Fatalf("the seed lost its place to a derived match: %v", selectedPaths(result.Packet.Items))
	}
}

func TestSeedsAreValidatedAgainstTheRepository(t *testing.T) {
	root := seededRepo(t)
	for name, test := range map[string]struct {
		seeds []string
		scope string
		want  string
	}{
		"outside the repository": {seeds: []string{"../elsewhere.go"}, want: "stay inside"},
		"absent":                 {seeds: []string{"internal/absent.go"}, want: "not a file"},
		"a directory":            {seeds: []string{"internal/obscure"}, want: "use --scope"},
		"outside the scope":      {seeds: []string{"docs/history.md"}, scope: "internal", want: "outside the requested scope"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Brief(BriefOptions{Root: root, Task: "anything", NoWrite: true, Seeds: test.seeds, Scope: test.scope})
			if err == nil {
				t.Fatalf("seed %v was accepted", test.seeds)
			}
			if !strings.Contains(err.Error(), ErrInvalidSeed.Error()) || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error does not explain the problem: %v", err)
			}
		})
	}
}

// Seeds are part of packet identity, and duplicates are not a different request.
func TestSeedsAreCanonicalAndPartOfIdentity(t *testing.T) {
	root := seededRepo(t)
	build := func(seeds ...string) Packet {
		t.Helper()
		result, err := Brief(BriefOptions{Root: root, Task: "readme", NoWrite: true, Seeds: seeds})
		if err != nil {
			t.Fatalf("Brief returned error: %v", err)
		}
		return result.Packet
	}
	plain := build()
	seeded := build("docs/history.md")
	if plain.PacketHash == seeded.PacketHash {
		t.Fatal("a seeded packet shares identity with an unseeded one")
	}

	duplicated := build("docs/history.md", "./docs/history.md", "docs/history.md")
	if len(duplicated.Seeds) != 1 {
		t.Fatalf("seeds were not deduplicated: %#v", duplicated.Seeds)
	}
	if duplicated.PacketHash != seeded.PacketHash {
		t.Fatal("naming one seed three ways produced a different packet")
	}
}
