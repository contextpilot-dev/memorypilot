package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/contextpilot-dev/memorypilot/internal/embedding"
	"github.com/contextpilot-dev/memorypilot/internal/store"
	"github.com/contextpilot-dev/memorypilot/pkg/models"
	"github.com/oklog/ulid/v2"
)

// Server implements the MCP protocol over stdio
type Server struct {
	store  *store.Store
	reader *bufio.Reader
	writer io.Writer
}

// NewServer creates a new MCP server
func NewServer(dbPath string) (*Server, error) {
	s, err := store.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open store: %w", err)
	}

	return &Server{
		store:  s,
		reader: bufio.NewReader(os.Stdin),
		writer: os.Stdout,
	}, nil
}

// Run starts the MCP server (blocks until stdin closes)
func (s *Server) Run() error {
	log.SetOutput(os.Stderr) // Log to stderr, not stdout

	// Send server info
	s.sendServerInfo()

	// Main loop - read JSON-RPC messages from stdin
	for {
		line, err := s.reader.ReadString('\n')
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read error: %w", err)
		}

		// Parse JSON-RPC request
		var req JSONRPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			s.sendError(nil, -32700, "Parse error")
			continue
		}

		// Handle request
		s.handleRequest(&req)
	}
}

type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (s *Server) sendServerInfo() {
	info := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"serverInfo": map[string]string{
			"name":    "memorypilot",
			"version": "0.1.0",
		},
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
	}
	s.sendResult(nil, info)
}

func (s *Server) handleRequest(req *JSONRPCRequest) {
	switch req.Method {
	case "initialize":
		s.handleInitialize(req)
	case "tools/list":
		s.handleToolsList(req)
	case "tools/call":
		s.handleToolsCall(req)
	default:
		s.sendError(req.ID, -32601, "Method not found")
	}
}

func (s *Server) handleInitialize(req *JSONRPCRequest) {
	result := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"serverInfo": map[string]string{
			"name":    "memorypilot",
			"version": "0.1.0",
		},
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
	}
	s.sendResult(req.ID, result)
}

func (s *Server) handleToolsList(req *JSONRPCRequest) {
	tools := []map[string]interface{}{
		{
			"name":        "memorypilot_recall",
			"description": "Search your memory for relevant context",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "What to search for",
					},
					"limit": map[string]interface{}{
						"type":        "number",
						"description": "Maximum results",
						"default":     5,
					},
				},
				"required": []string{"query"},
			},
		},
		{
			"name":        "memorypilot_remember",
			"description": "Explicitly remember something important",
			"inputSchema": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"content": map[string]interface{}{
						"type":        "string",
						"description": "What to remember",
					},
					"type": map[string]interface{}{
						"type":        "string",
						"description": "Memory type",
						"enum":        []string{"decision", "pattern", "fact", "preference", "mistake", "learning"},
						"default":     "fact",
					},
					"topics": map[string]interface{}{
						"type":        "array",
						"description": "Topics/tags for this memory",
						"items":       map[string]interface{}{"type": "string"},
					},
				},
				"required": []string{"content"},
			},
		},
		{
			"name":        "memorypilot_status",
			"description": "Get memory statistics",
			"inputSchema": map[string]interface{}{
				"type":       "object",
				"properties": map[string]interface{}{},
			},
		},
	}

	s.sendResult(req.ID, map[string]interface{}{"tools": tools})
}

func (s *Server) handleToolsCall(req *JSONRPCRequest) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}

	if err := json.Unmarshal(req.Params, &params); err != nil {
		s.sendError(req.ID, -32602, "Invalid params")
		return
	}

	switch params.Name {
	case "memorypilot_recall":
		s.handleRecall(req, params.Arguments)
	case "memorypilot_remember":
		s.handleRemember(req, params.Arguments)
	case "memorypilot_status":
		s.handleStatus(req)
	default:
		s.sendError(req.ID, -32602, "Unknown tool")
	}
}

