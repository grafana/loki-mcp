package server

import (
	"github.com/mark3labs/mcp-go/server"

	"github.com/grafana/loki-mcp/internal/logging"
)

// RunStdio starts the MCP server in stdio mode
func RunStdio(version string) error {
	logging.Info("Starting MCP server in stdio mode")

	s := New(version)

	if err := server.ServeStdio(s); err != nil {
		return err
	}

	return nil
}
