package files

import (
	"fmt"
	"strings"
	"time"
)

// OKFFrontmatter renders an Open Knowledge Format (OKF v0.1) frontmatter
// block. `type` is the only required OKF field; title, description, and
// timestamp are recommended fields.
func OKFFrontmatter(docType, title, description string, ts time.Time) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("type: " + docType + "\n")
	schemaType := docType
	if schemaType == "context-packet" {
		schemaType = "packet"
	} else if schemaType == "evidence-log" {
		schemaType = "evidence"
	}
	b.WriteString("schema: struktly/" + schemaType + "/v1\n")
	if title != "" {
		fmt.Fprintf(&b, "title: %q\n", title)
	}
	if description != "" {
		fmt.Fprintf(&b, "description: %q\n", description)
	}
	if !ts.IsZero() {
		b.WriteString("timestamp: " + ts.UTC().Format(time.RFC3339) + "\n")
	}
	b.WriteString("---\n\n")
	return b.String()
}

// StripFrontmatter removes a leading YAML frontmatter block so file content
// can be embedded or excerpted without leaking metadata into rendered output.
func StripFrontmatter(content string) string {
	rest, ok := strings.CutPrefix(content, "---\n")
	if !ok {
		return content
	}
	_, after, found := strings.Cut(rest, "\n---\n")
	if !found {
		return content
	}
	return strings.TrimLeft(after, "\n")
}
