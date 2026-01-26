package handlers

import (
	"strings"
	"testing"
	"time"

	"github.com/grafana/loki/v3/pkg/loghttp"
)

// TestFormatLokiResults_TimestampParsing tests that timestamps from Loki are correctly formatted
// With the official client, timestamps are already parsed as time.Time, so this test
// verifies that formatting works correctly.
func TestFormatLokiResults_TimestampParsing(t *testing.T) {
	// Test case with known timestamp: 2024-01-15T10:30:45Z
	timestamp := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)

	// Create a sample Loki result with the test timestamp
	result := &loghttp.QueryResponse{
		Status: "success",
		Data: loghttp.QueryResponseData{
			ResultType: loghttp.ResultTypeStream,
			Result: loghttp.Streams{
				{
					Labels: loghttp.LabelSet{
						"job":   "test-job",
						"level": "info",
					},
					Entries: []loghttp.Entry{
						{Timestamp: timestamp, Line: "Test log message"},
					},
				},
			},
		},
	}

	// Format the results
	output, err := formatLokiResults(result, "text")
	if err != nil {
		t.Fatalf("formatLokiResults failed: %v", err)
	}

	// Verify the output contains the correct timestamp
	if !strings.Contains(output, "2024-01-15T") {
		t.Errorf("Expected output to contain date '2024-01-15T', but got:\n%s", output)
	}

	// Verify it doesn't contain the year 2262 (the bug we fixed)
	if strings.Contains(output, "2262") {
		t.Errorf("Output contains year 2262, indicating timestamp parsing bug is present:\n%s", output)
	}

	// Verify it contains the expected log message
	if !strings.Contains(output, "Test log message") {
		t.Errorf("Expected output to contain 'Test log message', but got:\n%s", output)
	}

	// Verify it contains the stream information
	if !strings.Contains(output, "job") && !strings.Contains(output, "test-job") {
		t.Errorf("Expected output to contain stream info 'job' or 'test-job', but got:\n%s", output)
	}
}

// TestFormatLokiResults_MultipleTimestamps tests formatting of multiple log entries with different timestamps
func TestFormatLokiResults_MultipleTimestamps(t *testing.T) {
	result := &loghttp.QueryResponse{
		Status: "success",
		Data: loghttp.QueryResponseData{
			ResultType: loghttp.ResultTypeStream,
			Result: loghttp.Streams{
				{
					Labels: loghttp.LabelSet{
						"job": "test-job",
					},
					Entries: []loghttp.Entry{
						{Timestamp: time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC), Line: "First log message"},
						{Timestamp: time.Date(2024, 1, 15, 10, 31, 0, 0, time.UTC), Line: "Second log message"},
						{Timestamp: time.Date(2024, 1, 15, 10, 31, 15, 0, time.UTC), Line: "Third log message"},
					},
				},
			},
		},
	}

	output, err := formatLokiResults(result, "text")
	if err != nil {
		t.Fatalf("formatLokiResults failed: %v", err)
	}

	// Check that all timestamps are in 2024, not 2262
	if strings.Contains(output, "2262") {
		t.Errorf("Output contains year 2262, indicating timestamp parsing bug:\n%s", output)
	}

	// All timestamps should be in 2024
	occurrences := strings.Count(output, "2024")
	if occurrences < 3 {
		t.Errorf("Expected at least 3 occurrences of '2024' in output, but found %d:\n%s", occurrences, output)
	}
}

// TestFormatLokiResults_EmptyResult tests handling of empty results
func TestFormatLokiResults_EmptyResult(t *testing.T) {
	result := &loghttp.QueryResponse{
		Status: "success",
		Data: loghttp.QueryResponseData{
			ResultType: loghttp.ResultTypeStream,
			Result:     loghttp.Streams{},
		},
	}

	output, err := formatLokiResults(result, "text")
	if err != nil {
		t.Fatalf("formatLokiResults failed: %v", err)
	}

	expected := "No logs found matching the query"
	if output != expected {
		t.Errorf("Expected output '%s', but got '%s'", expected, output)
	}
}

// TestFormatLokiResults_RecentTimestamp tests with a very recent timestamp to ensure current dates work
func TestFormatLokiResults_RecentTimestamp(t *testing.T) {
	// Use current time
	now := time.Now().UTC()

	result := &loghttp.QueryResponse{
		Status: "success",
		Data: loghttp.QueryResponseData{
			ResultType: loghttp.ResultTypeStream,
			Result: loghttp.Streams{
				{
					Labels: loghttp.LabelSet{
						"job": "recent-test",
					},
					Entries: []loghttp.Entry{
						{Timestamp: now, Line: "Recent log message"},
					},
				},
			},
		},
	}

	output, err := formatLokiResults(result, "text")
	if err != nil {
		t.Fatalf("formatLokiResults failed: %v", err)
	}

	// Should contain current year, not 2262
	currentYear := now.Format("2006")
	if !strings.Contains(output, currentYear) {
		t.Errorf("Expected output to contain current year %s, but got:\n%s", currentYear, output)
	}

	if strings.Contains(output, "2262") {
		t.Errorf("Output contains year 2262, indicating timestamp parsing bug:\n%s", output)
	}
}

