package context

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// importNeighbor is a file that declares something a selected file calls.
type importNeighbor struct {
	path string
	// provides are the identifiers the selected files use from this file, named
	// in provenance so the packet says what the file is here to supply.
	provides []string
}

// modulePath reads the module line from the repository's root go.mod. Only the
// root module is read: a nested module's import paths will not resolve and its
// neighbours are simply not found, which is a smaller wrong than guessing at a
// prefix and pulling in unrelated directories.
func modulePath(root string) string {
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		if declared, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.TrimSpace(declared)
		}
	}
	return ""
}

func parseGoFile(root, rel string, mode parser.Mode) (*ast.File, bool) {
	full := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Lstat(full)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxDeclarationParseBytes {
		return nil, false
	}
	file, err := parser.ParseFile(token.NewFileSet(), full, nil, mode|parser.SkipObjectResolution)
	if err != nil {
		return nil, false
	}
	return file, true
}

// findImportNeighbors returns repository files that declare identifiers the
// already-selected files actually use.
//
// Following imports alone does not work, and measuring it is what showed why:
// a Go import names a package, a package is a directory, and a directory can be
// fifteen files. Expanding `internal/app/inspect.go` by its imports added every
// file in `internal/context` to a request about task frontmatter, doubling the
// packet with code that had nothing to do with it. Reachability is not
// relevance.
//
// So the unit is the identifier, not the package. `files.CleanRoot` pulls in
// whichever file declares CleanRoot and leaves its siblings alone, and the
// packet can say which identifier earned each file its place.
//
// First-degree only. Transitive expansion reaches most of a repository within
// two or three steps, which is the opposite of selecting context.
func findImportNeighbors(root string, selected []PacketItem, considered map[string]struct{}, tracked []string, eligible func(string) bool) []importNeighbor {
	module := modulePath(root)
	if module == "" {
		return nil
	}
	byDirectory := map[string][]string{}
	for _, rel := range tracked {
		// A dependency's tests describe how that package is verified, not what
		// the selected file uses.
		if !isGoSource(rel) || strings.HasSuffix(rel, "_test.go") {
			continue
		}
		byDirectory[path.Dir(rel)] = append(byDirectory[path.Dir(rel)], rel)
	}

	resolver := packageResolver{root: root, byDirectory: byDirectory, names: map[string]string{}}
	// directory -> identifier -> selected files using it
	used := map[string]map[string]struct{}{}
	for _, item := range selected {
		if !isGoSource(item.Path) {
			continue
		}
		file, ok := parseGoFile(root, item.Path, parser.AllErrors)
		if !ok {
			continue
		}
		local := resolver.localNames(file, module)
		if len(local) == 0 {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			qualifier, ok := selector.X.(*ast.Ident)
			if !ok {
				return true
			}
			directory, ok := local[qualifier.Name]
			if !ok {
				return true
			}
			if used[directory] == nil {
				used[directory] = map[string]struct{}{}
			}
			used[directory][selector.Sel.Name] = struct{}{}
			return true
		})
	}

	provides := map[string]map[string]struct{}{}
	for directory, identifiers := range used {
		for _, rel := range byDirectory[directory] {
			if _, seen := considered[rel]; seen {
				continue
			}
			if !eligible(rel) {
				continue
			}
			file, ok := parseGoFile(root, rel, parser.SkipObjectResolution)
			if !ok {
				continue
			}
			for _, name := range declaredNames(file) {
				if _, wanted := identifiers[name]; !wanted {
					continue
				}
				if provides[rel] == nil {
					provides[rel] = map[string]struct{}{}
				}
				provides[rel][name] = struct{}{}
			}
		}
	}

	neighbors := make([]importNeighbor, 0, len(provides))
	for rel, names := range provides {
		neighbors = append(neighbors, importNeighbor{path: rel, provides: sortedSetOf(names)})
	}
	// A file supplying more of what the selected code calls is more central to
	// it; path order breaks ties so the result is stable.
	sort.Slice(neighbors, func(i, j int) bool {
		if len(neighbors[i].provides) != len(neighbors[j].provides) {
			return len(neighbors[i].provides) > len(neighbors[j].provides)
		}
		return neighbors[i].path < neighbors[j].path
	})
	return neighbors
}

// packageResolver maps the name a file uses for an import to the repository
// directory it refers to.
type packageResolver struct {
	root        string
	byDirectory map[string][]string
	names       map[string]string
}

// localNames returns the qualifier each in-repository import is known by inside
// this file.
func (r packageResolver) localNames(file *ast.File, module string) map[string]string {
	local := map[string]string{}
	for _, spec := range file.Imports {
		imported, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		suffix, ok := strings.CutPrefix(imported, module)
		if !ok {
			// Outside this repository: its source is not in the tree.
			continue
		}
		directory := strings.TrimPrefix(suffix, "/")
		if directory == "" {
			directory = "."
		}
		name := ""
		switch {
		case spec.Name == nil:
			// Unaliased: the qualifier is the imported package's name, which is
			// not always its directory name, so it is read rather than assumed.
			name = r.packageName(directory)
		case spec.Name.Name == "_", spec.Name.Name == ".":
			// A blank import uses nothing; a dot import has no qualifier to
			// attribute a use to. Neither yields an identifier to follow.
			continue
		default:
			name = spec.Name.Name
		}
		if name != "" {
			local[name] = directory
		}
	}
	return local
}

func (r packageResolver) packageName(directory string) string {
	if name, ok := r.names[directory]; ok {
		return name
	}
	name := ""
	for _, rel := range r.byDirectory[directory] {
		if file, ok := parseGoFile(r.root, rel, parser.PackageClauseOnly); ok {
			name = file.Name.Name
			break
		}
	}
	r.names[directory] = name
	return name
}

func sortedSetOf(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
