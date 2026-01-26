package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/grafana/loki/v3/pkg/logcli/client"
	"github.com/grafana/loki/v3/pkg/loghttp"
	"github.com/grafana/loki/v3/pkg/logproto"
	"github.com/mark3labs/mcp-go/mcp"
)

// SSEEvent represents an event to be sent via SSE
type SSEEvent struct {
	Type      string `json:"type"`
	Query     string `json:"query"`
	Timestamp string `json:"timestamp"`
	Results   any    `json:"results"`
}

// Environment variable names
const (
	EnvLokiURL      = "LOKI_URL"
	EnvLokiOrgID    = "LOKI_ORG_ID"
	EnvLokiUsername = "LOKI_USERNAME"
	EnvLokiPassword = "LOKI_PASSWORD"
	EnvLokiToken    = "LOKI_TOKEN"
)

// Default values
const (
	DefaultLokiURL     = "http://localhost:3100"
	DefaultHTTPTimeout = 30 * time.Second
	DefaultQueryLimit  = 100
	DefaultLookback    = 1 * time.Hour
)

// LokiParams contains common parameters for Loki tool requests
type LokiParams struct {
	URL      string  `json:"url"`
	Username string  `json:"username"`
	Password string  `json:"password"`
	Token    string  `json:"token"`
	Org      string  `json:"org"`
	Start    string  `json:"start"`
	End      string  `json:"end"`
	Format   string  `json:"format"`
	Limit    float64 `json:"limit"` // JSON numbers deserialize as float64
	Query    string  `json:"query"` // for loki_query
	Label    string  `json:"label"` // for loki_label_values
}

// applyDefaults fills in missing values from environment variables
func (p *LokiParams) applyDefaults() {
	if p.URL == "" {
		p.URL = os.Getenv(EnvLokiURL)
		if p.URL == "" {
			p.URL = DefaultLokiURL
		}
	}
	if p.Username == "" {
		p.Username = os.Getenv(EnvLokiUsername)
	}
	if p.Password == "" {
		p.Password = os.Getenv(EnvLokiPassword)
	}
	if p.Token == "" {
		p.Token = os.Getenv(EnvLokiToken)
	}
	if p.Org == "" {
		p.Org = os.Getenv(EnvLokiOrgID)
	}
	if p.Format == "" {
		p.Format = "raw"
	}
}

// newLokiClient creates a Loki client from the given parameters
func newLokiClient(params *LokiParams) *client.DefaultClient {
	return &client.DefaultClient{
		Address:     params.URL,
		Username:    params.Username,
		Password:    params.Password,
		BearerToken: params.Token,
		OrgID:       params.Org,
	}
}

// NewLokiQueryTool creates and returns a tool for querying Grafana Loki
func NewLokiQueryTool() mcp.Tool {
	// Get Loki URL from environment variable or use default
	lokiURL := os.Getenv(EnvLokiURL)
	if lokiURL == "" {
		lokiURL = DefaultLokiURL
	}

	// Get Loki Org ID from environment variable if set
	orgID := os.Getenv(EnvLokiOrgID)

	// Get authentication parameters from environment variables if set
	username := os.Getenv(EnvLokiUsername)
	password := os.Getenv(EnvLokiPassword)
	token := os.Getenv(EnvLokiToken)

	return mcp.NewTool("loki_query",
		mcp.WithDescription("Run a query against Grafana Loki"),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("LogQL query string"),
		),
		mcp.WithString("url",
			mcp.Description(fmt.Sprintf("Loki server URL (default: %s from %s env var)", lokiURL, EnvLokiURL)),
			mcp.DefaultString(lokiURL),
		),
		mcp.WithString("username",
			mcp.Description(fmt.Sprintf("Username for basic authentication (default: %s from %s env var)", username, EnvLokiUsername)),
		),
		mcp.WithString("password",
			mcp.Description(fmt.Sprintf("Password for basic authentication (default: %s from %s env var)", password, EnvLokiPassword)),
		),
		mcp.WithString("token",
			mcp.Description(fmt.Sprintf("Bearer token for authentication (default: %s from %s env var)", token, EnvLokiToken)),
		),
		mcp.WithString("start",
			mcp.Description("Start time for the query (default: 1h ago)"),
		),
		mcp.WithString("end",
			mcp.Description("End time for the query (default: now)"),
		),
		mcp.WithNumber("limit",
			mcp.Description("Maximum number of entries to return (default: 100)"),
		),
		mcp.WithString("org",
			mcp.Description(fmt.Sprintf("Organization ID for the query (default: %s from %s env var)", orgID, EnvLokiOrgID)),
		),
		mcp.WithString("format",
			mcp.Description("Output format: raw, json, or text (default: raw)"),
			mcp.DefaultString("raw"),
		),
	)
}

