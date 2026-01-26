package handlers

import (
	"fmt"
	"strings"

	"github.com/grafana/loki/v3/pkg/logql/syntax"
	"github.com/prometheus/prometheus/model/labels"
)

// ParseFilter parses a LogQL stream selector and returns its matchers.
// The filter must be a valid LogQL stream selector like {namespace="prod", job="api"}.
func ParseFilter(filter string) ([]*labels.Matcher, error) {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return nil, nil
	}

	// Parse as a log selector (stream selector + optional pipeline)
	expr, err := syntax.ParseLogSelector(filter, true)
	if err != nil {
		return nil, fmt.Errorf("invalid LogQL stream selector %q: %w", filter, err)
	}

	return expr.Matchers(), nil
}

// MergeQueryWithFilter injects filter matchers into a LogQL query.
// The filter matchers are prepended to the query's stream selector,
// effectively ANDing the filter with the original query.
func MergeQueryWithFilter(query string, filterMatchers []*labels.Matcher) (string, error) {
	if len(filterMatchers) == 0 {
		return query, nil
	}

	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("query cannot be empty")
	}

	// Try to parse as a log query first (most common case)
	expr, err := syntax.ParseExpr(query)
	if err != nil {
		return "", fmt.Errorf("failed to parse query %q: %w", query, err)
	}

	// Walk the expression tree and inject filter matchers into stream selectors
	merged, err := injectMatchers(expr, filterMatchers)
	if err != nil {
		return "", err
	}

	return merged.String(), nil
}

// injectMatchers walks the expression tree and injects filter matchers into all stream selectors.
func injectMatchers(expr syntax.Expr, filterMatchers []*labels.Matcher) (syntax.Expr, error) {
	switch e := expr.(type) {
	case *syntax.MatchersExpr:
		// Simple stream selector like {job="api"}
		return mergeMatchersExpr(e, filterMatchers), nil

	case *syntax.PipelineExpr:
		// Log query with pipeline like {job="api"} |= "error" | json
		// PipelineExpr.Left is *MatchersExpr
		newLeft := mergeMatchersExpr(e.Left, filterMatchers)
		return &syntax.PipelineExpr{
			Left:        newLeft,
			MultiStages: e.MultiStages,
		}, nil

	case *syntax.RangeAggregationExpr:
		// Metric queries like rate({job="api"}[5m])
		newLeft, err := injectMatchers(e.Left, filterMatchers)
		if err != nil {
			return nil, err
		}
		if logRange, ok := newLeft.(*syntax.LogRangeExpr); ok {
			return &syntax.RangeAggregationExpr{
				Left:      logRange,
				Operation: e.Operation,
				Params:    e.Params,
				Grouping:  e.Grouping,
			}, nil
		}
		return nil, fmt.Errorf("unexpected type for range aggregation: %T", newLeft)

	case *syntax.VectorAggregationExpr:
		// Aggregation like sum(rate({job="api"}[5m]))
		newLeft, err := injectMatchers(e.Left, filterMatchers)
		if err != nil {
			return nil, err
		}
		if sampleExpr, ok := newLeft.(syntax.SampleExpr); ok {
			return &syntax.VectorAggregationExpr{
				Left:      sampleExpr,
				Grouping:  e.Grouping,
				Params:    e.Params,
				Operation: e.Operation,
			}, nil
		}
		return nil, fmt.Errorf("unexpected type for vector aggregation: %T", newLeft)

	case *syntax.LogRangeExpr:
		// Log range like {job="api"}[5m]
		newLeft, err := injectMatchers(e.Left, filterMatchers)
		if err != nil {
			return nil, err
		}
		if logSelector, ok := newLeft.(syntax.LogSelectorExpr); ok {
			return &syntax.LogRangeExpr{
				Left:     logSelector,
				Interval: e.Interval,
				Offset:   e.Offset,
				Unwrap:   e.Unwrap,
			}, nil
		}
		return nil, fmt.Errorf("unexpected type for log range: %T", newLeft)

	case *syntax.BinOpExpr:
		// Binary operation like sum(rate({job="api"}[5m])) / sum(rate({job="api"}[5m]))
		newLeft, err := injectMatchers(e.SampleExpr, filterMatchers)
		if err != nil {
			return nil, err
		}
		newRight, err := injectMatchers(e.RHS, filterMatchers)
		if err != nil {
			return nil, err
		}
		leftSample, ok := newLeft.(syntax.SampleExpr)
		if !ok {
			return nil, fmt.Errorf("unexpected type for binop left: %T", newLeft)
		}
		rightSample, ok := newRight.(syntax.SampleExpr)
		if !ok {
			return nil, fmt.Errorf("unexpected type for binop right: %T", newRight)
		}
		return &syntax.BinOpExpr{
			SampleExpr: leftSample,
			RHS:        rightSample,
			Op:         e.Op,
			Opts:       e.Opts,
		}, nil

	case *syntax.LabelReplaceExpr:
		newLeft, err := injectMatchers(e.Left, filterMatchers)
		if err != nil {
			return nil, err
		}
		if sampleExpr, ok := newLeft.(syntax.SampleExpr); ok {
			return &syntax.LabelReplaceExpr{
				Left:        sampleExpr,
				Dst:         e.Dst,
				Replacement: e.Replacement,
				Src:         e.Src,
				Regex:       e.Regex,
			}, nil
		}
		return nil, fmt.Errorf("unexpected type for label_replace: %T", newLeft)

	case *syntax.LiteralExpr:
		// Literal values don't need modification
		return e, nil

	default:
		return nil, fmt.Errorf("unsupported expression type: %T", expr)
	}
}

// mergeMatchersExpr creates a new MatchersExpr with filter matchers prepended.
func mergeMatchersExpr(expr *syntax.MatchersExpr, filterMatchers []*labels.Matcher) *syntax.MatchersExpr {
	// Combine filter matchers with existing matchers
	// Filter matchers come first, then original matchers
	allMatchers := make([]*labels.Matcher, 0, len(filterMatchers)+len(expr.Mts))
	allMatchers = append(allMatchers, filterMatchers...)
	allMatchers = append(allMatchers, expr.Mts...)

	return &syntax.MatchersExpr{Mts: allMatchers}
}

// BuildFilterSelector builds a LogQL stream selector string from matchers.
// This is useful for label API calls that accept a query parameter.
func BuildFilterSelector(matchers []*labels.Matcher) string {
	if len(matchers) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteByte('{')
	for i, m := range matchers {
		if i > 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(m.String())
	}
	sb.WriteByte('}')
	return sb.String()
}
