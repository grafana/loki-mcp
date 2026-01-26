package server

import (
	"github.com/mark3labs/mcp-go/server"

	"github.com/grafana/loki-mcp/internal/handlers"
)

// New creates a new MCP server with all Loki tools registered
func New(version string) *server.MCPServer {
	s := server.NewMCPServer(
		"Loki MCP Server",
		version,
		server.WithResourceCapabilities(true, true),
		server.WithLogging(),
	)

	// Add Loki query tool
	lokiQueryTool := handlers.NewLokiQueryTool()
	s.AddTool(lokiQueryTool, handlers.HandleLokiQuery)

	// Add Loki label names tool
	lokiLabelNamesTool := handlers.NewLokiLabelNamesTool()
	s.AddTool(lokiLabelNamesTool, handlers.HandleLokiLabelNames)

	// Add Loki label values tool
	lokiLabelValuesTool := handlers.NewLokiLabelValuesTool()
	s.AddTool(lokiLabelValuesTool, handlers.HandleLokiLabelValues)

	return s
}
