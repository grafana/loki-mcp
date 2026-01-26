package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/grafana/loki-mcp/internal/logging"
	"github.com/grafana/loki-mcp/internal/server"
)

var (
	httpPort       string
	httpHost       string
	httpDisableSSE bool
)

// httpCmd represents the http command
var httpCmd = &cobra.Command{
	Use:   "http",
	Short: "Run the MCP server in HTTP mode",
	Long: `Run the MCP server in HTTP mode with streamable HTTP endpoint.

Endpoints:
  /stream - Streamable HTTP endpoint (modern clients)
  /sse    - SSE event stream (legacy, can be disabled)
  /mcp    - SSE message endpoint (legacy, can be disabled)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg := server.HTTPConfig{
			Host:       httpHost,
			Port:       httpPort,
			DisableSSE: httpDisableSSE,
		}

		if err := server.RunHTTP(Version, cfg); err != nil {
			logging.Errorf("HTTP server error: %v", err)
			return err
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(httpCmd)

	// Get default port from environment or use 8080
	defaultPort := os.Getenv("PORT")
	if defaultPort == "" {
		defaultPort = "8080"
	}

	httpCmd.Flags().StringVarP(&httpPort, "port", "p", defaultPort,
		"HTTP server port. Can also be set via PORT env var.")
	httpCmd.Flags().StringVar(&httpHost, "host", "0.0.0.0",
		"HTTP server bind address")
	httpCmd.Flags().BoolVar(&httpDisableSSE, "disable-sse", false,
		"Disable legacy SSE endpoints (/sse, /mcp)")
}
