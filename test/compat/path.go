// Package compat implements a test runner for Elasticsearch YAML compatibility tests.
package compat

import (
	"strconv"
	"strings"
)

// GetPath navigates a dot-separated path through nested JSON-decoded data.
// Integer segments index into []any slices; string segments index into map[string]any maps.
// Returns (value, true) on success, (nil, false) if any segment is missing or type mismatch.
func GetPath(data any, path string) (any, bool) {
	if path == "" {
		return data, true
	}

	current := data

	for seg := range strings.SplitSeq(path, ".") {
		switch node := current.(type) {
		case map[string]any:
			val, ok := node[seg]
			if !ok {
				return nil, false
			}

			current = val
		case []any:
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil, false
			}

			current = node[idx]
		default:
			return nil, false
		}
	}

	return current, true
}
