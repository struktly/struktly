package context

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// The quality corpus. Golden files lock the shape of output; this locks its
// quality, which is a different claim and the one that moves when selection
// changes.
//
// Each case names files that must be selected for a request and files that must
// not be. Recall over `mustSelect` has to be total: these are not preferences
// but the reason the request would be asked at all. `mustExclude` covers both
// irrelevance and disclosure.
//
// Deliberately not a precision metric. Labelling every file in a fixture as
// relevant or not is a judgement I would be inventing, and a number derived
// from an invented label reads as evidence without being any. Selected-item
// counts are recorded in the report instead, where growth is visible without
// being dressed up as accuracy.
type corpusCase struct {
	fixture string
	request string
	scope   string
	seeds   []string
	// mustSelect: content must be in the packet.
	mustSelect []string
	// mustSurface: the packet must at least point at the file, whether by
	// including it or by listing it under suggested files. Weaker than
	// mustSelect on purpose — the packet distinguishes carrying a file from
	// naming one, and a corpus that blurred them would hide which it did.
	mustSurface []string
	mustExclude []string
}

var corpus = []corpusCase{
	{
		fixture: "go-service",
		request: "add request timeout middleware",
		// clock.go is here only because timeout.go calls clock.Grace: nothing in
		// its path, its identifiers or its package name matches the request.
		// unused.go is its sibling in the same package, called by nothing
		// selected, and is the distinction import expansion has to make —
		// reachability is not relevance.
		mustSelect:  []string{"middleware/timeout.go", "README.md", ".struktly/constraints.md", "internal/clock/clock.go"},
		mustExclude: []string{"docs/adr/0001-record.md", "internal/clock/unused.go"},
	},
	{
		fixture: "go-service",
		request: "document the architecture decisions",
		// 0001-record.md is titled "ADR 0001: Record architecture decisions".
		// Its filename is a serial number, so only its title reaches it.
		mustSelect: []string{"docs/architecture.md", "docs/adr/0001-record.md"},
	},
	{
		fixture:     "go-service",
		request:     "add request timeout middleware",
		scope:       "middleware",
		mustSelect:  []string{"middleware/timeout.go", ".struktly/constraints.md"},
		mustExclude: []string{"docs/architecture.md", "README.md"},
	},
	{
		fixture:    "go-service",
		request:    "unrelated request about billing",
		seeds:      []string{"middleware/logger.go"},
		mustSelect: []string{"middleware/logger.go"},
	},
	{
		fixture:    "flat-package",
		request:    "fix the config parser",
		mustSelect: []string{"config.go", "parser.go"},
	},
	{
		fixture: "noisy-legacy",
		request: "update the current documentation",
		// Archived and fixture trees are deprioritized, never treated as current.
		mustSelect:  []string{"docs/current.md"},
		mustExclude: []string{"_legacy/docs/old-system.md", "archive/spec.md", "legacy/docs/old-plan.md", "testdata/fixture.txt"},
	},
}

// corpusMeasurement is one case's recorded numbers. Latency is deliberately
// absent: it is machine-dependent, and a committed figure that differs between
// a laptop and three CI runners is noise that trains people to ignore the file.
// It is measured and printed instead, where a human can read it in context.
type corpusMeasurement struct {
	Items       int      `json:"items"`
	PacketBytes int      `json:"packet_bytes"`
	Reasons     []string `json:"reasons"`
	Exclusions  int      `json:"exclusions"`
	Truncations int      `json:"truncations"`
}

