// Package mcp implements the FlowCatalyst MCP (Model Context Protocol)
// server. Read-only access for AI clients: list event types, list
// subscriptions, fetch event details.
//
// Phase 5 ships the server scaffold + one example tool (list-event-types).
// Full tool catalogue lands alongside the corresponding subdomain ports.
package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/flowcatalyst/flowcatalyst-go/pkg/fcsdk/client"
)

// Server is the MCP HTTP server.
type Server struct {
	platform *client.FlowCatalystClient
}

// New wires a server pointing at a platform API.
func New(platform *client.FlowCatalystClient) *Server { return &Server{platform: platform} }

// JSONRPCRequest is the inbound MCP request envelope.
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse is the outbound envelope.
type JSONRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *JSONRPCError   `json:"error,omitempty"`
}

// JSONRPCError matches the JSON-RPC 2.0 error shape.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// HandleHTTP serves the streamable-HTTP MCP transport at /mcp.
func (s *Server) HandleHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}
	var req JSONRPCRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, req.ID, -32700, "parse error")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	result, jerr := s.dispatch(ctx, req)
	if jerr != nil {
		writeError(w, req.ID, jerr.Code, jerr.Message)
		return
	}
	writeOK(w, req.ID, result)
}

func (s *Server) dispatch(ctx context.Context, req JSONRPCRequest) (any, *JSONRPCError) {
	switch req.Method {
	case "tools/list":
		return s.toolsList(), nil
	case "tools/call":
		return s.toolsCall(ctx, req.Params)
	default:
		return nil, &JSONRPCError{Code: -32601, Message: "method not found: " + req.Method}
	}
}

func (s *Server) toolsList() any {
	return map[string]any{
		"tools": []map[string]any{
			{
				"name":        "list_event_types",
				"description": "List event types defined in the FlowCatalyst platform",
				"inputSchema": map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				},
			},
		},
	}
}

func (s *Server) toolsCall(ctx context.Context, params json.RawMessage) (any, *JSONRPCError) {
	var p struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, &JSONRPCError{Code: -32602, Message: "invalid params"}
	}
	switch p.Name {
	case "list_event_types":
		var out map[string]any
		if err := s.platform.Get(ctx, "/api/event-types", &out); err != nil {
			return nil, &JSONRPCError{Code: -32000, Message: fmt.Sprintf("platform: %v", err)}
		}
		return map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": fmt.Sprintf("%v", out)},
			},
		}, nil
	default:
		return nil, &JSONRPCError{Code: -32601, Message: "unknown tool: " + p.Name}
	}
}

func writeOK(w http.ResponseWriter, id json.RawMessage, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(JSONRPCResponse{JSONRPC: "2.0", ID: id, Result: result})
}

func writeError(w http.ResponseWriter, id json.RawMessage, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(JSONRPCResponse{
		JSONRPC: "2.0", ID: id,
		Error: &JSONRPCError{Code: code, Message: msg},
	})
}
