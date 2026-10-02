// Package compat implements a test runner for Elasticsearch YAML compatibility tests.
package compat

import (
	"strconv"
	"strings"
)

// GetPath navigates a dot-separated path through nested JSON-decoded data.
// Integer segments index into []any slices; string segments index into
// map[string]any maps. A leading "(local)." prefix is stripped: it denotes the
// local response root. A segment that is exactly $name is resolved from vars:
// as a value when it is the final segment, otherwise as a key.
// Returns (value, true) on success, (nil, false) if any segment is missing or mistyped.
func GetPath(data any, path string, vars map[string]any) (any, bool) {
	path = strings.TrimPrefix(path, "(local).")

	if path == "" {
		return data, true
	}

	segments := strings.Split(path, ".")
	current := data

	for idx, seg := range segments {
		if val, isStash := stashValue(seg, vars); isStash {
			if idx == len(segments)-1 {
				return val, true
			}

			next, ok := stashNavigation(current, val)
			if !ok {
				return nil, false
			}

			current = next

			continue
		}

		next, ok := descend(current, seg)
		if !ok {
			return nil, false
		}

		current = next
	}

	return current, true
}

// stashValue resolves a segment that is exactly $name, returning the stashed
// value and whether the segment was a stash reference.
func stashValue(segment string, vars map[string]any) (any, bool) {
	name, isStash := stashName(segment)
	if !isStash || vars == nil {
		return nil, false
	}

	val, found := vars[name]

	return val, found
}

// stashNavigation continues navigation using a stash value: a string is a map
// key, anything else replaces the current node.
func stashNavigation(current, val any) (any, bool) {
	key, isString := val.(string)
	if !isString {
		return val, true
	}

	node, ok := current.(map[string]any)
	if !ok {
		return nil, false
	}

	next, ok := node[key]

	return next, ok
}

func descend(data any, segment string) (any, bool) {
	switch node := data.(type) {
	case map[string]any:
		val, ok := node[segment]

		return val, ok
	case []any:
		idx, err := strconv.Atoi(segment)
		if err != nil || idx < 0 || idx >= len(node) {
			return nil, false
		}

		return node[idx], true
	default:
		return nil, false
	}
}

func stashName(segment string) (string, bool) {
	if len(segment) < 2 || segment[0] != '$' {
		return "", false
	}

	for i := 1; i < len(segment); i++ {
		c := segment[i]
		if c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (i > 1 && c >= '0' && c <= '9') {
			continue
		}

		return "", false
	}

	return segment[1:], true
}