// HandleLokiQuery handles Loki query tool requests
func HandleLokiQuery(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var params LokiParams
	if err := request.BindArguments(&params); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	params.applyDefaults()

	// Apply query filter if configured
	if cfg := GetFilterConfig(); cfg != nil {
		mergedQuery, err := MergeQueryWithFilter(params.Query, cfg.Matchers)
		if err != nil {
			return nil, fmt.Errorf("failed to apply query filter: %w", err)
		}
		params.Query = mergedQuery
	}

	// Set defaults for optional time parameters
	start := time.Now().Add(-DefaultLookback)
	end := time.Now()
	limit := DefaultQueryLimit

	// Override defaults if parameters are provided
	if params.Start != "" {
		startTime, err := parseTime(params.Start)
		if err != nil {
			return nil, fmt.Errorf("invalid start time: %w", err)
		}
		start = startTime
	}

	if params.End != "" {
		endTime, err := parseTime(params.End)
		if err != nil {
			return nil, fmt.Errorf("invalid end time: %w", err)
		}
		end = endTime
	}

	if params.Limit > 0 {
		limit = int(params.Limit)
	}

	// Create Loki client and execute query
	lokiClient := newLokiClient(&params)

	result, err := lokiClient.QueryRange(
		params.Query,
		limit,
		start,
		end,
		logproto.BACKWARD, // newest first
		0,                 // step (0 = auto)
		0,                 // interval (0 = auto)
		true,              // quiet (no progress output)
	)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}

	// Format results
	formattedResult, err := formatLokiResults(result, params.Format)
	if err != nil {
		return nil, fmt.Errorf("failed to format results: %w", err)
	}

	return mcp.NewToolResultText(formattedResult), nil
}

// parseTime parses a time string in various formats
func parseTime(timeStr string) (time.Time, error) {
	// Handle "now" keyword
	if timeStr == "now" {
		return time.Now(), nil
	}

	// Handle relative time strings like "-1h", "-30m"
	if len(timeStr) > 0 && timeStr[0] == '-' {
		duration, err := time.ParseDuration(timeStr)
		if err == nil {
			return time.Now().Add(duration), nil
		}
	}

	// Try parsing as RFC3339
	t, err := time.Parse(time.RFC3339, timeStr)
	if err == nil {
		return t, nil
	}

	// Try other common formats
	formats := []string{
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}

	for _, format := range formats {
		t, err := time.Parse(format, timeStr)
		if err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unsupported time format: %s", timeStr)
}

// formatLokiResults formats the Loki query results into a readable string
func formatLokiResults(result *loghttp.QueryResponse, format string) (string, error) {
	// Extract streams from the result
	streams, ok := result.Data.Result.(loghttp.Streams)
	if !ok {
		// Handle case where result is not streams (could be matrix/vector for metric queries)
		switch format {
		case "json":
			jsonBytes, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return "", fmt.Errorf("failed to marshal JSON: %w", err)
			}
			return string(jsonBytes), nil
		default:
			return fmt.Sprintf("Query returned %s result type", result.Data.ResultType), nil
		}
	}

	if len(streams) == 0 {
		switch format {
		case "json":
			return "{\"message\": \"No logs found matching the query\"}", nil
		default:
			return "No logs found matching the query", nil
		}
	}

	switch format {
	case "json":
		// Return raw JSON response
		jsonBytes, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to marshal JSON: %w", err)
		}
		return string(jsonBytes), nil

	case "raw":
		// Return raw log lines with timestamps and labels in simple format
		var sb strings.Builder
		for _, stream := range streams {
			// Build labels string
			labels := stream.Labels.String()
			if labels != "{}" {
				labels = labels + " "
			} else {
				labels = ""
			}

			for _, entry := range stream.Entries {
				timestamp := entry.Timestamp.Format(time.RFC3339)
				fmt.Fprintf(&sb, "%s %s%s\n", timestamp, labels, entry.Line)
			}
		}
		return sb.String(), nil

	case "text":
		// Return formatted text with timestamps and stream info (original behavior)
		var sb strings.Builder
		fmt.Fprintf(&sb, "Found %d streams:\n\n", len(streams))

		for i, stream := range streams {
			// Format stream labels
			fmt.Fprintf(&sb, "Stream %s %d:\n", stream.Labels.String(), i+1)

			// Format log entries
			for _, entry := range stream.Entries {
				fmt.Fprintf(&sb, "[%s] %s\n", entry.Timestamp.Format(time.RFC3339), entry.Line)
			}
			sb.WriteString("\n")
		}
		return sb.String(), nil

	default:
		return "", fmt.Errorf("unsupported format: %s. Supported formats: raw, json, text", format)
	}
}

