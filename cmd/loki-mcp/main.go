package main

import (
	"github.com/grafana/loki-mcp/cmd/loki-mcp/cmd"
)

// version is set via -ldflags at build time
var version = "dev"

func main() {
	cmd.Version = version
	cmd.Execute()
}
