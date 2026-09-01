package struktly_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// This is the repository people install from, and README.md, CONTRIBUTING.md
// and docs/roadmap.md all promise the same two things about it: it reaches no
// network, and it carries almost nothing. These tests hold it to them.

const modulePath = "github.com/struktly/struktly"

// allowedDependencies is every module compiled into the installed command.
// mousetrap arrives with cobra and is linked on Windows. Adding an entry is a
// deliberate act with a cost attached: every consumer inherits it, including
// the ones that read this repository precisely because they intend to audit
// what reaches their source.
var allowedDependencies = []string{
	"github.com/inconshreveable/mousetrap",
	"github.com/spf13/cobra",
	"github.com/spf13/pflag",
}

// networkPackages are the standard library's routes off this machine. net/url
// is deliberately absent: parsing a URL is not reaching one.
var networkPackages = map[string]string{
	"net":        "opening a socket",
	"net/http":   "making a request",
	"net/rpc":    "calling another process over a socket",
	"net/smtp":   "sending mail",
	"crypto/tls": "establishing a transport nothing here should need",
}

// inheritedNetworkPackages are network packages an allowed dependency already
// links, against the dependency answerable for each. pflag imports net for its
// IP flag types; no code path in this module reaches them.
var inheritedNetworkPackages = map[string]string{
	"net": "github.com/spf13/pflag",
}

// buildTargets are the platforms CI tests and releases publish for.
var buildTargets = []string{"darwin", "linux", "windows"}

// packageDirectories lists every directory in this module that holds Go source.
//
// testdata is skipped for the reason the Go tool skips it: those are fixture
// repositories this CLI reads and selects from, and one of them imports
// net/http.
func packageDirectories(t *testing.T) []string {
	t.Helper()
	dirs := map[string]struct{}{}
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		name := entry.Name()
		if entry.IsDir() {
			if path != "." && (strings.HasPrefix(name, ".") || name == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(name, ".go") {
			dirs[filepath.Dir(path)] = struct{}{}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) == 0 {
		t.Fatal("no Go packages were found, so this test proved nothing")
	}
	return slices.Sorted(maps.Keys(dirs))
}

// imports returns every path the Go files in dir reach for, tests included.
//
// The files are parsed rather than resolved through go/build, so a build
// constraint cannot hide an import from this check by not applying to whatever
// platform the suite happens to be running on.
func imports(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	fileSet := token.NewFileSet()
	var paths []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		name := filepath.Join(dir, entry.Name())
		file, err := parser.ParseFile(fileSet, name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatalf("%s: unquoting import %s: %v", name, spec.Path.Value, err)
			}
			paths = append(paths, path)
		}
	}
	return paths
}

// listedPackage is the part of `go list -json` these tests read.
type listedPackage struct {
	ImportPath string
	Module     *struct{ Path string }
	Imports    []string
}

// linkedPackages returns every package compiled into the command for goos.
// Imports alone cannot answer this: what a consumer inherits is the whole
// graph, and most of it is reached through a dependency rather than named here.
func linkedPackages(t *testing.T, goos string) []listedPackage {
	t.Helper()
	command := exec.Command("go", "list", "-deps", "-json=ImportPath,Module,Imports", "./cmd/struktly")
	command.Env = append(os.Environ(), "GOOS="+goos, "CGO_ENABLED=0")
	var stderr bytes.Buffer
	command.Stderr = &stderr
	out, err := command.Output()
	if err != nil {
		t.Fatalf("go list for %s: %v\n%s", goos, err, stderr.String())
	}
	var packages []listedPackage
	decoder := json.NewDecoder(bytes.NewReader(out))
	for {
		var pkg listedPackage
		err := decoder.Decode(&pkg)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("decoding go list output for %s: %v", goos, err)
		}
		packages = append(packages, pkg)
	}
	return packages
}

// external reports whether an import path leaves the standard library. A
// standard-library path has no dot in its first segment, which is the same test
// the module system makes.
func external(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return strings.Contains(first, ".")
}

// fromModule reports whether an import path belongs to module.
func fromModule(path, module string) bool {
	return path == module || strings.HasPrefix(path, module+"/")
}

func TestTheInstalledCommandLinksOnlyAllowedModules(t *testing.T) {
	linked := map[string]string{}
	for _, goos := range buildTargets {
		for _, pkg := range linkedPackages(t, goos) {
			if pkg.Module == nil || pkg.Module.Path == modulePath {
				continue
			}
			if _, seen := linked[pkg.Module.Path]; !seen {
				linked[pkg.Module.Path] = goos
			}
		}
	}
	for _, module := range slices.Sorted(maps.Keys(linked)) {
		if !slices.Contains(allowedDependencies, module) {
			t.Errorf("the %s build links %q, which is not an allowed dependency.\n"+
				"Every consumer of this CLI inherits it. If it is genuinely worth "+
				"that, add it to allowedDependencies with the reason, and say so in "+
				"the pull request rather than in a go.mod diff.", linked[module], module)
		}
	}
	for _, module := range allowedDependencies {
		if _, ok := linked[module]; !ok {
			t.Errorf("allowedDependencies names %q, which no build links any more.\n"+
				"Remove it: the list is only worth reading while it is exact.", module)
		}
	}
}

// Imports of a sibling under github.com/struktly/ are held separately from the
// linked graph above, because a test-only import of one never reaches it and
// still breaks the build for everyone.
func TestNoPackageImportsAnUnpublishedSibling(t *testing.T) {
	for _, dir := range packageDirectories(t) {
		for _, path := range imports(t, dir) {
			switch {
			case !external(path), fromModule(path, modulePath):
			case slices.ContainsFunc(allowedDependencies, func(module string) bool {
				return fromModule(path, module)
			}):
			case strings.HasPrefix(path, "github.com/struktly/"):
				t.Errorf("%s imports %q.\n"+
					"Modules under github.com/struktly/ other than this one are not "+
					"published. This one is, and people install it with `go install`, so "+
					"a dependency on an unpublished sibling compiles on a maintainer's "+
					"machine and for nobody else. Pass the value in, or write the few "+
					"lines this needs here.", dir, path)
			default:
				t.Errorf("%s imports %q, which is neither the standard library nor an "+
					"allowed dependency.", dir, path)
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
					"model, upload source, or fetch anything. Whatever needs the network "+
					"is the caller's, and the packet is how it gets there.", dir, path, why)
			}
		}
	}

	// What the dependencies bring, which no import in this repository shows.
	// Each edge is held against inheritedNetworkPackages, so an allowance that
	// stops being true fails here rather than widening quietly.
	for _, goos := range buildTargets {
		importers := map[string][]string{}
		for _, pkg := range linkedPackages(t, goos) {
			for _, imported := range pkg.Imports {
				if _, forbidden := networkPackages[imported]; forbidden {
					importers[imported] = append(importers[imported], pkg.ImportPath)
				}
			}
		}
		for _, imported := range slices.Sorted(maps.Keys(importers)) {
			source, inherited := inheritedNetworkPackages[imported]
			for _, importer := range importers[imported] {
				if inherited && fromModule(importer, source) {
					continue
				}
				t.Errorf("the %s build links %q (%s), imported by %q.\n"+
					"A dependency that reaches the network reaches it from every machine "+
					"this is installed on. Drop the dependency, or record the package in "+
					"inheritedNetworkPackages with the reason it cannot be called.",
					goos, imported, networkPackages[imported], importer)
			}
		}
	}
}
