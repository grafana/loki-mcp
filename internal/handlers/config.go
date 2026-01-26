package handlers

import (
	"sync"

	"github.com/prometheus/prometheus/model/labels"
)

// FilterConfig holds the global query filter configuration.
type FilterConfig struct {
	RawFilter string
	Matchers  []*labels.Matcher
}

var (
	filterConfig *FilterConfig
	filterMu     sync.RWMutex
)

// InitializeFilterConfig parses and stores the global query filter.
// This should be called once at startup. Returns an error if the filter is invalid.
func InitializeFilterConfig(filter string) error {
	if filter == "" {
		return nil
	}

	matchers, err := ParseFilter(filter)
	if err != nil {
		return err
	}

	filterMu.Lock()
	defer filterMu.Unlock()
	filterConfig = &FilterConfig{
		RawFilter: filter,
		Matchers:  matchers,
	}
	return nil
}

// GetFilterConfig returns the current filter configuration.
// Returns nil if no filter is configured.
func GetFilterConfig() *FilterConfig {
	filterMu.RLock()
	defer filterMu.RUnlock()
	return filterConfig
}

// ResetFilterConfig clears the filter configuration (for testing).
func ResetFilterConfig() {
	filterMu.Lock()
	defer filterMu.Unlock()
	filterConfig = nil
}
