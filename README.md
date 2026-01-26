# Loki MCP Server

[![CI](https://github.com/grafana/loki-mcp/workflows/CI/badge.svg)](https://github.com/grafana/loki-mcp/actions/workflows/ci.yml)

A Go-based server implementation for the Model Context Protocol (MCP) with Grafana Loki integration.

## Installation

### Option 1: Go Install

```bash
go install github.com/grafana/loki-mcp/cmd/loki-mcp@latest
```

### Option 2: Download Release Binary

Download the pre-built binary for your platform from the [Releases page](https://github.com/grafana/loki-mcp/releases).

### Option 3: Build from Source

```bash
git clone https://github.com/grafana/loki-mcp.git
cd loki-mcp
go build -o loki-mcp ./cmd/loki-mcp
```

The server communicates using stdin/stdout and SSE following the Model Context Protocol (MCP). This makes it suitable for use with Claude Code, Claude Desktop, and other MCP-compatible clients.

## Project Structure

```
.
├── cmd/
│   └── loki-mcp/         # MCP server implementation
├── internal/
│   └── handlers/         # Tool handlers
├── docker/               # Docker-related files (promtail, grafana, scripts)
└── go.mod                # Go module definition
```

## MCP Server

The Loki MCP Server implements the Model Context Protocol (MCP) and provides the following tools:

### Loki Query Tool

The `loki_query` tool allows you to query Grafana Loki log data:

- Required parameters:
  - `query`: LogQL query string

- Optional parameters:
  - `url`: The Loki server URL (default: from LOKI_URL environment variable or http://localhost:3100)
  - `start`: Start time for the query (default: 1h ago)
  - `end`: End time for the query (default: now)
  - `limit`: Maximum number of entries to return (default: 100)
  - `org`: Organization ID for the query (sent as X-Scope-OrgID header)

### Loki Label Names Tool

The `loki_label_names` tool returns all available label names from Loki.

### Loki Label Values Tool

The `loki_label_values` tool returns all values for a specific label.

#### Environment Variables

The Loki tools support the following environment variables:

- `LOKI_URL`: Default Loki server URL to use if not specified in the request
- `LOKI_ORG_ID`: Default organization ID to use if not specified in the request
- `LOKI_USERNAME`: Default username for basic authentication if not specified in the request
- `LOKI_PASSWORD`: Default password for basic authentication if not specified in the request
- `LOKI_TOKEN`: Default bearer token for authentication if not specified in the request

**Security Note**: When using authentication environment variables, be careful not to expose sensitive credentials in logs or configuration files. Consider using token-based authentication over username/password when possible.

## Docker Support

You can build and run the MCP server using Docker:

```bash
# Build the Docker image
docker build -t loki-mcp .

# Run the server
docker run --rm -i loki-mcp
```

Alternatively, you can use Docker Compose:

```bash
# Build and run with Docker Compose
docker-compose up --build
```

### Local Testing with Loki

The project includes a complete Docker Compose setup to test Loki queries locally:

1. Start the Docker Compose environment:
   ```bash
   docker-compose up -d
   ```

   This will start:
   - A Loki server on port 3100
   - A Grafana instance on port 3000 (pre-configured with Loki as a data source)
   - A log generator container that sends sample logs to Loki
   - The Loki MCP server

2. Insert dummy logs for testing:
   ```bash
   # Insert 10 dummy logs with default settings
   ./docker/insert-loki-logs.sh

   # Insert 20 logs with custom job and app name
   ./docker/insert-loki-logs.sh --num 20 --job "custom-job" --app "my-app"

   # Insert logs with custom environment and interval
   ./docker/insert-loki-logs.sh --env "production" --interval 0.5

   # Show help message
   ./docker/insert-loki-logs.sh --help
   ```

3. Access the Grafana UI at http://localhost:3000 to explore logs visually.

## Server-Sent Events (SSE) Support

The server now supports multiple modes of communication:
1. Standard input/output (stdin/stdout) following the Model Context Protocol (MCP)
2. HTTP Server with Server-Sent Events (SSE) endpoint for integration with tools like n8n
3. Streamable HTTP endpoint for modern MCP clients

The default port for the HTTP server is 8080, but can be configured using the `PORT` environment variable.

### Server Endpoints

When running in HTTP mode, the server exposes the following endpoints:

- SSE Endpoint: `http://localhost:8080/sse` - For real-time event streaming (legacy)
- MCP Endpoint: `http://localhost:8080/mcp` - For MCP protocol messaging (legacy)
- Streamable HTTP: `http://localhost:8080/stream` - For modern MCP clients

### Using Docker with SSE

When running the server with Docker, make sure to expose port 8080:

```bash
# Build the Docker image
docker build -t loki-mcp .

# Run the server with port mapping
docker run -p 8080:8080 --rm -i loki-mcp
```

### n8n Integration

You can integrate the Loki MCP Server with n8n workflows:

1. Install the MCP Client Tools node in n8n

2. Configure the node with these parameters:
   - **SSE Endpoint**: `http://your-server-address:8080/sse` (replace with your actual server address)
   - **Authentication**: Choose appropriate authentication if needed
   - **Tools to Include**: Choose which Loki tools to expose to the AI Agent

3. Connect the MCP Client Tool node to an AI Agent node that will use the Loki querying capabilities

Example workflow:
Trigger → MCP Client Tool (Loki server) → AI Agent (Claude)

## Architecture

The Loki MCP Server uses a modular architecture:

- **Server**: The main MCP server implementation in `cmd/loki-mcp/main.go`
- **Handlers**: Individual tool handlers in `internal/handlers/`
  - `loki.go`: Grafana Loki query functionality

## Using with Claude Code

Add the MCP server to your Claude Code configuration using the CLI:

```bash
claude mcp add loki-mcp -- loki-mcp
```

Or with environment variables:

```bash
claude mcp add loki-mcp -e LOKI_URL=http://localhost:3100 -e LOKI_ORG_ID=your-org-id -- loki-mcp
```

This adds the configuration to your `~/.claude/settings.json`. You can also manually edit the file:

```json
{
  "mcpServers": {
    "loki-mcp": {
      "command": "loki-mcp",
      "args": [],
      "env": {
        "LOKI_URL": "http://localhost:3100",
        "LOKI_ORG_ID": "your-org-id"
      }
    }
  }
}
```

## Using with Claude Desktop

Add the following to your Claude Desktop configuration file:

- macOS: `~/Library/Application Support/Claude/claude_desktop_config.json`
- Windows: `%APPDATA%\Claude\claude_desktop_config.json`
- Linux: `~/.config/Claude/claude_desktop_config.json`

```json
{
  "mcpServers": {
    "loki-mcp": {
      "command": "loki-mcp",
      "args": [],
      "env": {
        "LOKI_URL": "http://localhost:3100",
        "LOKI_ORG_ID": "your-org-id"
      }
    }
  }
}
```

Restart Claude Desktop after updating the configuration.

## Example Queries

Once configured, you can use natural language to query Loki:

- "Query Loki for logs with the query {job=\"varlogs\"}"
- "Find error logs from the last hour using {job=\"varlogs\"} |= \"ERROR\""
- "Show me the most recent 50 logs from job=varlogs"
- "Query Loki for logs with org 'tenant-123' using {job=\"varlogs\"}"

## Using with Cursor

Add the following to your Cursor MCP settings (`.cursor/mcp.json`):

```json
{
  "mcpServers": {
    "loki-mcp": {
      "command": "loki-mcp",
      "args": [],
      "env": {
        "LOKI_URL": "http://localhost:3100",
        "LOKI_ORG_ID": "your-org-id"
      }
    }
  }
}
```

Restart Cursor after adding the configuration.

## License

This project is licensed under the MIT License - see the LICENSE file for details.

## Running Tests

The project includes comprehensive unit tests and CI/CD workflows to ensure reliability:

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out

# Run tests with race detection
go test -race ./...
```
