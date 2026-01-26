//go:build integration

package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// LokiContainer wraps a testcontainers generic container for Loki
type LokiContainer struct {
	testcontainers.Container
	Endpoint string
}

var lokiContainer *LokiContainer

// SetupLokiContainer starts a Loki container for testing
func SetupLokiContainer(ctx context.Context) (*LokiContainer, error) {
	req := testcontainers.ContainerRequest{
		Image:        "grafana/loki:3.6.0",
		ExposedPorts: []string{"3100/tcp"},
		WaitingFor:   wait.ForHTTP("/ready").WithPort("3100/tcp").WithStartupTimeout(60 * time.Second),
		// Run as root to avoid permission issues with /loki directory
		User: "0",
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start Loki container: %w", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get container host: %w", err)
	}

	mappedPort, err := container.MappedPort(ctx, "3100")
	if err != nil {
		return nil, fmt.Errorf("failed to get mapped port: %w", err)
	}

	endpoint := fmt.Sprintf("http://%s:%s", host, mappedPort.Port())

	return &LokiContainer{
		Container: container,
		Endpoint:  endpoint,
	}, nil
}

// LokiStream represents a stream of log entries for pushing to Loki
type LokiStream struct {
	Stream map[string]string `json:"stream"`
	Values [][]string        `json:"values"`
}

// LokiPushRequest represents the request body for pushing logs to Loki
type LokiPushRequest struct {
	Streams []LokiStream `json:"streams"`
}

// PushLogs pushes test logs to Loki via the /loki/api/v1/push endpoint
func PushLogs(ctx context.Context, lokiURL string, streams []LokiStream) error {
	pushURL := lokiURL + "/loki/api/v1/push"

	payload := LokiPushRequest{Streams: streams}
	jsonData, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal logs: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, pushURL, bytes.NewReader(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to push logs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return nil
}

// NewCallToolRequest creates an mcp.CallToolRequest for testing handlers
func NewCallToolRequest(name string, args map[string]interface{}) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: args,
		},
	}
}

// extractTextContent extracts text content from an mcp.CallToolResult
func extractTextContent(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}

	for _, content := range result.Content {
		if textContent, ok := content.(mcp.TextContent); ok {
			return textContent.Text
		}
	}
	return ""
}

// UniqueTestID generates a unique test ID for isolation
func UniqueTestID() string {
	return uuid.New().String()[:8]
}

// makeTimestampNs creates a nanosecond timestamp string for Loki
func makeTimestampNs(t time.Time) string {
	return fmt.Sprintf("%d", t.UnixNano())
}

