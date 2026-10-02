package compat

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"regexp"
	"strings"
	"time"
)

var (
	errPathNotFound  = errors.New("path not found in response")
	errTypeMismatch  = errors.New("type mismatch")
	errValueMismatch = errors.New("value mismatch")
	errUnknownAssert = errors.New("unknown assertion type")
	errNotNumeric    = errors.New("value is not numeric")
	errNotCollection = errors.New("value is not a slice or map")
)

// Assertion types from the ES client test format.
const (
	AssertMatch             = "match"
	AssertIsTrue            = "is_true"
	AssertIsFalse           = "is_false"
	AssertLength            = "length"
	AssertGTE               = "gte"
	AssertLTE               = "lte"
	AssertGT                = "gt"
	AssertLT                = "lt"
	AssertExists            = "exists"
	AssertContains          = "contains"
	AssertIsAfter           = "is_after"
	AssertCloseTo           = "close_to"
	AssertMatchesInAnyOrder = "matches_in_any_order"
)

// CheckAssertion evaluates a single assertion against the response body,
// resolving $name stash references from vars in both paths and expected values.
func CheckAssertion(response any, assertion Assertion, vars map[string]any) error {
	switch assertion.Type {
	case AssertMatch:
		return checkMatch(response, assertion.Value, vars)
	case AssertIsTrue:
		return checkIsTrue(response, assertion.Value, vars)
	case AssertIsFalse:
		return checkIsFalse(response, assertion.Value, vars)
	case AssertLength:
		return checkLength(response, assertion.Value, vars)
	case AssertGTE:
		return checkComparison(response, assertion.Value, vars, func(got, want float64) bool { return got >= want })
	case AssertLTE:
		return checkComparison(response, assertion.Value, vars, func(got, want float64) bool { return got <= want })
	case AssertGT:
		return checkComparison(response, assertion.Value, vars, func(got, want float64) bool { return got > want })
	case AssertLT:
		return checkComparison(response, assertion.Value, vars, func(got, want float64) bool { return got < want })
	case AssertIsAfter:
		return checkIsAfter(response, assertion.Value, vars)
	case AssertCloseTo:
		return checkCloseTo(response, assertion.Value, vars)
	case AssertMatchesInAnyOrder:
		return checkMatchesInAnyOrder(response, assertion.Value, vars)
	case AssertExists:
		return checkExists(response, assertion.Value, vars)
	case AssertContains:
		return checkContains(response, assertion.Value, vars)
	default:
		return fmt.Errorf("%w: %q", errUnknownAssert, assertion.Type)
	}
}

// regexPattern reports whether value is a /pattern/flags regex literal and
// returns the pattern compiled with the Java-style flags mapped to Go syntax.
func regexPattern(value string) (string, bool) {
	if len(value) < 2 || value[0] != '/' {
		return "", false
	}

	end := strings.LastIndex(value, "/")
	if end == 0 {
		return "", false
	}

	pattern := value[1:end]
	flags := value[end+1:]

	if flags == "" {
		return pattern, true
	}

	var mapped strings.Builder

	mapped.WriteString("(?")

	for _, flag := range flags {
		switch flag {
		case 'i', 'I':
			mapped.WriteByte('i')
		case 'm', 'M':
			mapped.WriteByte('m')
		case 's', 'S':
			mapped.WriteByte('s')
		case 'x', 'X':
			mapped.WriteByte('x')
		case 'U', 'u':
			mapped.WriteByte('U')
		}
	}

	mapped.WriteByte(')')

	return mapped.String() + pattern, true
}

func checkMatch(response, value any, vars map[string]any) error {
	pairs, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("match value must be a map: %w", errTypeMismatch)
	}

	for dotPath, want := range pairs {
		got, found := GetPath(response, dotPath, vars)
		if !found {
			return fmt.Errorf("match %q: %w", dotPath, errPathNotFound)
		}

		err := matchValue(got, substituteValue(want, vars))
		if err != nil {
			return fmt.Errorf("match %q: %w", dotPath, err)
		}
	}

	return nil
}

func matchValue(got, want any) error {
	if isRegex, err := matchRegex(got, want); isRegex {
		return err
	}

	if !reflect.DeepEqual(normalizeNumeric(got), normalizeNumeric(want)) {
		return fmt.Errorf("got %v, want %v: %w", got, want, errValueMismatch)
	}

	return nil
}

// matchRegex compares got against want when want is a /pattern/ regex literal,
// reporting whether want was a regex at all.
func matchRegex(got, want any) (bool, error) {
	wantStr, ok := want.(string)
	if !ok {
		return false, nil
	}

	pattern, isRegex := regexPattern(wantStr)
	if !isRegex {
		return false, nil
	}

	matched, err := regexp.MatchString(pattern, fmt.Sprint(got))
	if err != nil {
		return true, fmt.Errorf("invalid regex %q: %w", wantStr, err)
	}

	if !matched {
		return true, fmt.Errorf("got %v, want regex %q: %w", got, wantStr, errValueMismatch)
	}

	return true, nil
}

