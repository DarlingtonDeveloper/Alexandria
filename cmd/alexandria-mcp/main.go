// Package main implements the Alexandria MCP stdio server.
// It reads JSON-RPC requests from stdin and writes responses to stdout,
// forwarding tool calls to the Alexandria HTTP API.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/google/uuid"

	"github.com/MikeSquared-Agency/Alexandria/internal/mcpclient"
)

// --- JSON-RPC types ---

type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// --- MCP types ---

type initializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    capabilities `json:"capabilities"`
	ServerInfo      serverInfo   `json:"serverInfo"`
}

type capabilities struct {
	Tools *toolsCap `json:"tools,omitempty"`
}

type toolsCap struct{}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type toolsListResult struct {
	Tools []toolDef `json:"tools"`
}

type toolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type toolCallResult struct {
	Content []contentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// --- Tool definitions ---

var tools = []toolDef{
	{
		Name:        "cg_resolve",
		Description: "Identity resolution: find an existing entity by alias, or create a new one. Use this to record people, projects, agents, and other entities you encounter.",
		InputSchema: mustJSON(`{
			"type": "object",
			"properties": {
				"alias_type":   {"type": "string", "description": "Type of alias (e.g. phone, email, github, mc-task, mc-worker)"},
				"alias_value":  {"type": "string", "description": "The alias value (e.g. +447700900000, mike@example.com)"},
				"entity_type":  {"type": "string", "description": "Type of entity (e.g. person, project, agent, task)"},
				"display_name": {"type": "string", "description": "Human-readable name for the entity"},
				"source":       {"type": "string", "description": "Where this information came from"}
			},
			"required": ["alias_type", "alias_value", "entity_type"]
		}`),
	},
	{
		Name:        "cg_lookup",
		Description: "Get full details for an entity: its properties, all aliases, and all edges (relationships). Use this to understand everything known about a person, project, or other entity.",
		InputSchema: mustJSON(`{
			"type": "object",
			"properties": {
				"entity_id": {"type": "string", "description": "UUID of the entity to look up"}
			},
			"required": ["entity_id"]
		}`),
	},
	{
		Name:        "cg_search",
		Description: "List entities, optionally filtered by type. Returns all non-deleted entities matching the filter.",
		InputSchema: mustJSON(`{
			"type": "object",
			"properties": {
				"entity_type": {"type": "string", "description": "Filter by entity type (e.g. person, project, agent). Omit to list all."}
			}
		}`),
	},
	{
		Name:        "cg_record_edge",
		Description: "Record a relationship between two entities. Examples: person works_on project, agent assigned_to task, decision blocks task.",
		InputSchema: mustJSON(`{
			"type": "object",
			"properties": {
				"from_id": {"type": "string", "description": "UUID of the source entity"},
				"to_id":   {"type": "string", "description": "UUID of the target entity"},
				"type":    {"type": "string", "description": "Relationship type (e.g. works_on, assigned_to, blocks, owns)"},
				"source":  {"type": "string", "description": "Where this information came from"}
			},
			"required": ["from_id", "to_id", "type"]
		}`),
	},
	{
		Name:        "cg_pending",
		Description: "List alias matches that need human review. These are low-confidence identity matches that should be approved or rejected.",
		InputSchema: mustJSON(`{
			"type": "object",
			"properties": {}
		}`),
	},
	{
		Name:        "cg_similar",
		Description: "Find entities semantically similar to a given entity. Returns entities ranked by similarity score. Requires the semantic layer to be enabled.",
		InputSchema: mustJSON(`{
			"type": "object",
			"properties": {
				"entity_id":      {"type": "string", "description": "UUID of the entity to find similar entities for"},
				"limit":          {"type": "integer", "description": "Max results (default 10)"},
				"min_similarity": {"type": "number", "description": "Minimum similarity 0-1 (default 0.7)"}
			},
			"required": ["entity_id"]
		}`),
	},
	{
		Name:        "cg_clusters",
		Description: "List semantic clusters — groups of related entities that have been automatically identified by the semantic layer.",
		InputSchema: mustJSON(`{
			"type": "object",
			"properties": {}
		}`),
	},
	{
		Name:        "cg_semantic_status",
		Description: "Get the status of the semantic layer: entity counts, embedding coverage, active clusters, and pending merge proposals.",
		InputSchema: mustJSON(`{
			"type": "object",
			"properties": {}
		}`),
	},
}

func mustJSON(s string) json.RawMessage {
	var v json.RawMessage
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		panic(fmt.Sprintf("invalid JSON in tool schema: %v", err))
	}
	return v
}

// --- Main ---

func main() {
	baseURL := os.Getenv("ALEXANDRIA_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8500"
	}

	client := mcpclient.New(baseURL)

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var req jsonrpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			writeResponse(jsonrpcResponse{
				JSONRPC: "2.0",
				ID:      nil,
				Error:   &rpcError{Code: -32700, Message: "Parse error"},
			})
			continue
		}

		// Notifications (no ID) — just acknowledge silently
		if req.ID == nil {
			continue
		}

		switch req.Method {
		case "initialize":
			writeResponse(jsonrpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result: initializeResult{
					ProtocolVersion: "2025-11-25",
					Capabilities:    capabilities{Tools: &toolsCap{}},
					ServerInfo:      serverInfo{Name: "alexandria", Version: "0.1.0"},
				},
			})

		case "tools/list":
			writeResponse(jsonrpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  toolsListResult{Tools: tools},
			})

		case "tools/call":
			var params toolCallParams
			if err := json.Unmarshal(req.Params, &params); err != nil {
				writeResponse(jsonrpcResponse{
					JSONRPC: "2.0",
					ID:      req.ID,
					Error:   &rpcError{Code: -32602, Message: "Invalid params"},
				})
				continue
			}
			result := handleToolCall(client, params)
			writeResponse(jsonrpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Result:  result,
			})

		default:
			writeResponse(jsonrpcResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error:   &rpcError{Code: -32601, Message: fmt.Sprintf("Method not found: %s", req.Method)},
			})
		}
	}
}

