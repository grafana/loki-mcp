package cmd

import (
	"github.com/spf13/cobra"

	"github.com/grafana/loki-mcp/internal/logging"
	"github.com/grafana/loki-mcp/internal/server"
)

// stdioCmd represents the stdio command
var stdioCmd = &cobra.Command{
	Use:   "stdio",
	Short: "Run the MCP server in stdio mode",
	Long: `Run the MCP server in stdio mode for integration with Claude Desktop
and other MCP clients that communicate via stdin/stdout.

All log output is sent to stderr to avoid interfering with the MCP protocol.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := server.RunStdio(Version); err != nil {
			logging.Errorf("Stdio server error: %v", err)
			return err
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(stdioCmd)
}
