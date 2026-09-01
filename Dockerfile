FROM golang:1.24-alpine@sha256:8bee1901f1e530bfb4a7850aa7a479d17ae3a18beb6e09064ed54cfd245b7191 AS builder

WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o loki-mcp-server ./cmd/server

# Use a smaller image for the final stage
FROM alpine:latest@sha256:28bd5fe8b56d1bd048e5babf5b10710ebe0bae67db86916198a6eec434943f8b

WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/loki-mcp-server .

# Expose port for unified MCP server (both SSE and Streamable HTTP)
EXPOSE 8080

# Set the entry point
ENTRYPOINT ["./loki-mcp-server"]