func checkIsTrue(response, value any, vars map[string]any) error {
	dotPath, ok := value.(string)
	if !ok {
		return fmt.Errorf("is_true value must be a string path: %w", errTypeMismatch)
	}

	got, found := GetPath(response, dotPath, vars)
	if !found {
		return fmt.Errorf("is_true %q: %w", dotPath, errPathNotFound)
	}

	if !isTruthy(got) {
		return fmt.Errorf("is_true %q: value %v is falsy: %w", dotPath, got, errValueMismatch)
	}

	return nil
}

func checkIsFalse(response, value any, vars map[string]any) error {
	dotPath, ok := value.(string)
	if !ok {
		return fmt.Errorf("is_false value must be a string path: %w", errTypeMismatch)
	}

	got, found := GetPath(response, dotPath, vars)
	if !found {
		return nil // missing path is falsy — assertion passes
	}

	if isTruthy(got) {
		return fmt.Errorf("is_false %q: value %v is truthy: %w", dotPath, got, errValueMismatch)
	}

	return nil
}

func checkLength(response, value any, vars map[string]any) error {
	pairs, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("length value must be a map: %w", errTypeMismatch)
	}

	for dotPath, wantRaw := range pairs {
		got, found := GetPath(response, dotPath, vars)
		if !found {
			return fmt.Errorf("length %q: %w", dotPath, errPathNotFound)
		}

		wantLen, err := toFloat64(substituteValue(wantRaw, vars))
		if err != nil {
			return fmt.Errorf("length %q expected: %w", dotPath, err)
		}

		gotLen, err := collectionLen(got)
		if err != nil {
			return fmt.Errorf("length %q: %w", dotPath, err)
		}

		if float64(gotLen) != wantLen {
			return fmt.Errorf("length %q: got %d, want %g: %w", dotPath, gotLen, wantLen, errValueMismatch)
		}
	}

	return nil
}

func checkComparison(response, value any, vars map[string]any, cmp func(got, want float64) bool) error {
	pairs, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("comparison value must be a map: %w", errTypeMismatch)
	}

	for dotPath, wantRaw := range pairs {
		got, found := GetPath(response, dotPath, vars)
		if !found {
			return fmt.Errorf("comparison %q: %w", dotPath, errPathNotFound)
		}

		gotF, err := toFloat64(got)
		if err != nil {
			return fmt.Errorf("comparison %q got: %w", dotPath, err)
		}

		wantF, err := toFloat64(substituteValue(wantRaw, vars))
		if err != nil {
			return fmt.Errorf("comparison %q want: %w", dotPath, err)
		}

		if !cmp(gotF, wantF) {
			return fmt.Errorf("comparison %q: got %g vs %g: %w", dotPath, gotF, wantF, errValueMismatch)
		}
	}

	return nil
}

func checkIsAfter(response, value any, vars map[string]any) error {
	pairs, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("is_after value must be a map: %w", errTypeMismatch)
	}

	for dotPath, wantRaw := range pairs {
		got, found := GetPath(response, dotPath, vars)
		if !found {
			return fmt.Errorf("is_after %q: %w", dotPath, errPathNotFound)
		}

		gotTime, err := parseTime(got)
		if err != nil {
			return fmt.Errorf("is_after %q got: %w", dotPath, err)
		}

		wantTime, err := parseTime(substituteValue(wantRaw, vars))
		if err != nil {
			return fmt.Errorf("is_after %q want: %w", dotPath, err)
		}

		if !gotTime.After(wantTime) {
			return fmt.Errorf("is_after %q: %v is not after %v: %w", dotPath, got, wantRaw, errValueMismatch)
		}
	}

	return nil
}

func parseTime(value any) (time.Time, error) {
	if t, ok := value.(time.Time); ok {
		return t, nil
	}

	text, ok := value.(string)
	if !ok {
		return time.Time{}, fmt.Errorf("%w: %T is not a timestamp", errTypeMismatch, value)
	}

	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z0700", "2006-01-02"} {
		parsed, err := time.Parse(layout, text)
		if err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, fmt.Errorf("%w: unparseable timestamp %q", errTypeMismatch, text)
}

func checkCloseTo(response, value any, vars map[string]any) error {
	pairs, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("close_to value must be a map: %w", errTypeMismatch)
	}

	for dotPath, specRaw := range pairs {
		spec, ok := substituteValue(specRaw, vars).(map[string]any)
		if !ok {
			return fmt.Errorf("close_to %q spec must be a map: %w", dotPath, errTypeMismatch)
		}

		got, found := GetPath(response, dotPath, vars)
		if !found {
			return fmt.Errorf("close_to %q: %w", dotPath, errPathNotFound)
		}

		gotF, err := toFloat64(got)
		if err != nil {
			return fmt.Errorf("close_to %q got: %w", dotPath, err)
		}

		wantF, err := toFloat64(spec["value"])
		if err != nil {
			return fmt.Errorf("close_to %q value: %w", dotPath, err)
		}

		errF, err := toFloat64(spec["error"])
		if err != nil {
			return fmt.Errorf("close_to %q error: %w", dotPath, err)
		}

		if math.Abs(gotF-wantF) > errF {
			return fmt.Errorf("close_to %q: %g not within %g of %g: %w", dotPath, gotF, errF, wantF, errValueMismatch)
		}
	}

	return nil
}