func TestSelectionQualityCorpus(t *testing.T) {
	measured := map[string]corpusMeasurement{}

	for _, test := range corpus {
		name := test.fixture + " / " + test.request
		if test.scope != "" {
			name += " @" + test.scope
		}
		if len(test.seeds) > 0 {
			name += " +seeds"
		}
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), test.fixture)
			copyTree(t, filepath.Join("testdata", "fixtures", test.fixture), root)
			initGitRepo(t, root)

			started := time.Now()
			result, err := Brief(BriefOptions{
				Root: root, Task: test.request, Scope: test.scope, Seeds: test.seeds,
				NoWrite: true, Now: time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC),
			})
			if err != nil {
				t.Fatalf("Brief returned error: %v", err)
			}
			elapsed := time.Since(started)

			selected := map[string]string{}
			for _, item := range result.Packet.Items {
				selected[item.Path] = item.Reason
			}
			for _, want := range test.mustSelect {
				if _, ok := selected[want]; !ok {
					t.Errorf("recall: %q was not selected; got %v", want, sortedKeys(selected))
				}
			}
			surfaced := map[string]struct{}{}
			for path := range selected {
				surfaced[path] = struct{}{}
			}
			for _, path := range result.Packet.SuggestedFiles {
				surfaced[path] = struct{}{}
			}
			for _, want := range test.mustSurface {
				if _, ok := surfaced[want]; !ok {
					t.Errorf("reachability: %q is neither selected nor suggested", want)
				}
			}
			for _, unwanted := range test.mustExclude {
				if reason, ok := selected[unwanted]; ok {
					t.Errorf("precision: %q was selected as %q and should not have been", unwanted, reason)
				}
			}

			// Determinism, per case rather than once: a selection signal that
			// depends on map order fails here and nowhere else.
			repeat, err := Brief(BriefOptions{
				Root: root, Task: test.request, Scope: test.scope, Seeds: test.seeds,
				NoWrite: true, Now: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
			})
			if err != nil {
				t.Fatalf("Brief returned error: %v", err)
			}
			if repeat.Packet.PacketHash != result.Packet.PacketHash {
				t.Errorf("determinism: two runs produced %s and %s",
					result.Packet.PacketHash, repeat.Packet.PacketHash)
			}

			assertConforms(t, "packet.v2.json", result.Packet)

			encoded, err := json.Marshal(result.Packet)
			if err != nil {
				t.Fatal(err)
			}
			reasons := map[string]struct{}{}
			for _, reason := range selected {
				reasons[reason] = struct{}{}
			}
			measured[name] = corpusMeasurement{
				Items:       len(result.Packet.Items),
				PacketBytes: len(encoded),
				Reasons:     sortedSet(reasons),
				Exclusions:  len(result.Packet.Exclusions),
				Truncations: len(result.Packet.Truncations),
			}
			t.Logf("%d items, %d packet bytes, %v", len(result.Packet.Items), len(encoded), elapsed.Round(time.Millisecond))
		})
	}

	compareCorpusReport(t, measured)
}

// compareCorpusReport holds the recorded numbers the way the golden files hold
// recorded output: a change is legitimate, but it has to appear in a diff and
// be looked at rather than drifting.
func compareCorpusReport(t *testing.T, measured map[string]corpusMeasurement) {
	t.Helper()
	path := filepath.Join("testdata", "corpus", "report.json")
	encoded, err := json.MarshalIndent(measured, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	encoded = append(encoded, '\n')

	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, encoded, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	recorded, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read corpus report: %v; regenerate with -update", err)
	}
	if string(recorded) != string(encoded) {
		t.Errorf("selection quality moved.\nrecorded:\n%s\nmeasured:\n%s\n"+
			"Verify the change is an improvement, then rerun with -update.", recorded, encoded)
	}
}

// Secret exclusion is asserted against every fixture rather than one, because
// it is the property with the worst failure and the least tolerance.
func TestCorpusNeverDisclosesPlantedSecrets(t *testing.T) {
	const marker = "private-material-must-not-leak"
	for _, fixture := range []string{"go-service", "flat-package", "noisy-legacy"} {
		t.Run(fixture, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), fixture)
			copyTree(t, filepath.Join("testdata", "fixtures", fixture), root)
			// Planted where each guard should catch it: content, filename, and
			// a guidance file the packet copies into its own fields.
			writeFile(t, root, "internal/creds.go", "package internal\n\n// -----BEGIN PRIVATE KEY-----\n// "+marker+"\n")
			writeFile(t, root, "config/app.key", marker+"\n")
			writeFile(t, root, "secrets/database.txt", marker+"\n")
			writeFile(t, root, ".struktly/decisions.md", "# Decisions\n\nghp_"+strings.Repeat("A", 36)+"\n"+marker+"\n")
			initGitRepo(t, root)

			for _, request := range []string{
				"inspect credentials and keys",
				"review database secrets configuration",
				"read the decisions ledger",
			} {
				result, err := Brief(BriefOptions{Root: root, Task: request, NoWrite: true})
				if err != nil {
					t.Fatalf("Brief returned error: %v", err)
				}
				encoded, err := json.Marshal(result.Packet)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(string(encoded), marker) {
					t.Fatalf("request %q disclosed planted material", request)
				}
			}
		})
	}
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedSet(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