// TestFormatLokiResults_NoYear2262Bug is a regression test for the specific bug reported in issue #3
// This test ensures that timestamps never show year 2262 due to incorrect nanosecond conversion
func TestFormatLokiResults_NoYear2262Bug(t *testing.T) {
	// This test uses a variety of realistic timestamps
	testCases := []struct {
		name         string
		timestamp    time.Time
		expectedYear string
	}{
		{
			name:         "Current timestamp",
			timestamp:    time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC),
			expectedYear: "2024",
		},
		{
			name:         "Recent timestamp",
			timestamp:    time.Date(2023, 11, 14, 22, 13, 20, 0, time.UTC),
			expectedYear: "2023",
		},
		{
			name:         "Future timestamp",
			timestamp:    time.Date(2027, 1, 11, 2, 13, 20, 0, time.UTC),
			expectedYear: "2027",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := &loghttp.QueryResponse{
				Status: "success",
				Data: loghttp.QueryResponseData{
					ResultType: loghttp.ResultTypeStream,
					Result: loghttp.Streams{
						{
							Labels: loghttp.LabelSet{
								"job": "regression-test",
							},
							Entries: []loghttp.Entry{
								{Timestamp: tc.timestamp, Line: "Test log message"},
							},
						},
					},
				},
			}

			output, err := formatLokiResults(result, "text")
			if err != nil {
				t.Fatalf("formatLokiResults failed: %v", err)
			}

			// The main regression check: ensure we never see year 2262
			if strings.Contains(output, "2262") {
				t.Errorf("REGRESSION: Output contains year 2262, the original bug is present:\n%s", output)
			}

			// Verify we see the expected year instead
			if !strings.Contains(output, tc.expectedYear) {
				t.Errorf("Expected output to contain year %s, but got:\n%s", tc.expectedYear, output)
			}
		})
	}
}

// TestFormatLokiResults_RawFormat tests the raw output format
func TestFormatLokiResults_RawFormat(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)

	result := &loghttp.QueryResponse{
		Status: "success",
		Data: loghttp.QueryResponseData{
			ResultType: loghttp.ResultTypeStream,
			Result: loghttp.Streams{
				{
					Labels: loghttp.LabelSet{
						"job": "test-job",
					},
					Entries: []loghttp.Entry{
						{Timestamp: timestamp, Line: "Test log message"},
					},
				},
			},
		},
	}

	output, err := formatLokiResults(result, "raw")
	if err != nil {
		t.Fatalf("formatLokiResults failed: %v", err)
	}

	// Raw format should have timestamp, labels, and message
	if !strings.Contains(output, "2024-01-15T10:30:45Z") {
		t.Errorf("Expected output to contain timestamp '2024-01-15T10:30:45Z', but got:\n%s", output)
	}

	if !strings.Contains(output, "Test log message") {
		t.Errorf("Expected output to contain 'Test log message', but got:\n%s", output)
	}
}

// TestFormatLokiResults_JSONFormat tests the JSON output format
func TestFormatLokiResults_JSONFormat(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 45, 0, time.UTC)

	result := &loghttp.QueryResponse{
		Status: "success",
		Data: loghttp.QueryResponseData{
			ResultType: loghttp.ResultTypeStream,
			Result: loghttp.Streams{
				{
					Labels: loghttp.LabelSet{
						"job": "test-job",
					},
					Entries: []loghttp.Entry{
						{Timestamp: timestamp, Line: "Test log message"},
					},
				},
			},
		},
	}

	output, err := formatLokiResults(result, "json")
	if err != nil {
		t.Fatalf("formatLokiResults failed: %v", err)
	}

	// JSON format should be valid JSON with status
	if !strings.Contains(output, `"status"`) {
		t.Errorf("Expected output to contain '\"status\"', but got:\n%s", output)
	}
}

// TestFormatLokiLabelsResults tests the labels formatting
func TestFormatLokiLabelsResults(t *testing.T) {
	result := &loghttp.LabelResponse{
		Status: "success",
		Data:   []string{"job", "level", "namespace"},
	}

	// Test raw format
	output, err := formatLokiLabelsResults(result, "raw")
	if err != nil {
		t.Fatalf("formatLokiLabelsResults failed: %v", err)
	}

	if !strings.Contains(output, "job\n") {
		t.Errorf("Expected output to contain 'job', but got:\n%s", output)
	}

	// Test text format
	output, err = formatLokiLabelsResults(result, "text")
	if err != nil {
		t.Fatalf("formatLokiLabelsResults failed: %v", err)
	}

	if !strings.Contains(output, "Found 3 labels") {
		t.Errorf("Expected output to contain 'Found 3 labels', but got:\n%s", output)
	}
}

// TestFormatLokiLabelValuesResults tests the label values formatting
func TestFormatLokiLabelValuesResults(t *testing.T) {
	result := &loghttp.LabelResponse{
		Status: "success",
		Data:   []string{"value1", "value2", "value3"},
	}

	// Test raw format
	output, err := formatLokiLabelValuesResults("testlabel", result, "raw")
	if err != nil {
		t.Fatalf("formatLokiLabelValuesResults failed: %v", err)
	}

	if !strings.Contains(output, "value1\n") {
		t.Errorf("Expected output to contain 'value1', but got:\n%s", output)
	}

	// Test text format
	output, err = formatLokiLabelValuesResults("testlabel", result, "text")
	if err != nil {
		t.Fatalf("formatLokiLabelValuesResults failed: %v", err)
	}

	if !strings.Contains(output, "Found 3 values for label 'testlabel'") {
		t.Errorf("Expected output to contain label count and name, but got:\n%s", output)
	}
}
