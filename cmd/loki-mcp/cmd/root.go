package cmd

import (
	"os"

	"github.com/spf13/cobra"

	"github.com/grafana/loki-mcp/internal/handlers"
	"github.com/grafana/loki-mcp/internal/logging"
)

var (
	// Version is set via ldflags at build time
	Version = "dev"

	// Global flags
	queryFilter string
	logLevel    string
	logFormat   string
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "loki-mcp",
	Short: "MCP server for Grafana Loki",
	Long: `An MCP (Model Context Protocol) server that provides Grafana Loki query tools
to AI assistants like Claude Desktop, Cursor, and n8n.

Use subcommands to run the server in different modes:
  - stdio: For Claude Desktop and similar MCP clients
  - http:  For web-based clients and n8n`,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Initialize logging
		if err := logging.Init(logLevel, logFormat); err != nil {
			return err
		}

		// Initialize and validate the query filter
		if queryFilter != "" {
			if err := handlers.InitializeFilterConfig(queryFilter); err != nil {
				return err
			}
			logging.Infof("Query filter configured: %s", queryFilter)
		}

		return nil
	},
}

// Execute adds all child commands to the root command and sets flags appropriately.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	// Query filter flag
	rootCmd.PersistentFlags().StringVarP(&queryFilter, "query-filter", "f", "",
		"LogQL stream selector to restrict all queries (e.g., {namespace=\"prod\"}). "+
			"Can also be set via LOKI_QUERY_FILTER env var.")

	// Logging flags
	rootCmd.PersistentFlags().StringVarP(&logLevel, "log-level", "l", "info",
		"Log level: debug, info, warn, error")
	rootCmd.PersistentFlags().StringVar(&logFormat, "log-format", "text",
		"Log format: text, json")

	// Bind environment variable for query filter
	if envFilter := os.Getenv("LOKI_QUERY_FILTER"); envFilter != "" && queryFilter == "" {
		queryFilter = envFilter
	}
}
