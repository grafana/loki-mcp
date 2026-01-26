package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mark3labs/mcp-go/server"

	"github.com/grafana/loki-mcp/internal/logging"
)

// HTTPConfig holds configuration for the HTTP server
type HTTPConfig struct {
	Host       string
	Port       string
	DisableSSE bool
}

// DefaultHTTPConfig returns the default HTTP configuration
func DefaultHTTPConfig() HTTPConfig {
	return HTTPConfig{
		Host:       "0.0.0.0",
		Port:       "8080",
		DisableSSE: false,
	}
}

const defaultShutdownTimeout = 5 * time.Second

// RunHTTP starts the MCP server in HTTP mode
func RunHTTP(version string, cfg HTTPConfig) error {
	s := New(version)

	mux := http.NewServeMux()

	// Register legacy SSE endpoints unless disabled
	if !cfg.DisableSSE {
		sseServer := server.NewSSEServer(s,
			server.WithSSEEndpoint("/sse"),
			server.WithMessageEndpoint("/mcp"),
		)
		mux.Handle("/sse", sseServer)
		mux.Handle("/mcp", sseServer)
	}

	// Register Streamable HTTP endpoint
	streamableServer := server.NewStreamableHTTPServer(s)
	mux.Handle("/stream", streamableServer)

	addr := fmt.Sprintf("%s:%s", cfg.Host, cfg.Port)
	httpServer := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a channel to handle shutdown signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Start HTTP server in goroutine
	errChan := make(chan error, 1)
	go func() {
		logging.Infof("Starting MCP HTTP server on http://%s", addr)
		if !cfg.DisableSSE {
			logging.Infof("SSE Endpoint (legacy): http://%s/sse", addr)
			logging.Infof("SSE Message Endpoint: http://%s/mcp", addr)
		}
		logging.Infof("Streamable HTTP Endpoint: http://%s/stream", addr)

		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errChan <- err
		}
	}()

	// Wait for interrupt signal or error
	select {
	case <-stop:
		logging.Info("Shutting down HTTP server...")
	case err := <-errChan:
		return fmt.Errorf("HTTP server error: %w", err)
	}

	// Cancel context to signal shutdown
	cancel()

	// Gracefully shutdown HTTP server
	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, defaultShutdownTimeout)
	defer shutdownCancel()

	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		logging.Errorf("HTTP server shutdown error: %v", err)
		return err
	}

	logging.Info("HTTP server stopped gracefully")
	return nil
}