func TestMain(m *testing.M) {
	ctx := context.Background()

	var err error
	lokiContainer, err = SetupLokiContainer(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start Loki container: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	if lokiContainer != nil {
		if err := lokiContainer.Terminate(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to terminate Loki container: %v\n", err)
		}
	}

	os.Exit(code)
}

// ============================================================================
// loki_query Tests
// ============================================================================

func TestIntegration_LokiQuery_BasicQuery(t *testing.T) {
	ctx := context.Background()
	testID := UniqueTestID()

	// Insert 3 logs with unique test ID
	timestamp := time.Now().Add(-30 * time.Second)
	streams := []LokiStream{
		{
			Stream: map[string]string{
				"job":     "test-job",
				"test_id": testID,
			},
			Values: [][]string{
				{makeTimestampNs(timestamp), "Log message 1"},
				{makeTimestampNs(timestamp.Add(1 * time.Second)), "Log message 2"},
				{makeTimestampNs(timestamp.Add(2 * time.Second)), "Log message 3"},
			},
		},
	}

	err := PushLogs(ctx, lokiContainer.Endpoint, streams)
	require.NoError(t, err, "Failed to push logs")

	// Wait for Loki to index the logs
	time.Sleep(2 * time.Second)

	// Query the logs back
	request := NewCallToolRequest("loki_query", map[string]interface{}{
		"query":  fmt.Sprintf(`{test_id="%s"}`, testID),
		"url":    lokiContainer.Endpoint,
		"format": "text",
		"start":  "-5m",
		"end":    "now",
	})

	result, err := HandleLokiQuery(ctx, request)
	require.NoError(t, err, "HandleLokiQuery failed")

	text := extractTextContent(result)
	assert.Contains(t, text, "Found 1 streams", "Should find 1 stream")
	assert.Contains(t, text, "Log message 1", "Should contain first log")
	assert.Contains(t, text, "Log message 2", "Should contain second log")
	assert.Contains(t, text, "Log message 3", "Should contain third log")
}

func TestIntegration_LokiQuery_EmptyResults(t *testing.T) {
	ctx := context.Background()
	testID := UniqueTestID()

	// Query for non-existent labels
	request := NewCallToolRequest("loki_query", map[string]interface{}{
		"query":  fmt.Sprintf(`{test_id="%s"}`, testID),
		"url":    lokiContainer.Endpoint,
		"format": "text",
		"start":  "-5m",
		"end":    "now",
	})

	result, err := HandleLokiQuery(ctx, request)
	require.NoError(t, err, "HandleLokiQuery failed")

	text := extractTextContent(result)
	assert.Contains(t, text, "No logs found", "Should indicate no logs found")
}

func TestIntegration_LokiQuery_MultipleStreams(t *testing.T) {
	ctx := context.Background()
	testID := UniqueTestID()

	// Insert logs with different label values creating multiple streams
	timestamp := time.Now().Add(-30 * time.Second)
	streams := []LokiStream{
		{
			Stream: map[string]string{
				"job":     "job-alpha",
				"test_id": testID,
			},
			Values: [][]string{
				{makeTimestampNs(timestamp), "Alpha log 1"},
				{makeTimestampNs(timestamp.Add(1 * time.Second)), "Alpha log 2"},
			},
		},
		{
			Stream: map[string]string{
				"job":     "job-beta",
				"test_id": testID,
			},
			Values: [][]string{
				{makeTimestampNs(timestamp.Add(2 * time.Second)), "Beta log 1"},
				{makeTimestampNs(timestamp.Add(3 * time.Second)), "Beta log 2"},
			},
		},
	}

	err := PushLogs(ctx, lokiContainer.Endpoint, streams)
	require.NoError(t, err, "Failed to push logs")

	// Wait for Loki to index
	time.Sleep(2 * time.Second)

	// Query all logs with the test ID
	request := NewCallToolRequest("loki_query", map[string]interface{}{
		"query":  fmt.Sprintf(`{test_id="%s"}`, testID),
		"url":    lokiContainer.Endpoint,
		"format": "text",
		"start":  "-5m",
		"end":    "now",
	})

	result, err := HandleLokiQuery(ctx, request)
	require.NoError(t, err, "HandleLokiQuery failed")

	text := extractTextContent(result)
	assert.Contains(t, text, "Found 2 streams", "Should find 2 streams")
	assert.Contains(t, text, "Alpha log", "Should contain alpha logs")
	assert.Contains(t, text, "Beta log", "Should contain beta logs")
}

func TestIntegration_LokiQuery_WithLimit(t *testing.T) {
	ctx := context.Background()
	testID := UniqueTestID()

	// Insert 10 logs
	timestamp := time.Now().Add(-30 * time.Second)
	values := make([][]string, 10)
	for i := 0; i < 10; i++ {
		values[i] = []string{
			makeTimestampNs(timestamp.Add(time.Duration(i) * time.Second)),
			fmt.Sprintf("Log entry %d", i+1),
		}
	}

	streams := []LokiStream{
		{
			Stream: map[string]string{
				"job":     "test-job",
				"test_id": testID,
			},
			Values: values,
		},
	}

	err := PushLogs(ctx, lokiContainer.Endpoint, streams)
	require.NoError(t, err, "Failed to push logs")

	// Wait for Loki to index
	time.Sleep(2 * time.Second)

	// Query with limit=3
	request := NewCallToolRequest("loki_query", map[string]interface{}{
		"query":  fmt.Sprintf(`{test_id="%s"}`, testID),
		"url":    lokiContainer.Endpoint,
		"format": "raw",
		"limit":  float64(3),
		"start":  "-5m",
		"end":    "now",
	})

	result, err := HandleLokiQuery(ctx, request)
	require.NoError(t, err, "HandleLokiQuery failed")

	text := extractTextContent(result)
	// Count log entries (each line is a log entry in raw format)
	lines := strings.Split(strings.TrimSpace(text), "\n")
	assert.LessOrEqual(t, len(lines), 3, "Should return at most 3 log entries")
}

func TestIntegration_LokiQuery_RawFormat(t *testing.T) {
	ctx := context.Background()
	testID := UniqueTestID()

	timestamp := time.Now().Add(-30 * time.Second)
	streams := []LokiStream{
		{
			Stream: map[string]string{
				"job":     "test-job",
				"level":   "info",
				"test_id": testID,
			},
			Values: [][]string{
				{makeTimestampNs(timestamp), "Raw format test message"},
			},
		},
	}

	err := PushLogs(ctx, lokiContainer.Endpoint, streams)
	require.NoError(t, err, "Failed to push logs")

	time.Sleep(2 * time.Second)

	request := NewCallToolRequest("loki_query", map[string]interface{}{
		"query":  fmt.Sprintf(`{test_id="%s"}`, testID),
		"url":    lokiContainer.Endpoint,
		"format": "raw",
		"start":  "-5m",
		"end":    "now",
	})

	result, err := HandleLokiQuery(ctx, request)
	require.NoError(t, err, "HandleLokiQuery failed")

	text := extractTextContent(result)
	// Raw format: timestamp labels message
	assert.Contains(t, text, "Raw format test message", "Should contain log message")
	assert.Contains(t, text, "job", "Should contain labels")
	// Should have RFC3339 timestamp format
	assert.Regexp(t, `\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`, text, "Should contain RFC3339 timestamp")
}

func TestIntegration_LokiQuery_JSONFormat(t *testing.T) {
	ctx := context.Background()
	testID := UniqueTestID()

	timestamp := time.Now().Add(-30 * time.Second)
	streams := []LokiStream{
		{
			Stream: map[string]string{
				"job":     "test-job",
				"test_id": testID,
			},
			Values: [][]string{
				{makeTimestampNs(timestamp), "JSON format test message"},
			},
		},
	}

	err := PushLogs(ctx, lokiContainer.Endpoint, streams)
	require.NoError(t, err, "Failed to push logs")

	time.Sleep(2 * time.Second)

	request := NewCallToolRequest("loki_query", map[string]interface{}{
		"query":  fmt.Sprintf(`{test_id="%s"}`, testID),
		"url":    lokiContainer.Endpoint,
		"format": "json",
		"start":  "-5m",
		"end":    "now",
	})

	result, err := HandleLokiQuery(ctx, request)
	require.NoError(t, err, "HandleLokiQuery failed")

	text := extractTextContent(result)

	// Verify it's valid JSON
	var jsonResult map[string]interface{}
	err = json.Unmarshal([]byte(text), &jsonResult)
	require.NoError(t, err, "Result should be valid JSON")

	// Check expected fields
	assert.Contains(t, jsonResult, "status", "JSON should contain status field")
	assert.Contains(t, jsonResult, "data", "JSON should contain data field")
}

func TestIntegration_LokiQuery_TextFormat(t *testing.T) {
	ctx := context.Background()
	testID := UniqueTestID()

	timestamp := time.Now().Add(-30 * time.Second)
	streams := []LokiStream{
		{
			Stream: map[string]string{
				"job":     "test-job",
				"test_id": testID,
			},
			Values: [][]string{
				{makeTimestampNs(timestamp), "Text format test message"},
			},
		},
	}

	err := PushLogs(ctx, lokiContainer.Endpoint, streams)
	require.NoError(t, err, "Failed to push logs")

	time.Sleep(2 * time.Second)

	request := NewCallToolRequest("loki_query", map[string]interface{}{
		"query":  fmt.Sprintf(`{test_id="%s"}`, testID),
		"url":    lokiContainer.Endpoint,
		"format": "text",
		"start":  "-5m",
		"end":    "now",
	})

	result, err := HandleLokiQuery(ctx, request)
	require.NoError(t, err, "HandleLokiQuery failed")

	text := extractTextContent(result)
	assert.Contains(t, text, "Found", "Text format should contain 'Found'")
	assert.Contains(t, text, "streams", "Text format should contain 'streams'")
	assert.Contains(t, text, "Text format test message", "Should contain log message")
}

// ============================================================================
// loki_label_names Tests
// ============================================================================

func TestIntegration_LokiLabelNames_Basic(t *testing.T) {
	ctx := context.Background()
	testID := UniqueTestID()

	// Insert logs with multiple labels
	timestamp := time.Now().Add(-30 * time.Second)
	streams := []LokiStream{
		{
			Stream: map[string]string{
				"job":         "label-test-job",
				"environment": "testing",
				"service":     "label-service",
				"test_id":     testID,
			},
			Values: [][]string{
				{makeTimestampNs(timestamp), "Label test log"},
			},
		},
	}

	err := PushLogs(ctx, lokiContainer.Endpoint, streams)
	require.NoError(t, err, "Failed to push logs")

	time.Sleep(2 * time.Second)

	request := NewCallToolRequest("loki_label_names", map[string]interface{}{
		"url":    lokiContainer.Endpoint,
		"format": "raw",
		"start":  "-5m",
		"end":    "now",
	})

	result, err := HandleLokiLabelNames(ctx, request)
	require.NoError(t, err, "HandleLokiLabelNames failed")

	text := extractTextContent(result)
	assert.Contains(t, text, "job", "Should contain 'job' label")
	assert.Contains(t, text, "test_id", "Should contain 'test_id' label")
}

func TestIntegration_LokiLabelNames_AllFormats(t *testing.T) {
	ctx := context.Background()
	testID := UniqueTestID()

	// Insert logs to ensure labels exist
	timestamp := time.Now().Add(-30 * time.Second)
	streams := []LokiStream{
		{
			Stream: map[string]string{
				"job":     "format-test",
				"test_id": testID,
			},
			Values: [][]string{
				{makeTimestampNs(timestamp), "Format test log"},
			},
		},
	}

	err := PushLogs(ctx, lokiContainer.Endpoint, streams)
	require.NoError(t, err, "Failed to push logs")

	time.Sleep(2 * time.Second)

	testCases := []struct {
		format   string
		validate func(t *testing.T, text string)
	}{
		{
			format: "raw",
			validate: func(t *testing.T, text string) {
				// Raw format: one label per line
				assert.Contains(t, text, "job", "Raw format should contain 'job'")
			},
		},
		{
			format: "json",
			validate: func(t *testing.T, text string) {
				var jsonResult map[string]interface{}
				err := json.Unmarshal([]byte(text), &jsonResult)
				require.NoError(t, err, "JSON format should be valid JSON")
				assert.Contains(t, jsonResult, "status", "JSON should have status")
				assert.Contains(t, jsonResult, "data", "JSON should have data")
			},
		},
		{
			format: "text",
			validate: func(t *testing.T, text string) {
				assert.Contains(t, text, "Found", "Text format should contain 'Found'")
				assert.Contains(t, text, "labels", "Text format should contain 'labels'")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.format, func(t *testing.T) {
			request := NewCallToolRequest("loki_label_names", map[string]interface{}{
				"url":    lokiContainer.Endpoint,
				"format": tc.format,
				"start":  "-5m",
				"end":    "now",
			})

			result, err := HandleLokiLabelNames(ctx, request)
			require.NoError(t, err, "HandleLokiLabelNames failed")

			text := extractTextContent(result)
			tc.validate(t, text)
		})
	}
}

// ============================================================================
// loki_label_values Tests
// ============================================================================

func TestIntegration_LokiLabelValues_Basic(t *testing.T) {
	ctx := context.Background()
	testID := UniqueTestID()

	// Insert logs with different values for the same label
	timestamp := time.Now().Add(-30 * time.Second)
	streams := []LokiStream{
		{
			Stream: map[string]string{
				"job":     "values-job-1",
				"test_id": testID,
			},
			Values: [][]string{
				{makeTimestampNs(timestamp), "Values test log 1"},
			},
		},
		{
			Stream: map[string]string{
				"job":     "values-job-2",
				"test_id": testID,
			},
			Values: [][]string{
				{makeTimestampNs(timestamp.Add(1 * time.Second)), "Values test log 2"},
			},
		},
	}

	err := PushLogs(ctx, lokiContainer.Endpoint, streams)
	require.NoError(t, err, "Failed to push logs")

	time.Sleep(2 * time.Second)

	request := NewCallToolRequest("loki_label_values", map[string]interface{}{
		"label":  "job",
		"url":    lokiContainer.Endpoint,
		"format": "raw",
		"start":  "-5m",
		"end":    "now",
	})

	result, err := HandleLokiLabelValues(ctx, request)
	require.NoError(t, err, "HandleLokiLabelValues failed")

	text := extractTextContent(result)
	assert.Contains(t, text, "values-job-1", "Should contain first job value")
	assert.Contains(t, text, "values-job-2", "Should contain second job value")
}

func TestIntegration_LokiLabelValues_AllFormats(t *testing.T) {
	ctx := context.Background()
	testID := UniqueTestID()

	// Insert logs to ensure label values exist
	timestamp := time.Now().Add(-30 * time.Second)
	streams := []LokiStream{
		{
			Stream: map[string]string{
				"job":     "format-values-test",
				"test_id": testID,
			},
			Values: [][]string{
				{makeTimestampNs(timestamp), "Format values test log"},
			},
		},
	}

	err := PushLogs(ctx, lokiContainer.Endpoint, streams)
	require.NoError(t, err, "Failed to push logs")

	time.Sleep(2 * time.Second)

	testCases := []struct {
		format   string
		validate func(t *testing.T, text string)
	}{
		{
			format: "raw",
			validate: func(t *testing.T, text string) {
				// Raw format: one value per line
				lines := strings.Split(strings.TrimSpace(text), "\n")
				assert.Greater(t, len(lines), 0, "Raw format should have at least one value")
			},
		},
		{
			format: "json",
			validate: func(t *testing.T, text string) {
				var jsonResult map[string]interface{}
				err := json.Unmarshal([]byte(text), &jsonResult)
				require.NoError(t, err, "JSON format should be valid JSON")
				assert.Contains(t, jsonResult, "status", "JSON should have status")
				assert.Contains(t, jsonResult, "data", "JSON should have data")
			},
		},
		{
			format: "text",
			validate: func(t *testing.T, text string) {
				assert.Contains(t, text, "Found", "Text format should contain 'Found'")
				assert.Contains(t, text, "values for label", "Text format should contain 'values for label'")
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.format, func(t *testing.T) {
			request := NewCallToolRequest("loki_label_values", map[string]interface{}{
				"label":  "job",
				"url":    lokiContainer.Endpoint,
				"format": tc.format,
				"start":  "-5m",
				"end":    "now",
			})

			result, err := HandleLokiLabelValues(ctx, request)
			require.NoError(t, err, "HandleLokiLabelValues failed")

			text := extractTextContent(result)
			tc.validate(t, text)
		})
	}
}

// ============================================================================
// Query Filter Tests
// ============================================================================

func TestIntegration_QueryFilter_RestrictsQueryResults(t *testing.T) {
	ctx := context.Background()
	testID := UniqueTestID()

	// Insert logs with different namespaces
	timestamp := time.Now().Add(-30 * time.Second)
	streams := []LokiStream{
		{
			Stream: map[string]string{
				"job":       "api",
				"namespace": "allowed",
				"test_id":   testID,
			},
			Values: [][]string{
				{makeTimestampNs(timestamp), "Allowed namespace log"},
			},
		},
		{
			Stream: map[string]string{
				"job":       "api",
				"namespace": "restricted",
				"test_id":   testID,
			},
			Values: [][]string{
				{makeTimestampNs(timestamp.Add(1 * time.Second)), "Restricted namespace log"},
			},
		},
	}

	err := PushLogs(ctx, lokiContainer.Endpoint, streams)
	require.NoError(t, err, "Failed to push logs")

	time.Sleep(2 * time.Second)

	// Set up query filter to only allow "allowed" namespace
	ResetFilterConfig()
	err = InitializeFilterConfig(fmt.Sprintf(`{namespace="allowed", test_id="%s"}`, testID))
	require.NoError(t, err, "Failed to initialize filter config")
	defer ResetFilterConfig()

	// Query for all logs with the test ID - filter should restrict results
	request := NewCallToolRequest("loki_query", map[string]interface{}{
		"query":  fmt.Sprintf(`{test_id="%s"}`, testID),
		"url":    lokiContainer.Endpoint,
		"format": "text",
		"start":  "-5m",
		"end":    "now",
	})

	result, err := HandleLokiQuery(ctx, request)
	require.NoError(t, err, "HandleLokiQuery failed")

	text := extractTextContent(result)
	assert.Contains(t, text, "Allowed namespace log", "Should contain allowed namespace log")
	assert.NotContains(t, text, "Restricted namespace log", "Should NOT contain restricted namespace log")
}

func TestIntegration_QueryFilter_MergesWithClientQuery(t *testing.T) {
	ctx := context.Background()
	testID := UniqueTestID()

	// Insert logs with different jobs and namespaces
	timestamp := time.Now().Add(-30 * time.Second)
	streams := []LokiStream{
		{
			Stream: map[string]string{
				"job":       "api",
				"namespace": "prod",
				"test_id":   testID,
			},
			Values: [][]string{
				{makeTimestampNs(timestamp), "prod-api log"},
			},
		},
		{
			Stream: map[string]string{
				"job":       "worker",
				"namespace": "prod",
				"test_id":   testID,
			},
			Values: [][]string{
				{makeTimestampNs(timestamp.Add(1 * time.Second)), "prod-worker log"},
			},
		},
		{
			Stream: map[string]string{
				"job":       "api",
				"namespace": "staging",
				"test_id":   testID,
			},
			Values: [][]string{
				{makeTimestampNs(timestamp.Add(2 * time.Second)), "staging-api log"},
			},
		},
	}

	err := PushLogs(ctx, lokiContainer.Endpoint, streams)
	require.NoError(t, err, "Failed to push logs")

	time.Sleep(2 * time.Second)

	// Set up query filter for prod namespace
	ResetFilterConfig()
	err = InitializeFilterConfig(fmt.Sprintf(`{namespace="prod", test_id="%s"}`, testID))
	require.NoError(t, err, "Failed to initialize filter config")
	defer ResetFilterConfig()

	// Client queries for job="api" - should only get prod-api, not staging-api
	request := NewCallToolRequest("loki_query", map[string]interface{}{
		"query":  `{job="api"}`,
		"url":    lokiContainer.Endpoint,
		"format": "text",
		"start":  "-5m",
		"end":    "now",
	})

	result, err := HandleLokiQuery(ctx, request)
	require.NoError(t, err, "HandleLokiQuery failed")

	text := extractTextContent(result)
	assert.Contains(t, text, "prod-api log", "Should contain prod-api log")
	assert.NotContains(t, text, "staging-api log", "Should NOT contain staging-api log")
	assert.NotContains(t, text, "prod-worker log", "Should NOT contain prod-worker log (wrong job)")
}

func TestIntegration_QueryFilter_LabelNamesRespectFilter(t *testing.T) {
	ctx := context.Background()
	testID := UniqueTestID()

	// Insert logs with unique labels per namespace
	timestamp := time.Now().Add(-30 * time.Second)
	streams := []LokiStream{
		{
			Stream: map[string]string{
				"job":              "api",
				"namespace":        "filtered-ns",
				"unique_label_one": "value1",
				"test_id":          testID,
			},
			Values: [][]string{
				{makeTimestampNs(timestamp), "filtered namespace log"},
			},
		},
		{
			Stream: map[string]string{
				"job":              "api",
				"namespace":        "other-ns",
				"unique_label_two": "value2",
				"test_id":          testID,
			},
			Values: [][]string{
				{makeTimestampNs(timestamp.Add(1 * time.Second)), "other namespace log"},
			},
		},
	}

	err := PushLogs(ctx, lokiContainer.Endpoint, streams)
	require.NoError(t, err, "Failed to push logs")

	time.Sleep(2 * time.Second)

	// Set up query filter for filtered-ns namespace
	ResetFilterConfig()
	err = InitializeFilterConfig(fmt.Sprintf(`{namespace="filtered-ns", test_id="%s"}`, testID))
	require.NoError(t, err, "Failed to initialize filter config")
	defer ResetFilterConfig()

	request := NewCallToolRequest("loki_label_names", map[string]interface{}{
		"url":    lokiContainer.Endpoint,
		"format": "raw",
		"start":  "-5m",
		"end":    "now",
	})

	result, err := HandleLokiLabelNames(ctx, request)
	require.NoError(t, err, "HandleLokiLabelNames failed")

	text := extractTextContent(result)
	// Should contain unique_label_one from filtered-ns but not unique_label_two from other-ns
	assert.Contains(t, text, "unique_label_one", "Should contain label from filtered namespace")
	assert.NotContains(t, text, "unique_label_two", "Should NOT contain label from other namespace")
}

func TestIntegration_QueryFilter_LabelValuesRespectFilter(t *testing.T) {
	ctx := context.Background()
	testID := UniqueTestID()

	// Insert logs with different job values per namespace
	timestamp := time.Now().Add(-30 * time.Second)
	streams := []LokiStream{
		{
			Stream: map[string]string{
				"job":       "filter-job-allowed",
				"namespace": "filter-test-ns",
				"test_id":   testID,
			},
			Values: [][]string{
				{makeTimestampNs(timestamp), "allowed job log"},
			},
		},
		{
			Stream: map[string]string{
				"job":       "filter-job-blocked",
				"namespace": "other-test-ns",
				"test_id":   testID,
			},
			Values: [][]string{
				{makeTimestampNs(timestamp.Add(1 * time.Second)), "blocked job log"},
			},
		},
	}

	err := PushLogs(ctx, lokiContainer.Endpoint, streams)
	require.NoError(t, err, "Failed to push logs")

	time.Sleep(2 * time.Second)

	// Set up query filter
	ResetFilterConfig()
	err = InitializeFilterConfig(fmt.Sprintf(`{namespace="filter-test-ns", test_id="%s"}`, testID))
	require.NoError(t, err, "Failed to initialize filter config")
	defer ResetFilterConfig()

	request := NewCallToolRequest("loki_label_values", map[string]interface{}{
		"label":  "job",
		"url":    lokiContainer.Endpoint,
		"format": "raw",
		"start":  "-5m",
		"end":    "now",
	})

	result, err := HandleLokiLabelValues(ctx, request)
	require.NoError(t, err, "HandleLokiLabelValues failed")

	text := extractTextContent(result)
	// Should contain job value from filtered namespace but not from other namespace
	assert.Contains(t, text, "filter-job-allowed", "Should contain job value from filtered namespace")
	assert.NotContains(t, text, "filter-job-blocked", "Should NOT contain job value from other namespace")
}

func TestIntegration_QueryFilter_NoFilterReturnsAll(t *testing.T) {
	ctx := context.Background()
	testID := UniqueTestID()

	// Insert logs with different namespaces
	timestamp := time.Now().Add(-30 * time.Second)
	streams := []LokiStream{
		{
			Stream: map[string]string{
				"job":       "api",
				"namespace": "ns-one",
				"test_id":   testID,
			},
			Values: [][]string{
				{makeTimestampNs(timestamp), "namespace one log"},
			},
		},
		{
			Stream: map[string]string{
				"job":       "api",
				"namespace": "ns-two",
				"test_id":   testID,
			},
			Values: [][]string{
				{makeTimestampNs(timestamp.Add(1 * time.Second)), "namespace two log"},
			},
		},
	}

	err := PushLogs(ctx, lokiContainer.Endpoint, streams)
	require.NoError(t, err, "Failed to push logs")

	time.Sleep(2 * time.Second)

	// Ensure no filter is configured
	ResetFilterConfig()

	// Query for all logs with the test ID - should get both namespaces
	request := NewCallToolRequest("loki_query", map[string]interface{}{
		"query":  fmt.Sprintf(`{test_id="%s"}`, testID),
		"url":    lokiContainer.Endpoint,
		"format": "text",
		"start":  "-5m",
		"end":    "now",
	})

	result, err := HandleLokiQuery(ctx, request)
	require.NoError(t, err, "HandleLokiQuery failed")

	text := extractTextContent(result)
	assert.Contains(t, text, "namespace one log", "Should contain namespace one log")
	assert.Contains(t, text, "namespace two log", "Should contain namespace two log")
}
