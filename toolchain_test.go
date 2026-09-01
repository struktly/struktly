package struktly_test

import (
	"os"
	"regexp"
	"testing"
)

// One declared Go floor.
//
// The minimum Go this module supports is stated in five places: the `go`
// directive, the mise pin, the README badge, the README prose, and
// CONTRIBUTING. Nothing compared them. CI resolves go.mod for lint and builds
// the matrix on stable, so a raised directive would be noticed — but a stale
// badge would not, and the badge is the first thing a prospective installer
// reads.
//
// .struktly/decisions.md records which one is authoritative and why it is that
// one. This holds the rest to it.

// floorPattern reads the `go` directive. It is the authority; everything below
// is checked against whatever it says, so raising the floor stays a one-line
// edit plus whatever this test then reports.
var floorPattern = regexp.MustCompile(`(?m)^go (\d+\.\d+)(\.\d+)?$`)

func declaredFloor(t *testing.T) (full, short string) {
	t.Helper()
	body, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatal(err)
	}
	match := floorPattern.FindStringSubmatch(string(body))
	if match == nil {
		t.Fatal("go.mod declares no `go` directive, so there is no floor to hold anything to")
	}
	return match[1] + match[2], match[1]
}

func TestEveryStatementOfTheGoFloorAgreesWithGoMod(t *testing.T) {
	full, short := declaredFloor(t)

	for _, statement := range []struct {
		file    string
		what    string
		pattern *regexp.Regexp
		want    string
	}{
		{
			file:    "mise.toml",
			what:    "the mise pin",
			pattern: regexp.MustCompile(`(?m)^go = "([^"]+)"$`),
			want:    full,
		},
		{
			// Shields encodes the `+` of "1.25+" as %2B.
			file:    "README.md",
			what:    "the README badge",
			pattern: regexp.MustCompile(`Go-(\d+\.\d+)%2B`),
			want:    short,
		},
		{
			file:    "README.md",
			what:    "the README prose",
			pattern: regexp.MustCompile(`Go (\d+\.\d+) or newer`),
			want:    short,
		},
		{
			file:    "CONTRIBUTING.md",
			what:    "the CONTRIBUTING prose",
			pattern: regexp.MustCompile(`Go (\d+\.\d+) or newer`),
			want:    short,
		},
	} {
		body, err := os.ReadFile(statement.file)
		if err != nil {
			t.Errorf("reading %s: %v", statement.file, err)
			continue
		}
		matches := statement.pattern.FindAllStringSubmatch(string(body), -1)
		if len(matches) == 0 {
			t.Errorf("%s no longer states the Go floor, so nothing holds it to go.mod.\n"+
				"Either restore it or delete this check — a statement that stopped being "+
				"checked is worse than one that was never made.", statement.what)
			continue
		}
		// Every occurrence, not the first. A second mention is exactly where a
		// half-finished raise leaves the old number.
		for _, match := range matches {
			if match[1] != statement.want {
				t.Errorf("%s says Go %s, and go.mod declares %s.\n"+
					"go.mod is authoritative (.struktly/decisions.md); update %s to match it.",
					statement.what, match[1], statement.want, statement.file)
			}
		}
	}
}
