package main

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/struktly/struktly/internal/schema"
	"github.com/struktly/struktly/schemas"
)

// The capabilities document is what a consumer negotiates against, so each of
// its lists is held to the binary it describes rather than to itself.

// unadvertisedCommands are in the tree and deliberately outside the machine
// contract: intel hands over to another program's contract, and cobra adds
// help and completion on its own.
var unadvertisedCommands = []string{"completion", "help", "intel"}

// presentationSchemas are Markdown documents that carry an identifier and have
// no JSON Schema, as docs/compatibility.md declares.
var presentationSchemas = []string{
	"struktly/agent-instructions/v1",
	"struktly/project-context/v1",
}

// inputSchemas are read by this CLI and never emitted, so they are published
// but not advertised.
var inputSchemas = []string{
	"struktly/capability-requirements/v1",
	"struktly/config/v1",
}

// commandPaths lists every command in the tree as capabilities names it, with
// the root omitted and subcommands space-joined.
func commandPaths(root *cobra.Command) []string {
	var paths []string
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, child := range cmd.Commands() {
			paths = append(paths, strings.TrimPrefix(child.CommandPath(), root.Name()+" "))
			walk(child)
		}
	}
	walk(root)
	return paths
}

func TestAdvertisedCommandsExistInCobraTree(t *testing.T) {
	root := newRootCmd()
	root.InitDefaultHelpCmd()
	root.InitDefaultCompletionCmd()
	tree := commandPaths(root)
	advertised := currentCapabilities().Commands

	for _, name := range advertised {
		if !slices.Contains(tree, name) {
			t.Errorf("capabilities advertise the command %q, which this binary does not define", name)
		}
	}
	for _, path := range tree {
		first, _, _ := strings.Cut(path, " ")
		if slices.Contains(unadvertisedCommands, first) || slices.Contains(advertised, path) {
			continue
		}
		t.Errorf("this binary defines the command %q, which capabilities neither advertise nor list as unadvertised", path)
	}
	for _, name := range unadvertisedCommands {
		if !slices.Contains(tree, name) {
			t.Errorf("unadvertisedCommands names %q, which the tree no longer defines", name)
		}
	}
}

func TestAdvertisedSchemasExistAndAreEnforceable(t *testing.T) {
	names, err := schemas.Names()
	if err != nil {
		t.Fatal(err)
	}
	published := map[string]string{}
	for _, name := range names {
		raw, err := schemas.Bytes(name)
		if err != nil {
			t.Fatal(err)
		}
		var header struct {
			ID string `json:"$id"`
		}
		if err := json.Unmarshal(raw, &header); err != nil || header.ID == "" {
			t.Fatalf("schemas/%s declares no $id: %v", name, err)
		}
		published[header.ID] = name
		// Validating an empty object reaches every keyword that applies before
		// a value is examined, which is where an unsupported one surfaces.
		if err := schema.ValidateJSON(raw, []byte(`{}`)); err != nil && strings.Contains(err.Error(), "unsupported keyword") {
			t.Errorf("schemas/%s uses a keyword internal/schema cannot enforce: %v", name, err)
		}
	}

	advertised := currentCapabilities().Schemas
	for _, id := range advertised {
		if _, ok := published[id]; ok || slices.Contains(presentationSchemas, id) {
			continue
		}
		t.Errorf("capabilities advertise the schema %q, which has no file in schemas/ and is not declared presentation-only", id)
	}
	for _, id := range slices.Sorted(maps.Keys(published)) {
		if slices.Contains(advertised, id) || slices.Contains(inputSchemas, id) {
			continue
		}
		t.Errorf("schemas/%s publishes %q, which capabilities neither advertise nor list as input-only", published[id], id)
	}
	for _, id := range presentationSchemas {
		if name, ok := published[id]; ok {
			t.Errorf("presentationSchemas names %q, but schemas/%s defines it", id, name)
		}
		if !slices.Contains(advertised, id) {
			t.Errorf("presentationSchemas names %q, which capabilities do not advertise", id)
		}
	}
	for _, id := range inputSchemas {
		if _, ok := published[id]; !ok {
			t.Errorf("inputSchemas names %q, which has no file in schemas/", id)
		}
		if slices.Contains(advertised, id) {
			t.Errorf("inputSchemas names %q, which capabilities advertise as emitted", id)
		}
	}
}