func handleToolCall(client *mcpclient.Client, params toolCallParams) toolCallResult {
	ctx := context.Background()

	switch params.Name {
	case "cg_resolve":
		return handleResolve(ctx, client, params.Arguments)
	case "cg_lookup":
		return handleLookup(ctx, client, params.Arguments)
	case "cg_search":
		return handleSearch(ctx, client, params.Arguments)
	case "cg_record_edge":
		return handleRecordEdge(ctx, client, params.Arguments)
	case "cg_pending":
		return handlePending(ctx, client)
	case "cg_similar":
		return handleSimilar(ctx, client, params.Arguments)
	case "cg_clusters":
		return handleClusters(ctx, client)
	case "cg_semantic_status":
		return handleSemanticStatus(ctx, client)
	default:
		return errorResult(fmt.Sprintf("Unknown tool: %s", params.Name))
	}
}

func handleResolve(ctx context.Context, client *mcpclient.Client, args json.RawMessage) toolCallResult {
	var req mcpclient.ResolveRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return errorResult("Invalid arguments: " + err.Error())
	}
	result, err := client.Resolve(ctx, req)
	if err != nil {
		return errorResult(err.Error())
	}
	return jsonResult(result)
}

func handleLookup(ctx context.Context, client *mcpclient.Client, args json.RawMessage) toolCallResult {
	var params struct {
		EntityID string `json:"entity_id"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return errorResult("Invalid arguments: " + err.Error())
	}
	id, err := uuid.Parse(params.EntityID)
	if err != nil {
		return errorResult("Invalid UUID: " + err.Error())
	}
	result, err := client.GetEntity(ctx, id)
	if err != nil {
		return errorResult(err.Error())
	}
	return jsonResult(result)
}

func handleSearch(ctx context.Context, client *mcpclient.Client, args json.RawMessage) toolCallResult {
	var params struct {
		EntityType string `json:"entity_type"`
	}
	if args != nil {
		_ = json.Unmarshal(args, &params)
	}
	result, err := client.ListEntities(ctx, params.EntityType)
	if err != nil {
		return errorResult(err.Error())
	}
	return jsonResult(result)
}

func handleRecordEdge(ctx context.Context, client *mcpclient.Client, args json.RawMessage) toolCallResult {
	var params struct {
		FromID string `json:"from_id"`
		ToID   string `json:"to_id"`
		Type   string `json:"type"`
		Source string `json:"source"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return errorResult("Invalid arguments: " + err.Error())
	}
	result, err := client.CreateEdge(ctx, mcpclient.CreateEdgeRequest{
		SourceEntityID:   params.FromID,
		TargetEntityID:   params.ToID,
		RelationshipType: params.Type,
		Strength:         1.0,
		Source:           params.Source,
	})
	if err != nil {
		return errorResult(err.Error())
	}
	return jsonResult(result)
}

func handlePending(ctx context.Context, client *mcpclient.Client) toolCallResult {
	result, err := client.PendingReviews(ctx)
	if err != nil {
		return errorResult(err.Error())
	}
	if len(result) == 0 {
		return textResult("No aliases pending review.")
	}
	return jsonResult(result)
}

func handleSimilar(ctx context.Context, client *mcpclient.Client, args json.RawMessage) toolCallResult {
	var params struct {
		EntityID      string  `json:"entity_id"`
		Limit         int     `json:"limit"`
		MinSimilarity float64 `json:"min_similarity"`
	}
	if err := json.Unmarshal(args, &params); err != nil {
		return errorResult("Invalid arguments: " + err.Error())
	}
	id, err := uuid.Parse(params.EntityID)
	if err != nil {
		return errorResult("Invalid UUID: " + err.Error())
	}
	limit := params.Limit
	if limit <= 0 {
		limit = 10
	}
	minSim := params.MinSimilarity
	if minSim <= 0 {
		minSim = 0.7
	}
	result, err := client.SimilarEntities(ctx, id, limit, minSim)
	if err != nil {
		return errorResult(err.Error())
	}
	if len(result) == 0 {
		return textResult("No similar entities found.")
	}
	return jsonResult(result)
}

func handleClusters(ctx context.Context, client *mcpclient.Client) toolCallResult {
	result, err := client.Clusters(ctx)
	if err != nil {
		return errorResult(err.Error())
	}
	if len(result) == 0 {
		return textResult("No semantic clusters found.")
	}
	return jsonResult(result)
}

func handleSemanticStatus(ctx context.Context, client *mcpclient.Client) toolCallResult {
	result, err := client.SemanticStatusFn(ctx)
	if err != nil {
		return errorResult(err.Error())
	}
	return jsonResult(result)
}

// --- Helpers ---

func jsonResult(v any) toolCallResult {
	data, _ := json.MarshalIndent(v, "", "  ")
	return toolCallResult{
		Content: []contentBlock{{Type: "text", Text: string(data)}},
	}
}

func textResult(text string) toolCallResult {
	return toolCallResult{
		Content: []contentBlock{{Type: "text", Text: text}},
	}
}

func errorResult(msg string) toolCallResult {
	return toolCallResult{
		Content: []contentBlock{{Type: "text", Text: msg}},
		IsError: true,
	}
}

func writeResponse(resp jsonrpcResponse) {
	data, _ := json.Marshal(resp)
	fmt.Fprintf(os.Stdout, "%s\n", data)
	os.Stdout.Sync()
}