func checkMatchesInAnyOrder(response, value any, vars map[string]any) error {
	pairs, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("matches_in_any_order value must be a map: %w", errTypeMismatch)
	}

	for dotPath, wantRaw := range pairs {
		got, found := GetPath(response, dotPath, vars)
		if !found {
			return fmt.Errorf("matches_in_any_order %q: %w", dotPath, errPathNotFound)
		}

		gotList, ok := got.([]any)
		if !ok {
			return fmt.Errorf("matches_in_any_order %q: %w", dotPath, errTypeMismatch)
		}

		wantList, ok := substituteValue(wantRaw, vars).([]any)
		if !ok {
			return fmt.Errorf("matches_in_any_order %q want must be a list: %w", dotPath, errTypeMismatch)
		}

		if !sameElementsUnordered(gotList, wantList) {
			return fmt.Errorf("matches_in_any_order %q: %v does not match %v: %w", dotPath, got, wantRaw, errValueMismatch)
		}
	}

	return nil
}

// sameElementsUnordered reports whether both lists hold the same elements
// (with duplicates) under the match comparison rules.
func sameElementsUnordered(got, want []any) bool {
	if len(got) != len(want) {
		return false
	}

	used := make([]bool, len(got))

	for _, wantElem := range want {
		matched := false

		for idx, gotElem := range got {
			if used[idx] {
				continue
			}

			if matchValue(gotElem, wantElem) == nil {
				used[idx] = true
				matched = true

				break
			}
		}

		if !matched {
			return false
		}
	}

	return true
}

func checkExists(response, value any, vars map[string]any) error {
	dotPath, ok := value.(string)
	if !ok {
		return fmt.Errorf("exists value must be a string path: %w", errTypeMismatch)
	}

	if _, found := GetPath(response, dotPath, vars); !found {
		return fmt.Errorf("exists %q: %w", dotPath, errPathNotFound)
	}

	return nil
}

func checkContains(response, value any, vars map[string]any) error {
	pairs, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("contains value must be a map: %w", errTypeMismatch)
	}

	for dotPath, want := range pairs {
		got, found := GetPath(response, dotPath, vars)
		if !found {
			return fmt.Errorf("contains %q: %w", dotPath, errPathNotFound)
		}

		if !containsValue(got, substituteValue(want, vars)) {
			return fmt.Errorf("contains %q: %v does not contain %v: %w", dotPath, got, want, errValueMismatch)
		}
	}

	return nil
}

// containsValue reports whether got contains want: a slice contains want if any
// element does, a map contains a map if want is a subset, and a string contains
// a substring. Scalars fall back to deep equality.
func containsValue(got, want any) bool {
	switch typed := got.(type) {
	case []any:
		for _, elem := range typed {
			if containsValue(elem, want) {
				return true
			}
		}

		return false
	case map[string]any:
		wantMap, ok := want.(map[string]any)
		if !ok {
			return false
		}

		return containsSubset(typed, wantMap)
	case string:
		wantStr, ok := want.(string)

		return ok && strings.Contains(typed, wantStr)
	default:
		return reflect.DeepEqual(normalizeNumeric(got), normalizeNumeric(want))
	}
}

func containsSubset(got, want map[string]any) bool {
	for key, wantVal := range want {
		gotVal, ok := got[key]
		if !ok || !containsValue(gotVal, wantVal) {
			return false
		}
	}

	return true
}

func isTruthy(val any) bool {
	if val == nil {
		return false
	}

	switch typed := val.(type) {
	case bool:
		return typed
	case string:
		return typed != ""
	case float64:
		return typed != 0
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case []any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return true
	}
}

func normalizeNumeric(val any) any {
	switch typed := val.(type) {
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case float32:
		return float64(typed)
	default:
		return val
	}
}

func toFloat64(val any) (float64, error) {
	switch typed := val.(type) {
	case float64:
		return typed, nil
	case float32:
		return float64(typed), nil
	case int:
		return float64(typed), nil
	case int64:
		return float64(typed), nil
	default:
		return 0, fmt.Errorf("%w: got %T", errNotNumeric, val)
	}
}

func collectionLen(val any) (int, error) {
	switch typed := val.(type) {
	case []any:
		return len(typed), nil
	case map[string]any:
		return len(typed), nil
	case string:
		return len(typed), nil
	default:
		return 0, fmt.Errorf("%w: got %T", errNotCollection, val)
	}
}
