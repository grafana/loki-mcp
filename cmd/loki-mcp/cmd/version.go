package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number",
	Long:  `Print the version number of loki-mcp.`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("loki-mcp", Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
