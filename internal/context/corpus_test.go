package context

import (
	"encoding/json"
	"fmt"
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
	// limits: tighter than the defaults, for a case whose subject is what
	// happens at the boundary rather than what is chosen away from it.
	limits PacketLimits
	// wantTruncations: the packet must actually have truncated something. Only
	// meaningful with limits, and it is here so a case about truncation cannot
	// quietly stop truncating when a default or a fixture moves.
	wantTruncations bool
	// knownGap names a finding this case currently demonstrates.
	//
	// The labels stay what they should be — that is the whole value of a
	// labelled corpus, and weakening one to get a green run is how a corpus
	// stops measuring anything. Instead the assertion is inverted while the gap
	// is open: the case must fail. Closing the gap then fails this test and
	// forces somebody to delete the field, so a fix cannot land unnoticed and
	// an open gap cannot be forgotten.
	knownGap string
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
	{
		fixture: "agents-only",
		request: "cap the retry backoff",
		// The whole of this repository's guidance is AGENTS.md: no README, no
		// .struktly declarations. Guidance discovery that had quietly become
		// README-shaped would still pass every case above and fail this one.
		// drain.go is retry.go's package sibling, called by nothing selected,
		// and has nothing to do with backoff.
		mustSelect:  []string{"queue/retry.go", "AGENTS.md"},
		mustExclude: []string{"queue/drain.go"},
	},
	{
		fixture: "python-service",
		request: "round invoice totals correctly",
		// No Go anywhere, so declaration rendering, symbol matching and import
		// expansion all have nothing to work with. What is left is filename,
		// title and content, and this is the case that says those alone are
		// enough to reach the right file.
		//
		// rounding.md is titled for the subject and named for it too.
		// refund.py mentions an invoice total and is not where rounding is
		// decided; it is the distinction content matching has to make.
		mustSelect:  []string{"billing/invoice.py"},
		mustSurface: []string{"docs/rounding.md"},
		mustExclude: []string{"billing/refund.py"},
	},
	{
		fixture: "nested-modules",
		request: "record telemetry from the api checkout handler",
		// Four levels down, in one of two services. telemetry.go is here
		// because checkout.go calls telemetry.Count — the same reachability
		// claim go-service makes, at a depth where a selector that had learned
		// the shallow layout would stop.
		//
		// health.go is checkout.go's package sibling and consume.go is the
		// other service entirely.
		mustSelect:  []string{"services/api/internal/handler/checkout.go", "pkg/telemetry/telemetry.go"},
		mustExclude: []string{"services/api/internal/handler/health.go", "services/worker/internal/queue/consume.go"},
		knownGap:    "a request word matching a directory selects every file in it (.struktly/tasks/selection-precision-gaps.md)",
	},
	{
		fixture: "ambiguous-symbol",
		request: "close the store client cleanly",
		// Client is declared in three packages. The request names one of them,
		// so symbol matching must be narrowed by the rest of the request
		// rather than returning every declaration of the name.
		mustSelect:  []string{"store/client.go"},
		mustExclude: []string{"http/client.go", "worker/client.go"},
		knownGap:    "a symbol declared in several packages is not narrowed by the rest of the request (.struktly/tasks/selection-precision-gaps.md)",
	},
	{
		fixture: "ambiguous-symbol",
		request: "rename the Client type everywhere it is declared",
		// The opposite reading of the same repository: the request is about
		// every declaration, so a packet that carried one of them and said
		// nothing about the others would be wrong in a way no exclusion
		// catches. mustSurface rather than mustSelect — naming them is enough,
		// and carrying all three is the packet's decision, not this claim.
		mustSurface: []string{"store/client.go", "http/client.go", "worker/client.go"},
	},
	{
		fixture: "go-service",
		request: "add request timeout middleware",
		// The same request as the first case, at a byte budget the fixture
		// cannot fit. Deliberately only a byte budget: what is being measured
		// is that limits change how much of a file is carried and not which
		// files are worth carrying, so the answer to the request must still be
		// in the packet, truncated. Constraining the item count as well would
		// have made a failure here mean either thing.
		limits:          PacketLimits{MaxFileBytes: 220, MaxTotalBytes: 3000},
		wantTruncations: true,
		mustSelect:      []string{"middleware/timeout.go"},
	},
	{
		fixture: "go-service",
		request: "add request timeout middleware",
		// The item budget, which is the other half of the same boundary and
		// behaves differently: four items is tight but not absurd, and every
		// one of them goes to repository guidance, so the packet answers the
		// request with no code in it at all.
		//
		// Guidance is worth carrying. It is not worth the whole budget: a
		// packet that describes the repository and omits the file the request
		// is about has spent its room on the part a reader could have guessed.
		limits:     PacketLimits{MaxItems: 4},
		mustSelect: []string{"middleware/timeout.go"},
		knownGap:   "guidance fills a tight item budget before any code is selected (.struktly/tasks/selection-precision-gaps.md)",
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
		// Named by which budget binds, not by the whole struct: two cases on
		// one request differ only in that, and a report key has to be readable
		// in a diff.
		for _, budget := range []struct {
			label string
			value int
		}{
			{"items", test.limits.MaxItems},
			{"file-bytes", test.limits.MaxFileBytes},
			{"total-bytes", test.limits.MaxTotalBytes},
		} {
			if budget.value > 0 {
				name += fmt.Sprintf(" +%s=%d", budget.label, budget.value)
			}
		}
		t.Run(name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), test.fixture)
			copyTree(t, filepath.Join("testdata", "fixtures", test.fixture), root)
			initGitRepo(t, root)

			started := time.Now()
			result, err := Brief(BriefOptions{
				Root: root, Task: test.request, Scope: test.scope, Seeds: test.seeds,
				MaxItems: test.limits.MaxItems, MaxFileBytes: test.limits.MaxFileBytes,
				MaxTotalBytes: test.limits.MaxTotalBytes,
				NoWrite:       true, Now: time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC),
			})
			if err != nil {
				t.Fatalf("Brief returned error: %v", err)
			}
			elapsed := time.Since(started)

			selected := map[string]string{}
			for _, item := range result.Packet.Items {
				selected[item.Path] = item.Reason
			}
			// Label failures are collected rather than reported as they are
			// found, because a case carrying a knownGap is asserting that at
			// least one of them still happens.
			var problems []string
			note := func(format string, args ...any) {
				problems = append(problems, fmt.Sprintf(format, args...))
			}
			for _, want := range test.mustSelect {
				if _, ok := selected[want]; !ok {
					note("recall: %q was not selected; got %v", want, sortedKeys(selected))
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
					note("reachability: %q is neither selected nor suggested", want)
				}
			}
			if test.wantTruncations && len(result.Packet.Truncations) == 0 {
				t.Errorf("this case exists to measure the limit boundary and nothing was truncated; "+
					"the limits %+v no longer bind against this fixture", test.limits)
			}
			for _, unwanted := range test.mustExclude {
				if reason, ok := selected[unwanted]; ok {
					note("precision: %q was selected as %q and should not have been", unwanted, reason)
				}
			}

			switch {
			case test.knownGap == "":
				for _, problem := range problems {
					t.Error(problem)
				}
			case len(problems) == 0:
				t.Errorf("the known gap %q no longer reproduces.\n"+
					"If selection improved, delete knownGap from this case and let it "+
					"assert normally; that is the point of recording it here rather "+
					"than in a document.", test.knownGap)
			default:
				t.Logf("known gap %q still open:\n  %s", test.knownGap, strings.Join(problems, "\n  "))
			}

			// Determinism, per case rather than once: a selection signal that
			// depends on map order fails here and nowhere else.
			repeat, err := Brief(BriefOptions{
				Root: root, Task: test.request, Scope: test.scope, Seeds: test.seeds,
				MaxItems: test.limits.MaxItems, MaxFileBytes: test.limits.MaxFileBytes,
				MaxTotalBytes: test.limits.MaxTotalBytes,
				NoWrite:       true, Now: time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
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
	for _, fixture := range []string{
		"go-service", "flat-package", "noisy-legacy",
		"agents-only", "python-service", "nested-modules", "ambiguous-symbol",
	} {
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