// NewLokiLabelNamesTool creates and returns a tool for getting all label names from Grafana Loki
func NewLokiLabelNamesTool() mcp.Tool {
	// Get Loki URL from environment variable or use default
	lokiURL := os.Getenv(EnvLokiURL)
	if lokiURL == "" {
		lokiURL = DefaultLokiURL
	}

	// Get authentication parameters from environment variables if set
	username := os.Getenv(EnvLokiUsername)
	password := os.Getenv(EnvLokiPassword)
	token := os.Getenv(EnvLokiToken)
	orgID := os.Getenv(EnvLokiOrgID)

	return mcp.NewTool("loki_label_names",
		mcp.WithDescription("Get all label names from Grafana Loki"),
		mcp.WithString("url",
			mcp.Description(fmt.Sprintf("Loki server URL (default: %s from %s env var)", lokiURL, EnvLokiURL)),
			mcp.DefaultString(lokiURL),
		),
		mcp.WithString("username",
			mcp.Description(fmt.Sprintf("Username for basic authentication (default: %s from %s env var)", username, EnvLokiUsername)),
		),
		mcp.WithString("password",
			mcp.Description(fmt.Sprintf("Password for basic authentication (default: %s from %s env var)", password, EnvLokiPassword)),
		),
		mcp.WithString("token",
			mcp.Description(fmt.Sprintf("Bearer token for authentication (default: %s from %s env var)", token, EnvLokiToken)),
		),
		mcp.WithString("start",
			mcp.Description("Start time for the query (default: 1h ago)"),
		),
		mcp.WithString("end",
			mcp.Description("End time for the query (default: now)"),
		),
		mcp.WithString("org",
			mcp.Description(fmt.Sprintf("Organization ID for the query (default: %s from %s env var)", orgID, EnvLokiOrgID)),
		),
		mcp.WithString("format",
			mcp.Description("Output format: raw, json, or text (default: raw)"),
			mcp.DefaultString("raw"),
		),
	)
}

// NewLokiLabelValuesTool creates and returns a tool for getting values for a specific label from Grafana Loki
func NewLokiLabelValuesTool() mcp.Tool {
	// Get Loki URL from environment variable or use default
	lokiURL := os.Getenv(EnvLokiURL)
	if lokiURL == "" {
		lokiURL = DefaultLokiURL
	}

	// Get authentication parameters from environment variables if set
	username := os.Getenv(EnvLokiUsername)
	password := os.Getenv(EnvLokiPassword)
	token := os.Getenv(EnvLokiToken)
	orgID := os.Getenv(EnvLokiOrgID)

	return mcp.NewTool("loki_label_values",
		mcp.WithDescription("Get all values for a specific label from Grafana Loki"),
		mcp.WithString("label",
			mcp.Required(),
			mcp.Description("Label name to get values for"),
		),
		mcp.WithString("url",
			mcp.Description(fmt.Sprintf("Loki server URL (default: %s from %s env var)", lokiURL, EnvLokiURL)),
			mcp.DefaultString(lokiURL),
		),
		mcp.WithString("username",
			mcp.Description(fmt.Sprintf("Username for basic authentication (default: %s from %s env var)", username, EnvLokiUsername)),
		),
		mcp.WithString("password",
			mcp.Description(fmt.Sprintf("Password for basic authentication (default: %s from %s env var)", password, EnvLokiPassword)),
		),
		mcp.WithString("token",
			mcp.Description(fmt.Sprintf("Bearer token for authentication (default: %s from %s env var)", token, EnvLokiToken)),
		),
		mcp.WithString("start",
			mcp.Description("Start time for the query (default: 1h ago)"),
		),
		mcp.WithString("end",
			mcp.Description("End time for the query (default: now)"),
		),
		mcp.WithString("org",
			mcp.Description(fmt.Sprintf("Organization ID for the query (default: %s from %s env var)", orgID, EnvLokiOrgID)),
		),
		mcp.WithString("format",
			mcp.Description("Output format: raw, json, or text (default: raw)"),
			mcp.DefaultString("raw"),
		),
	)
}

// HandleLokiLabelNames handles Loki label names tool requests
func HandleLokiLabelNames(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var params LokiParams
	if err := request.BindArguments(&params); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	params.applyDefaults()

	// Set defaults for optional time parameters
	start := time.Now().Add(-DefaultLookback)
	end := time.Now()

	// Override defaults if parameters are provided
	if params.Start != "" {
		startTime, err := parseTime(params.Start)
		if err != nil {
			return nil, fmt.Errorf("invalid start time: %w", err)
		}
		start = startTime
	}

	if params.End != "" {
		endTime, err := parseTime(params.End)
		if err != nil {
			return nil, fmt.Errorf("invalid end time: %w", err)
		}
		end = endTime
	}

	var result *loghttp.LabelResponse
	var err error

	// Use HTTP client with query filter if configured
	if cfg := GetFilterConfig(); cfg != nil {
		httpClient := NewLokiHTTPClient(&params)
		filterQuery := BuildFilterSelector(cfg.Matchers)
		result, err = httpClient.ListLabelNamesWithQuery(filterQuery, start, end)
	} else {
		lokiClient := newLokiClient(&params)
		result, err = lokiClient.ListLabelNames(true, start, end)
	}
	if err != nil {
		return nil, fmt.Errorf("labels query execution failed: %w", err)
	}

	// Format results
	formattedResult, err := formatLokiLabelsResults(result, params.Format)
	if err != nil {
		return nil, fmt.Errorf("failed to format results: %w", err)
	}

	return mcp.NewToolResultText(formattedResult), nil
}

