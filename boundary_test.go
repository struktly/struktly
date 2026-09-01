package struktly_test

import (
	"go/build"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The rules that keep this binary installable and offline.
//
// README.md says the CLI runs locally and does not call a model or upload
// source code. CONTRIBUTING.md says not to add a dependency the standard
// library or an existing one already covers. docs/roadmap.md says no roadmap
// item requires a network call. All three were prose until this file existed,
// and prose is enforced by whoever happens to remember it at the moment a
// convenient import arrives.
//
// Here that costs more than it does in a private module. This is the
// repository people install from, so an import that resolves on a maintainer's
// machine and nowhere else breaks `go install` for everyone, and nothing else
// in this repository would notice.

// moduleImportPrefix is this module's own path. Its packages import each other
// freely: that is not a dependency anybody inherits.
const moduleImportPrefix = "github.com/struktly/struktly/"

// allowedDependencies is the whole of what installing this command pulls in
// beyond the standard library. cobra brings pflag, and both are named because
// both appear in an import graph.
//
// Adding an entry here is a deliberate act with a cost attached: every
// consumer inherits it, including the ones that read this repository precisely
// because they intend to audit what reaches their source. Prefer the standard
// library, and prefer doing without.
var allowedDependencies = []string{
	"github.com/spf13/cobra",
	"github.com/spf13/pflag",
}

// networkPackages are the standard library's routes off this machine.
//
// The claim this defends is the one the README leads with, and it is the
// reason the repository is public at all: what the CLI selects can be checked
// by reading it, which is only worth anything while the CLI cannot also send
// it somewhere. A packet is a local file, and every consumer transports it
// itself.
//
// net/url is deliberately absent. Parsing a URL is not reaching one.
var networkPackages = map[string]string{
	"net":        "opening a socket",
	"net/http":   "making a request",
	"net/rpc":    "calling another process over a socket",
	"net/smtp":   "sending mail",
	"crypto/tls": "establishing a transport nothing here should need",
}

// packageDirectories lists every directory in this module that holds Go
// source.
//
// testdata is skipped for the reason the Go tool skips it: those are fixture
// repositories, sample sources this CLI reads and selects from, not code it
// compiles. A fixture is allowed to import net/http, and one does.
func packageDirectories(t *testing.T) []string {
	t.Helper()
	var dirs []string
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			return nil
		}
		name := entry.Name()
		if path != "." && (strings.HasPrefix(name, ".") || name == "testdata") {
			return filepath.SkipDir
		}
		if matches, _ := filepath.Glob(filepath.Join(path, "*.go")); len(matches) > 0 {
			dirs = append(dirs, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) == 0 {
		t.Fatal("no Go packages were found, so this test proved nothing")
	}
	return dirs
}

// imports returns every path a directory's package reaches for, including from
// its tests.
//
// Test imports are included rather than exempted. A test that reaches the
// network reaches it from somebody's CI, and a test that needs a new module
// still writes that module into go.mod, where the next reader takes it for a
// dependency of the tool.
func imports(t *testing.T, dir string) []string {
	t.Helper()
	pkg, err := build.ImportDir(dir, 0)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	all := append([]string{}, pkg.Imports...)
	all = append(all, pkg.TestImports...)
	return append(all, pkg.XTestImports...)
}

// external reports whether an import path leaves the standard library. A
// standard-library path has no dot in its first segment, which is the same
// test the module system makes.
func external(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return strings.Contains(first, ".")
}

func TestTheModuleCarriesOneDependency(t *testing.T) {
	for _, dir := range packageDirectories(t) {
		for _, path := range imports(t, dir) {
			switch {
			case !external(path):
			case strings.HasPrefix(path, moduleImportPrefix):
			case slices.Contains(allowedDependencies, path):
			case strings.HasPrefix(path, "github.com/struktly/"):
				t.Errorf("%s imports %q.\n"+
					"Modules under github.com/struktly/ other than this one are not "+
					"published. This one is, and people install it with `go install`, so "+
					"a dependency on an unpublished sibling compiles on a maintainer's "+
					"machine and for nobody else. Pass the value in, or write the few "+
					"lines this needs here.", dir, path)
			default:
				t.Errorf("%s imports %q, which is neither the standard library nor an "+
					"allowed dependency.\n"+
					"Every consumer of this CLI inherits it. If it is genuinely worth "+
					"that, add it to allowedDependencies with the reason, and say so in "+
					"the pull request rather than in a go.mod diff.", dir, path)
			}
		}
	}
}

func TestNothingReachesTheNetwork(t *testing.T) {
	for _, dir := range packageDirectories(t) {
		for _, path := range imports(t, dir) {
			if why, forbidden := networkPackages[path]; forbidden {
				t.Errorf("%s imports %q: %s.\n"+
					"This CLI reads a repository and writes a file. It does not call a "+
					"model, upload source, or fetch anything, and that is checkable here "+
					"rather than promised in the README. Whatever needs the network is "+
					"the caller's, and the packet is how it gets there.", dir, path, why)
			}
		}
	}
}
