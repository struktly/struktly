package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	repoctx "github.com/struktly/struktly/internal/context"
)

func scannedRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# Demo Repo\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if _, err := repoctx.Scan(repoctx.ScanOptions{Root: root}); err != nil {
		t.Fatalf("scan fixture: %v", err)
	}
	for _, args := range [][]string{
		{"init", "-q"}, {"config", "user.email", "test@example.com"},
		{"config", "user.name", "Test"}, {"config", "commit.gpgsign", "false"},
		{"add", "-A"}, {"commit", "-qm", "fixture"},
	} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
	}
	return root
}

func serveLines(t *testing.T, root string, requests ...string) []string {
	t.Helper()
	var out bytes.Buffer
	input := strings.Join(requests, "\n") + "\n"
	if err := Serve(root, strings.NewReader(input), &out); err != nil {
		t.Fatalf("Serve returned error: %v", err)
	}
	trimmed := strings.TrimSuffix(out.String(), "\n")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

func decodeResponse(t *testing.T, line string) rpcResponse {
	t.Helper()
	var resp rpcResponse
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		t.Fatalf("unmarshal response %q: %v", line, err)
	}
	if resp.JSONRPC != "2.0" {
		t.Fatalf("response missing jsonrpc 2.0: %s", line)
	}
	return resp
}

func decodeToolResult(t *testing.T, line string) toolResult {
	t.Helper()
	resp := decodeResponse(t, line)
	if resp.Error != nil {
		t.Fatalf("tools/call returned protocol error: %+v", resp.Error)
	}
	var result toolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal tool result: %v", err)
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Fatalf("expected one text content item, got: %+v", result.Content)
	}
	return result
}

func TestInitialize(t *testing.T) {
	lines := serveLines(t, t.TempDir(),
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test-client","version":"0.0.1"}}}`)
	if len(lines) != 1 {
		t.Fatalf("expected 1 response line, got %d: %v", len(lines), lines)
	}
	resp := decodeResponse(t, lines[0])
	if resp.Error != nil {
		t.Fatalf("initialize returned error: %+v", resp.Error)
	}
	var result struct {
		ProtocolVersion string                     `json:"protocolVersion"`
		Capabilities    map[string]json.RawMessage `json:"capabilities"`
		ServerInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal initialize result: %v", err)
	}
	if result.ProtocolVersion != "2025-06-18" {
		t.Fatalf("unexpected protocolVersion: %q", result.ProtocolVersion)
	}
	if result.ServerInfo.Name != "struktly" || result.ServerInfo.Version == "" {
		t.Fatalf("unexpected serverInfo: %+v", result.ServerInfo)
	}
	if _, ok := result.Capabilities["tools"]; !ok {
		t.Fatalf("capabilities should declare tools: %v", result.Capabilities)
	}
}

func TestPing(t *testing.T) {
	lines := serveLines(t, t.TempDir(), `{"jsonrpc":"2.0","id":7,"method":"ping"}`)
	if len(lines) != 1 {
		t.Fatalf("expected 1 response line, got %d: %v", len(lines), lines)
	}
	resp := decodeResponse(t, lines[0])
	if resp.Error != nil {
		t.Fatalf("ping returned error: %+v", resp.Error)
	}
	if string(resp.Result) != "{}" {
		t.Fatalf("ping should return an empty result object, got: %s", resp.Result)
	}
}

func TestToolsList(t *testing.T) {
	lines := serveLines(t, t.TempDir(), `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if len(lines) != 1 {
		t.Fatalf("expected 1 response line, got %d: %v", len(lines), lines)
	}
	resp := decodeResponse(t, lines[0])
	if resp.Error != nil {
		t.Fatalf("tools/list returned error: %+v", resp.Error)
	}
	var result struct {
		Tools []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			InputSchema struct {
				Type       string                     `json:"type"`
				Properties map[string]json.RawMessage `json:"properties"`
				Required   []string                   `json:"required"`
			} `json:"inputSchema"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal tools/list result: %v", err)
	}
	if len(result.Tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(result.Tools))
	}

	wantRequired := map[string][]string{
		"context_scan":    nil,
		"context_brief":   {"task"},
		"evidence_record": {"task", "agent", "outcome"},
	}
	for _, tool := range result.Tools {
		required, ok := wantRequired[tool.Name]
		if !ok {
			t.Fatalf("unexpected tool: %s", tool.Name)
		}
		delete(wantRequired, tool.Name)
		if tool.Description == "" {
			t.Fatalf("tool %s has no description", tool.Name)
		}
		if tool.InputSchema.Type != "object" {
			t.Fatalf("tool %s inputSchema type = %q, want object", tool.Name, tool.InputSchema.Type)
		}
		if _, ok := tool.InputSchema.Properties["root"]; !ok {
			t.Fatalf("tool %s should accept a root property", tool.Name)
		}
		if len(tool.InputSchema.Required) != len(required) {
			t.Fatalf("tool %s required = %v, want %v", tool.Name, tool.InputSchema.Required, required)
		}
		for i, name := range required {
			if tool.InputSchema.Required[i] != name {
				t.Fatalf("tool %s required = %v, want %v", tool.Name, tool.InputSchema.Required, required)
			}
		}
	}
	if len(wantRequired) != 0 {
		t.Fatalf("missing tools: %v", wantRequired)
	}
}

