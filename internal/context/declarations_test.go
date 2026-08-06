package context

import (
	stdcontext "context"
	"fmt"
	"strings"
	"testing"
)

const declarationSource = `package server

import (
	"context"
	"net/http"
	"time"
)

// Config configures a Server.
type Config struct {
	Addr string
	// Timeout bounds a single request.
	Timeout time.Duration
}

// New returns a Server bound to cfg.
func New(cfg Config) *Server {
	// This comment lives inside a body and must not survive.
	s := &Server{cfg: cfg}
	return s
}

// WithTimeout wraps h, cancelling after d.
func WithTimeout(h http.Handler, d time.Duration) http.Handler {
	return http.TimeoutHandler(h, d, "timeout")
}

func (s *Server) Shutdown(ctx context.Context) error { return nil }
`

func TestGoDeclarationsKeepsSignaturesAndDropsBodies(t *testing.T) {
	rendered, ok := goDeclarations([]byte(declarationSource))
	if !ok {
		t.Fatal("goDeclarations refused a valid file")
	}
	for _, want := range []string{
		"package server",
		"// Config configures a Server.",
		"Timeout time.Duration",
		"// New returns a Server bound to cfg.",
		"func New(cfg Config) *Server",
		"// WithTimeout wraps h, cancelling after d.",
		"func WithTimeout(h http.Handler, d time.Duration) http.Handler",
		"func (s *Server) Shutdown(ctx context.Context) error",
	} {
		if !strings.Contains(rendered, want) {
			t.Errorf("rendering lost %q:\n%s", want, rendered)
		}
	}
	for _, unwanted := range []string{
		"s := &Server{cfg: cfg}",
		"http.TimeoutHandler",
		"lives inside a body",
	} {
		if strings.Contains(rendered, unwanted) {
			t.Errorf("rendering kept body content %q:\n%s", unwanted, rendered)
		}
	}
	if len(rendered) >= len(declarationSource) {
		t.Errorf("rendering did not shrink the file: %d >= %d", len(rendered), len(declarationSource))
	}
}

// A file mid-edit does not parse. Byte truncation is a worse summary but an
// honest one, so the renderer declines rather than inventing a partial tree.
func TestGoDeclarationsDeclinesUnparseableSource(t *testing.T) {
	if _, ok := goDeclarations([]byte("package server\n\nfunc Broken( {\n")); ok {
		t.Fatal("goDeclarations accepted a file that does not parse")
	}
}

// The point of the feature: a file too large to include verbatim arrives as a
// complete set of declarations instead of its first N bytes, so a reader can
// see every symbol rather than the first few.
func TestOversizedGoFileArrivesAsDeclarations(t *testing.T) {
	root := initSelectionRepo(t)
	var b strings.Builder
	b.WriteString("package big\n\n// Head is the first declaration.\nfunc Head() {\n")
	for b.Len() < 40*1024 {
		b.WriteString("\t_ = \"filler statement inside a function body\"\n")
	}
	b.WriteString("}\n\n// Tail is the last declaration.\nfunc Tail() error { return nil }\n")
	writeFile(t, root, "big.go", b.String())

	limits := DefaultPacketLimits()
	limits.MaxFileBytes = 4096
	selection, err := selectPacketContextWithLimits(stdcontext.Background(), root, "big", nil, limits)
	if err != nil {
		t.Fatalf("selectPacketContext returned error: %v", err)
	}

	var item *PacketItem
	for i := range selection.items {
		if selection.items[i].Path == "big.go" {
			item = &selection.items[i]
		}
	}
	if item == nil {
		t.Fatalf("big.go was not selected: %#v", selection.items)
	}
	if item.Rendering != declarationRendering {
		t.Fatalf("rendering = %q, want %q", item.Rendering, declarationRendering)
	}
	// Byte truncation could never reach Tail; declarations do.
	if !strings.Contains(item.Content, "func Tail() error") {
		t.Fatalf("the last declaration did not survive:\n%s", item.Content)
	}
	if strings.Contains(item.Content, "filler statement") {
		t.Fatal("a function body survived into the packet")
	}
	// The record has to say the content is a summary, not source.
	found := false
	for _, truncation := range selection.truncations {
		if truncation.Path == "big.go" {
			found = true
			if !strings.Contains(truncation.Detail, "declarations") {
				t.Fatalf("truncation record does not disclose the rendering: %q", truncation.Detail)
			}
		}
	}
	if !found {
		t.Fatalf("no truncation recorded for big.go: %#v", selection.truncations)
	}
}

