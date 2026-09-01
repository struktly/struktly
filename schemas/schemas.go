// The published Struktly schemas, carried in the binary that enforces them.
//
// These files are the contract. Until now they were documents this repository
// shipped and its tests checked against; nothing at runtime read them, so a
// command that wanted to enforce one had to restate its rules in Go — which is
// two expressions of one contract, and the second drifts.
//
// Embedding them closes that: `struktly verify` checks a bundle against
// schemas/record-bundle.v1.json itself, so the document a third party reads and
// the rules the verifier applies cannot disagree.
package schemas

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed *.json
var files embed.FS

// Names lists every published schema file.
func Names() ([]string, error) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return nil, fmt.Errorf("list schemas: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names, nil
}

// Bytes returns one published schema by file name, e.g. "record-bundle.v1.json".
func Bytes(name string) ([]byte, error) {
	raw, err := files.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read schema %s: %w", name, err)
	}
	return raw, nil
}
