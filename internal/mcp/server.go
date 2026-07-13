// Package mcp serves struktly over the Model Context Protocol stdio transport:
// newline-delimited JSON-RPC 2.0, one message per line.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/struktly/struktly/internal/buildinfo"
	repoctx "github.com/struktly/struktly/internal/context"
	"github.com/struktly/struktly/internal/evidence"
)

const (
	protocolVersion = "2025-06-18"
	serverName      = "struktly"
)

// Serve reads newline-delimited JSON-RPC messages from in and writes responses
// to out until in reaches EOF. root is the default repository root for tool
// calls that do not pass their own.
func Serve(root string, in io.Reader, out io.Writer) error {
	scanner := bufio.NewScanner(in)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	w := bufio.NewWriter(out)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		resp := handleMessage(root, []byte(line))
		if resp == nil {
			continue
		}
		data, err := json.Marshal(resp)
		if err != nil {
			return fmt.Errorf("marshal response: %w", err)
		}
		data = append(data, '\n')
		if _, err := w.Write(data); err != nil {
			return err
		}
		if err := w.Flush(); err != nil {
			return err
		}
	}
	return scanner.Err()
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func okResponse(id json.RawMessage, result any) *response {
	return &response{JSONRPC: "2.0", ID: id, Result: result}
}

func errResponse(id json.RawMessage, code int, message string) *response {
	return &response{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message}}
}

// handleMessage dispatches one JSON-RPC message and returns the response to
// write, or nil for notifications (which never get a response).
func handleMessage(root string, line []byte) *response {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return errResponse(nil, -32700, "parse error: "+err.Error())
	}
	if len(req.ID) == 0 || string(req.ID) == "null" {
		return nil
	}
	switch req.Method {
	case "initialize":
		return okResponse(req.ID, map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": serverName, "version": buildinfo.Current().Version},
		})
	case "ping":
		return okResponse(req.ID, struct{}{})
	case "tools/list":
		return okResponse(req.ID, map[string]any{"tools": toolDefs()})
	case "tools/call":
		result, rpcErr := callTool(root, req.Params)
		if rpcErr != nil {
			return &response{JSONRPC: "2.0", ID: req.ID, Error: rpcErr}
		}
		return okResponse(req.ID, result)
	default:
		return errResponse(req.ID, -32601, "method not found: "+req.Method)
	}
}

type toolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

func toolDefs() []toolDef {
	rootProp := map[string]any{
		"type":        "string",
		"description": "Repository root (defaults to the root the server was started with)",
	}
	return []toolDef{
		{
			Name:        "context_scan",
			Description: "Scan the repository and write .struktly/project-context.md",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"root": rootProp},
			},
		},
		{
			Name:        "context_brief",
			Description: "Write a task-specific context packet and return Markdown plus structured packet JSON",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root": rootProp,
					"task": map[string]any{"type": "string", "description": "Task the context packet is for"},
				},
				"required": []string{"task"},
			},
		},
		{
			Name:        "evidence_record",
			Description: "Append a structured evidence entry to .struktly/evidence.md",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"root":    rootProp,
					"task":    map[string]any{"type": "string", "description": "Task or work summary"},
					"agent":   map[string]any{"type": "string", "description": "Agent or tool name"},
					"outcome": map[string]any{"type": "string", "description": "Outcome summary"},
					"checks": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Verification commands that were run",
					},
					"result":        map[string]any{"type": "string", "description": "Result summary for checks run"},
					"contextPacket": map[string]any{"type": "string", "description": "Path to the context packet used for the work"},
				},
				"required": []string{"task", "agent", "outcome"},
			},
		},
	}
}

type toolArgs struct {
	Root          string   `json:"root"`
	Task          string   `json:"task"`
	Agent         string   `json:"agent"`
	Outcome       string   `json:"outcome"`
	Checks        []string `json:"checks"`
	Result        string   `json:"result"`
	ContextPacket string   `json:"contextPacket"`
}

type toolResult struct {
	Content           []toolContent `json:"content"`
	StructuredContent any           `json:"structuredContent,omitempty"`
	IsError           bool          `json:"isError,omitempty"`
}

type toolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func textResult(text string) toolResult {
	return toolResult{Content: []toolContent{{Type: "text", Text: text}}}
}

func errorResult(message string) toolResult {
	return toolResult{Content: []toolContent{{Type: "text", Text: message}}, IsError: true}
}

// callTool runs one tool. Malformed params and unknown tool names are protocol
// errors; tool argument and execution failures come back as isError results.
func callTool(root string, params json.RawMessage) (toolResult, *rpcError) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return toolResult{}, &rpcError{Code: -32602, Message: "invalid params: " + err.Error()}
	}
	var args toolArgs
	if len(call.Arguments) > 0 {
		if err := json.Unmarshal(call.Arguments, &args); err != nil {
			return errorResult("invalid arguments: " + err.Error()), nil
		}
	}
	if strings.TrimSpace(args.Root) == "" {
		args.Root = root
	}
	switch call.Name {
	case "context_scan":
		return runScan(args), nil
	case "context_brief":
		return runBrief(args), nil
	case "evidence_record":
		return runEvidence(args), nil
	default:
		return toolResult{}, &rpcError{Code: -32602, Message: "unknown tool: " + call.Name}
	}
}

func runScan(args toolArgs) toolResult {
	result, err := repoctx.Scan(repoctx.ScanOptions{Root: args.Root})
	if err != nil {
		return errorResult(err.Error())
	}
	return textResult("wrote " + result.OutputPath)
}

func runBrief(args toolArgs) toolResult {
	result, err := repoctx.Brief(repoctx.BriefOptions{Root: args.Root, Task: args.Task})
	if err != nil {
		return errorResult(err.Error())
	}
	data, err := os.ReadFile(result.OutputPath)
	if err != nil {
		return errorResult("read context packet: " + err.Error())
	}
	toolResult := textResult(result.OutputPath + "\n\n" + string(data))
	toolResult.StructuredContent = result.Packet
	return toolResult
}

func runEvidence(args toolArgs) toolResult {
	result, err := evidence.RecordEvidence(evidence.EvidenceOptions{
		Root:          args.Root,
		Task:          args.Task,
		Agent:         args.Agent,
		Outcome:       args.Outcome,
		Checks:        args.Checks,
		CheckResult:   args.Result,
		ContextPacket: args.ContextPacket,
	})
	if err != nil {
		return errorResult(err.Error())
	}
	return textResult("appended to " + result.OutputPath)
}