// A file that fits is worth more verbatim than as a summary of itself.
func TestGoFileThatFitsStaysVerbatim(t *testing.T) {
	root := initSelectionRepo(t)
	writeFile(t, root, "small.go", declarationSource)

	selection, err := selectPacketContextWithLimits(stdcontext.Background(), root, "small", nil, DefaultPacketLimits())
	if err != nil {
		t.Fatalf("selectPacketContext returned error: %v", err)
	}
	for _, item := range selection.items {
		if item.Path != "small.go" {
			continue
		}
		if item.Rendering != "" {
			t.Fatalf("a file that fits was summarised: rendering = %q", item.Rendering)
		}
		if !strings.Contains(item.Content, "s := &Server{cfg: cfg}") {
			t.Fatal("verbatim content lost a function body")
		}
		return
	}
	t.Fatal("small.go was not selected")
}

// Rendering declarations reads the whole file, so it also scans the whole file.
// A secret past the per-file prefix must exclude the file rather than reach the
// packet through the skeleton.
func TestDeclarationRenderingScansTheBytesItReads(t *testing.T) {
	root := initSelectionRepo(t)
	var b strings.Builder
	b.WriteString("package big\n\nfunc Head() {\n")
	for b.Len() < 40*1024 {
		b.WriteString("\t_ = \"filler statement inside a function body\"\n")
	}
	b.WriteString("}\n\n// -----BEGIN PRIVATE KEY-----\n// " + leakMarker + "\nfunc Tail() error { return nil }\n")
	writeFile(t, root, "big.go", b.String())

	limits := DefaultPacketLimits()
	limits.MaxFileBytes = 4096
	selection, err := selectPacketContextWithLimits(stdcontext.Background(), root, "big", nil, limits)
	if err != nil {
		t.Fatalf("selectPacketContext returned error: %v", err)
	}
	for _, item := range selection.items {
		if item.Path == "big.go" {
			t.Fatalf("a file with a secret past the prefix reached the packet as declarations")
		}
	}
	assertDecision(t, selection.exclusions, "big.go", "secret_detected")
}

// The skeleton is built from the whole file, so unlike the prefix it is not
// already inside the per-file budget. A caller tightening --max-file-bytes is
// bounding what they will be charged for; a summary that ignores the bound is
// still over the bound.
func TestDeclarationRenderingRespectsThePerFileLimit(t *testing.T) {
	root := initSelectionRepo(t)
	var b strings.Builder
	b.WriteString("package big\n")
	for i := 0; b.Len() < 60*1024; i++ {
		fmt.Fprintf(&b, "\n// Fn%d does a thing worth documenting at some length.\nfunc Fn%d(argument string) error {\n\treturn nil\n}\n", i, i)
	}
	writeFile(t, root, "big.go", b.String())

	for _, maxFileBytes := range []int{1024, 4096, 16384} {
		limits := DefaultPacketLimits()
		limits.MaxFileBytes = maxFileBytes
		selection, err := selectPacketContextWithLimits(stdcontext.Background(), root, "big", nil, limits)
		if err != nil {
			t.Fatalf("selectPacketContext returned error: %v", err)
		}
		for _, item := range selection.items {
			if item.Path != "big.go" {
				continue
			}
			if item.IncludedBytes > maxFileBytes {
				t.Errorf("max_file_bytes=%d produced a %d-byte item (rendering %q)",
					maxFileBytes, item.IncludedBytes, item.Rendering)
			}
		}
	}
}
