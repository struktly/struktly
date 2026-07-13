package flatpackage

import "strings"

// Join renders fields back into one line.
func Join(fields []string) string {
	return strings.Join(fields, " ")
}
