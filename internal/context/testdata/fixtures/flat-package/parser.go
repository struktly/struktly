package flatpackage

import "strings"

// Parse splits input into trimmed, non-empty fields.
func Parse(input string) []string {
	return strings.Fields(strings.TrimSpace(input))
}
