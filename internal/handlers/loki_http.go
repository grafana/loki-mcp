package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/grafana/loki/v3/pkg/loghttp"
)

// LokiHTTPClient provides HTTP access to Loki API endpoints that support query filtering.
type LokiHTTPClient struct {
	baseURL     string
	username    string
	password    string
	bearerToken string
	orgID       string
	httpClient  *http.Client
}

// NewLokiHTTPClient creates a new Loki HTTP client.
func NewLokiHTTPClient(params *LokiParams) *LokiHTTPClient {
	return &LokiHTTPClient{
		baseURL:     params.URL,
		username:    params.Username,
		password:    params.Password,
		bearerToken: params.Token,
		orgID:       params.Org,
		httpClient: &http.Client{
			Timeout: DefaultHTTPTimeout,
		},
	}
}

// ListLabelNamesWithQuery fetches label names from Loki with an optional query filter.
func (c *LokiHTTPClient) ListLabelNamesWithQuery(query string, start, end time.Time) (*loghttp.LabelResponse, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	u.Path = "/loki/api/v1/labels"

	q := u.Query()
	q.Set("start", fmt.Sprintf("%d", start.UnixNano()))
	q.Set("end", fmt.Sprintf("%d", end.UnixNano()))
	if query != "" {
		q.Set("query", query)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.addAuthHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result loghttp.LabelResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// ListLabelValuesWithQuery fetches values for a specific label with an optional query filter.
func (c *LokiHTTPClient) ListLabelValuesWithQuery(labelName, query string, start, end time.Time) (*loghttp.LabelResponse, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	u.Path = fmt.Sprintf("/loki/api/v1/label/%s/values", url.PathEscape(labelName))

	q := u.Query()
	q.Set("start", fmt.Sprintf("%d", start.UnixNano()))
	q.Set("end", fmt.Sprintf("%d", end.UnixNano()))
	if query != "" {
		q.Set("query", query)
	}
	u.RawQuery = q.Encode()

	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.addAuthHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result loghttp.LabelResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// addAuthHeaders adds authentication headers to the request.
func (c *LokiHTTPClient) addAuthHeaders(req *http.Request) {
	if c.username != "" && c.password != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	}
	if c.orgID != "" {
		req.Header.Set("X-Scope-OrgID", c.orgID)
	}
}