func (s *Server) handleRecall(req *JSONRPCRequest, args json.RawMessage) {
	var params struct {
		Query    string `json:"query"`
		Limit    int    `json:"limit"`
		Semantic bool   `json:"semantic"`
	}
	json.Unmarshal(args, &params)

	if params.Limit == 0 {
		params.Limit = 5
	}

	var memories []models.Memory
	var err error

	// Try semantic search first (hybrid: semantic + keyword)
	embedder := embedding.NewOllamaEmbedder("", "nomic-embed-text")
	if queryEmb, embErr := embedder.Embed(params.Query); embErr == nil && queryEmb != nil {
		memories, err = s.store.HybridSearch(params.Query, queryEmb, params.Limit)
	} else {
		// Fall back to keyword search
		memories, err = s.store.Recall(models.RecallRequest{
			Query: params.Query,
			Limit: params.Limit,
		})
	}

	if err != nil {
		s.sendError(req.ID, -32000, err.Error())
		return
	}

	// Format as text
	var text string
	if len(memories) == 0 {
		text = fmt.Sprintf("No memories found for: %q", params.Query)
	} else {
		text = fmt.Sprintf("Found %d memories:\n\n", len(memories))
		for i, m := range memories {
			topicsStr := ""
			if len(m.Topics) > 0 {
				topicsStr = fmt.Sprintf("\n   Topics: %v", m.Topics)
			}
			text += fmt.Sprintf("%d. [%s] %s\n   %s%s\n\n",
				i+1, m.Type, m.Summary, m.Content, topicsStr)
		}
	}

	s.sendResult(req.ID, map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": text},
		},
	})
}

func (s *Server) handleRemember(req *JSONRPCRequest, args json.RawMessage) {
	var params struct {
		Content string   `json:"content"`
		Type    string   `json:"type"`
		Topics  []string `json:"topics"`
	}
	json.Unmarshal(args, &params)

	if params.Type == "" {
		params.Type = "fact"
	}

	// Create memory
	now := time.Now()
	memory := models.Memory{
		ID:      ulid.Make().String(),
		Type:    models.MemoryType(params.Type),
		Content: params.Content,
		Summary: truncateStr(params.Content, 100),
		Scope:   models.MemoryScopePersonal,
		Source: models.Source{
			Type:      models.SourceTypeManual,
			Reference: "mcp",
			Timestamp: now,
		},
		Confidence:     1.0,
		Importance:     1.0,
		Topics:         params.Topics,
		CreatedAt:      now,
		LastAccessedAt: now,
		AccessCount:    0,
	}

	// Save memory
	if err := s.store.CreateMemory(&memory); err != nil {
		s.sendError(req.ID, -32000, fmt.Sprintf("Failed to save memory: %v", err))
		return
	}

	// Generate embedding (best effort)
	embedder := embedding.NewOllamaEmbedder("", "nomic-embed-text")
	if emb, err := embedder.Embed(memory.Content); err == nil && emb != nil {
		s.store.UpdateMemoryEmbedding(memory.ID, emb)
	}

	text := fmt.Sprintf("✅ Remembered: %s\n   Type: %s\n   ID: %s", params.Content, params.Type, memory.ID)

	s.sendResult(req.ID, map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": text},
		},
	})
}

func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func (s *Server) handleStatus(req *JSONRPCRequest) {
	stats, err := s.store.GetStats()
	if err != nil {
		s.sendError(req.ID, -32000, err.Error())
		return
	}

	text := fmt.Sprintf("MemoryPilot Status\n\nTotal memories: %d\nProjects: %d\n\nBy type:\n",
		stats.TotalMemories, stats.ProjectCount)
	for t, count := range stats.ByType {
		text += fmt.Sprintf("  %s: %d\n", t, count)
	}

	s.sendResult(req.ID, map[string]interface{}{
		"content": []map[string]interface{}{
			{"type": "text", "text": text},
		},
	})
}

func (s *Server) sendResult(id interface{}, result interface{}) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	s.send(resp)
}

func (s *Server) sendError(id interface{}, code int, message string) {
	resp := JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &RPCError{Code: code, Message: message},
	}
	s.send(resp)
}

func (s *Server) send(resp JSONRPCResponse) {
	data, _ := json.Marshal(resp)
	fmt.Fprintf(s.writer, "%s\n", data)
}