func TestToolsCallContextBrief(t *testing.T) {
	root := scannedRepo(t)
	lines := serveLines(t, root,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"context_brief","arguments":{"task":"Add MCP server"}}}`)
	if len(lines) != 1 {
		t.Fatalf("expected 1 response line, got %d: %v", len(lines), lines)
	}
	result := decodeToolResult(t, lines[0])
	if result.IsError {
		t.Fatalf("context_brief returned isError: %s", result.Content[0].Text)
	}
	text := result.Content[0].Text
	firstLine, rest, _ := strings.Cut(text, "\n")
	if !strings.Contains(firstLine, filepath.Join(".struktly", "context-packets")) || !strings.HasSuffix(firstLine, ".md") {
		t.Fatalf("first line should be the packet path, got: %q", firstLine)
	}
	if !strings.Contains(rest, "# Struktly Context Packet") {
		t.Fatalf("tool result should contain the packet markdown, got:\n%s", text)
	}
	structured, ok := result.StructuredContent.(map[string]any)
	if !ok || structured["schema"] != repoctx.PacketSchema || structured["packet_hash"] == "" {
		t.Fatalf("tool result should carry structured packet content, got: %#v", result.StructuredContent)
	}
	if _, err := os.Stat(firstLine); err != nil {
		t.Fatalf("packet path from first line should exist: %v", err)
	}
}

func TestToolsCallScanAndEvidence(t *testing.T) {
	serverRoot := t.TempDir()
	otherRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(otherRoot, "README.md"), []byte("# Other Repo\n"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	scanArgs, err := json.Marshal(map[string]string{"root": otherRoot})
	if err != nil {
		t.Fatalf("marshal scan arguments: %v", err)
	}
	lines := serveLines(t, serverRoot,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"context_scan","arguments":`+string(scanArgs)+`}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"evidence_record","arguments":{"task":"Add MCP server","agent":"claude","outcome":"done","checks":["go test ./..."],"result":"pass"}}}`)
	if len(lines) != 2 {
		t.Fatalf("expected 2 response lines, got %d: %v", len(lines), lines)
	}

	scanResult := decodeToolResult(t, lines[0])
	if scanResult.IsError {
		t.Fatalf("context_scan returned isError: %s", scanResult.Content[0].Text)
	}
	wantScanPath := filepath.Join(otherRoot, ".struktly", "project-context.md")
	if !strings.Contains(scanResult.Content[0].Text, wantScanPath) {
		t.Fatalf("scan confirmation should name %s, got: %s", wantScanPath, scanResult.Content[0].Text)
	}
	if _, err := os.Stat(wantScanPath); err != nil {
		t.Fatalf("scan should write project context in the root argument: %v", err)
	}

	evidenceResult := decodeToolResult(t, lines[1])
	if evidenceResult.IsError {
		t.Fatalf("evidence_record returned isError: %s", evidenceResult.Content[0].Text)
	}
	wantLedgerPath := filepath.Join(serverRoot, ".struktly", "evidence.md")
	if !strings.Contains(evidenceResult.Content[0].Text, wantLedgerPath) {
		t.Fatalf("evidence confirmation should name %s, got: %s", wantLedgerPath, evidenceResult.Content[0].Text)
	}
	ledger, err := os.ReadFile(wantLedgerPath)
	if err != nil {
		t.Fatalf("read evidence ledger: %v", err)
	}
	if !strings.Contains(string(ledger), "go test ./...") {
		t.Fatalf("evidence ledger should record the check, got:\n%s", ledger)
	}
}

func TestToolsCallMissingRequiredArg(t *testing.T) {
	root := scannedRepo(t)
	lines := serveLines(t, root,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"context_brief","arguments":{}}}`)
	if len(lines) != 1 {
		t.Fatalf("expected 1 response line, got %d: %v", len(lines), lines)
	}
	result := decodeToolResult(t, lines[0])
	if !result.IsError {
		t.Fatalf("missing task should return isError, got: %+v", result)
	}
	if !strings.Contains(result.Content[0].Text, "task") {
		t.Fatalf("error message should mention task, got: %q", result.Content[0].Text)
	}
}

func TestUnknownMethod(t *testing.T) {
	lines := serveLines(t, t.TempDir(), `{"jsonrpc":"2.0","id":8,"method":"bogus/method"}`)
	if len(lines) != 1 {
		t.Fatalf("expected 1 response line, got %d: %v", len(lines), lines)
	}
	resp := decodeResponse(t, lines[0])
	if resp.Error == nil || resp.Error.Code != -32601 {
		t.Fatalf("unknown method should return -32601, got: %s", lines[0])
	}
}

func TestNotificationProducesNoResponse(t *testing.T) {
	lines := serveLines(t, t.TempDir(),
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":1}}`)
	if len(lines) != 0 {
		t.Fatalf("notifications should produce no response lines, got: %v", lines)
	}
}