// HandleLokiLabelValues handles Loki label values tool requests
func HandleLokiLabelValues(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var params LokiParams
	if err := request.BindArguments(&params); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	params.applyDefaults()

	// Set defaults for optional time parameters
	start := time.Now().Add(-DefaultLookback)
	end := time.Now()

	// Override defaults if parameters are provided
	if params.Start != "" {
		startTime, err := parseTime(params.Start)
		if err != nil {
			return nil, fmt.Errorf("invalid start time: %w", err)
		}
		start = startTime
	}

	if params.End != "" {
		endTime, err := parseTime(params.End)
		if err != nil {
			return nil, fmt.Errorf("invalid end time: %w", err)
		}
		end = endTime
	}

	var result *loghttp.LabelResponse
	var err error

	// Use HTTP client with query filter if configured
	if cfg := GetFilterConfig(); cfg != nil {
		httpClient := NewLokiHTTPClient(&params)
		filterQuery := BuildFilterSelector(cfg.Matchers)
		result, err = httpClient.ListLabelValuesWithQuery(params.Label, filterQuery, start, end)
	} else {
		lokiClient := newLokiClient(&params)
		result, err = lokiClient.ListLabelValues(params.Label, true, start, end)
	}
	if err != nil {
		return nil, fmt.Errorf("label values query execution failed: %w", err)
	}

	// Format results
	formattedResult, err := formatLokiLabelValuesResults(params.Label, result, params.Format)
	if err != nil {
		return nil, fmt.Errorf("failed to format results: %w", err)
	}

	return mcp.NewToolResultText(formattedResult), nil
}

// formatLokiLabelsResults formats the Loki labels results into a readable string
func formatLokiLabelsResults(result *loghttp.LabelResponse, format string) (string, error) {
	if len(result.Data) == 0 {
		switch format {
		case "json":
			return "{\"message\": \"No labels found\"}", nil
		default:
			return "No labels found", nil
		}
	}

	switch format {
	case "json":
		// Return raw JSON response
		jsonBytes, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to marshal JSON: %w", err)
		}
		return string(jsonBytes), nil

	case "raw":
		// Return raw label names only, one per line
		var sb strings.Builder
		for _, label := range result.Data {
			sb.WriteString(label)
			sb.WriteString("\n")
		}
		return sb.String(), nil

	case "text":
		// Return formatted text with numbering (original behavior)
		var sb strings.Builder
		fmt.Fprintf(&sb, "Found %d labels:\n\n", len(result.Data))

		for i, label := range result.Data {
			fmt.Fprintf(&sb, "%d. %s\n", i+1, label)
		}
		return sb.String(), nil

	default:
		return "", fmt.Errorf("unsupported format: %s. Supported formats: raw, json, text", format)
	}
}

// formatLokiLabelValuesResults formats the Loki label values results into a readable string
func formatLokiLabelValuesResults(labelName string, result *loghttp.LabelResponse, format string) (string, error) {
	if len(result.Data) == 0 {
		switch format {
		case "json":
			return fmt.Sprintf("{\"message\": \"No values found for label '%s'\"}", labelName), nil
		default:
			return fmt.Sprintf("No values found for label '%s'", labelName), nil
		}
	}

	switch format {
	case "json":
		// Return raw JSON response
		jsonBytes, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return "", fmt.Errorf("failed to marshal JSON: %w", err)
		}
		return string(jsonBytes), nil

	case "raw":
		// Return raw label values only, one per line
		var sb strings.Builder
		for _, value := range result.Data {
			sb.WriteString(value)
			sb.WriteString("\n")
		}
		return sb.String(), nil

	case "text":
		// Return formatted text with numbering (original behavior)
		var sb strings.Builder
		fmt.Fprintf(&sb, "Found %d values for label '%s':\n\n", len(result.Data), labelName)

		for i, value := range result.Data {
			fmt.Fprintf(&sb, "%d. %s\n", i+1, value)
		}
		return sb.String(), nil

	default:
		return "", fmt.Errorf("unsupported format: %s. Supported formats: raw, json, text", format)
	}
}
