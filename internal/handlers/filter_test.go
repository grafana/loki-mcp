package handlers

import (
	"testing"

	"github.com/prometheus/prometheus/model/labels"
)

func TestParseFilter(t *testing.T) {
	tests := []struct {
		name      string
		filter    string
		wantCount int
		wantErr   bool
	}{
		{
			name:      "empty filter",
			filter:    "",
			wantCount: 0,
			wantErr:   false,
		},
		{
			name:      "single matcher",
			filter:    `{namespace="prod"}`,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name:      "multiple matchers",
			filter:    `{namespace="prod", job="api"}`,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "regex matcher",
			filter:    `{job=~"api.*"}`,
			wantCount: 1,
			wantErr:   false,
		},
		{
			name:      "not equal with positive matcher",
			filter:    `{namespace="prod", job!="test"}`,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:      "not regex with positive matcher",
			filter:    `{namespace="prod", job!~"test.*"}`,
			wantCount: 2,
			wantErr:   false,
		},
		{
			name:    "invalid filter - no braces",
			filter:  "namespace=prod",
			wantErr: true,
		},
		{
			name:    "invalid filter - bad syntax",
			filter:  `{namespace=}`,
			wantErr: true,
		},
		{
			name:      "whitespace around filter",
			filter:    `  {namespace="prod"}  `,
			wantCount: 1,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matchers, err := ParseFilter(tt.filter)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseFilter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && len(matchers) != tt.wantCount {
				t.Errorf("ParseFilter() got %d matchers, want %d", len(matchers), tt.wantCount)
			}
		})
	}
}

func TestMergeQueryWithFilter(t *testing.T) {
	// Create filter matchers
	filterMatchers := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, "namespace", "prod"),
	}

	tests := []struct {
		name    string
		query   string
		want    string
		wantErr bool
	}{
		{
			name:  "simple stream selector",
			query: `{job="api"}`,
			want:  `{namespace="prod", job="api"}`,
		},
		{
			name:  "stream selector with line filter",
			query: `{job="api"} |= "error"`,
			want:  `{namespace="prod", job="api"} |= "error"`,
		},
		{
			name:  "stream selector with json parser",
			query: `{job="api"} | json`,
			want:  `{namespace="prod", job="api"} | json`,
		},
		{
			name:  "stream selector with multiple pipeline stages",
			query: `{job="api"} |= "error" | json | level="error"`,
			want:  `{namespace="prod", job="api"} |= "error" | json | level="error"`,
		},
		{
			name:  "rate query",
			query: `rate({job="api"}[5m])`,
			want:  `rate({namespace="prod", job="api"}[5m])`,
		},
		{
			name:  "sum rate query",
			query: `sum(rate({job="api"}[5m]))`,
			want:  `sum(rate({namespace="prod", job="api"}[5m]))`,
		},
		{
			name:  "sum by rate query",
			query: `sum by (level) (rate({job="api"}[5m]))`,
			want:  `sum by (level)(rate({namespace="prod", job="api"}[5m]))`,
		},
		{
			name:  "count_over_time query",
			query: `count_over_time({job="api"}[1h])`,
			want:  `count_over_time({namespace="prod", job="api"}[1h])`,
		},
		{
			name:    "empty query",
			query:   "",
			wantErr: true,
		},
		{
			name:    "invalid query",
			query:   "not a valid query",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MergeQueryWithFilter(tt.query, filterMatchers)
			if (err != nil) != tt.wantErr {
				t.Errorf("MergeQueryWithFilter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("MergeQueryWithFilter() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMergeQueryWithFilter_NoFilter(t *testing.T) {
	query := `{job="api"} |= "error"`
	got, err := MergeQueryWithFilter(query, nil)
	if err != nil {
		t.Errorf("MergeQueryWithFilter() unexpected error: %v", err)
		return
	}
	if got != query {
		t.Errorf("MergeQueryWithFilter() with no filter should return original query, got %q, want %q", got, query)
	}
}

func TestMergeQueryWithFilter_MultipleFilterMatchers(t *testing.T) {
	filterMatchers := []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, "namespace", "prod"),
		labels.MustNewMatcher(labels.MatchEqual, "env", "us-west-2"),
	}

	query := `{job="api"}`
	got, err := MergeQueryWithFilter(query, filterMatchers)
	if err != nil {
		t.Errorf("MergeQueryWithFilter() unexpected error: %v", err)
		return
	}

	expected := `{namespace="prod", env="us-west-2", job="api"}`
	if got != expected {
		t.Errorf("MergeQueryWithFilter() = %q, want %q", got, expected)
	}
}

func TestBuildFilterSelector(t *testing.T) {
	tests := []struct {
		name     string
		matchers []*labels.Matcher
		want     string
	}{
		{
			name:     "empty matchers",
			matchers: nil,
			want:     "",
		},
		{
			name: "single matcher",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "namespace", "prod"),
			},
			want: `{namespace="prod"}`,
		},
		{
			name: "multiple matchers",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchEqual, "namespace", "prod"),
				labels.MustNewMatcher(labels.MatchEqual, "job", "api"),
			},
			want: `{namespace="prod", job="api"}`,
		},
		{
			name: "regex matcher",
			matchers: []*labels.Matcher{
				labels.MustNewMatcher(labels.MatchRegexp, "job", "api.*"),
			},
			want: `{job=~"api.*"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildFilterSelector(tt.matchers)
			if got != tt.want {
				t.Errorf("BuildFilterSelector() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFilterConfig(t *testing.T) {
	// Reset config before and after test
	ResetFilterConfig()
	defer ResetFilterConfig()

	// Test that GetFilterConfig returns nil initially
	if cfg := GetFilterConfig(); cfg != nil {
		t.Error("GetFilterConfig() should return nil before initialization")
	}

	// Test initialization with valid filter
	err := InitializeFilterConfig(`{namespace="prod"}`)
	if err != nil {
		t.Errorf("InitializeFilterConfig() unexpected error: %v", err)
	}

	cfg := GetFilterConfig()
	if cfg == nil {
		t.Fatal("GetFilterConfig() should not return nil after initialization")
	}

	if cfg.RawFilter != `{namespace="prod"}` {
		t.Errorf("FilterConfig.RawFilter = %q, want %q", cfg.RawFilter, `{namespace="prod"}`)
	}

	if len(cfg.Matchers) != 1 {
		t.Errorf("FilterConfig.Matchers length = %d, want 1", len(cfg.Matchers))
	}
}

func TestFilterConfig_EmptyFilter(t *testing.T) {
	ResetFilterConfig()
	defer ResetFilterConfig()

	err := InitializeFilterConfig("")
	if err != nil {
		t.Errorf("InitializeFilterConfig() with empty filter should not error: %v", err)
	}

	if cfg := GetFilterConfig(); cfg != nil {
		t.Error("GetFilterConfig() should return nil after initializing with empty filter")
	}
}

func TestFilterConfig_InvalidFilter(t *testing.T) {
	ResetFilterConfig()
	defer ResetFilterConfig()

	err := InitializeFilterConfig("invalid filter")
	if err == nil {
		t.Error("InitializeFilterConfig() with invalid filter should return error")
	}
}
