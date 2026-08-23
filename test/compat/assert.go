package compat

import (
	"errors"
	"fmt"
	"reflect"
)

var (
	errPathNotFound  = errors.New("path not found in response")
	errTypeMismatch  = errors.New("type mismatch")
	errValueMismatch = errors.New("value mismatch")
	errUnknownAssert = errors.New("unknown assertion type")
	errNotNumeric    = errors.New("value is not numeric")
	errNotCollection = errors.New("value is not a slice or map")
)

// CheckAssertion evaluates a single assertion against the response body.
func CheckAssertion(response map[string]any, assertion Assertion) error {
	switch assertion.Type {
	case "match":
		return checkMatch(response, assertion.Value)
	case "is_true":
		return checkIsTrue(response, assertion.Value)
	case "is_false":
		return checkIsFalse(response, assertion.Value)
	case "length":
		return checkLength(response, assertion.Value)
	case "gte":
		return checkComparison(response, assertion.Value, func(got, want float64) bool { return got >= want })
	case "lte":
		return checkComparison(response, assertion.Value, func(got, want float64) bool { return got <= want })
	default:
		return fmt.Errorf("%w: %q", errUnknownAssert, assertion.Type)
	}
}

func checkMatch(response map[string]any, value any) error {
	pairs, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("match value must be a map: %w", errTypeMismatch)
	}

	for dotPath, want := range pairs {
		got, found := GetPath(response, dotPath)
		if !found {
			return fmt.Errorf("match %q: %w", dotPath, errPathNotFound)
		}

		if !reflect.DeepEqual(normalizeNumeric(got), normalizeNumeric(want)) {
			return fmt.Errorf("match %q: got %v, want %v: %w", dotPath, got, want, errValueMismatch)
		}
	}

	return nil
}

func checkIsTrue(response map[string]any, value any) error {
	dotPath, ok := value.(string)
	if !ok {
		return fmt.Errorf("is_true value must be a string path: %w", errTypeMismatch)
	}

	got, found := GetPath(response, dotPath)
	if !found {
		return fmt.Errorf("is_true %q: %w", dotPath, errPathNotFound)
	}

	if !isTruthy(got) {
		return fmt.Errorf("is_true %q: value %v is falsy: %w", dotPath, got, errValueMismatch)
	}

	return nil
}

func checkIsFalse(response map[string]any, value any) error {
	dotPath, ok := value.(string)
	if !ok {
		return fmt.Errorf("is_false value must be a string path: %w", errTypeMismatch)
	}

	got, found := GetPath(response, dotPath)
	if !found {
		return nil // missing path is falsy — assertion passes
	}

	if isTruthy(got) {
		return fmt.Errorf("is_false %q: value %v is truthy: %w", dotPath, got, errValueMismatch)
	}

	return nil
}

func checkLength(response map[string]any, value any) error {
	pairs, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("length value must be a map: %w", errTypeMismatch)
	}

	for dotPath, wantRaw := range pairs {
		got, found := GetPath(response, dotPath)
		if !found {
			return fmt.Errorf("length %q: %w", dotPath, errPathNotFound)
		}

		wantLen, err := toFloat64(wantRaw)
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

func checkComparison(response map[string]any, value any, cmp func(got, want float64) bool) error {
	pairs, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("comparison value must be a map: %w", errTypeMismatch)
	}

	for dotPath, wantRaw := range pairs {
		got, found := GetPath(response, dotPath)
		if !found {
			return fmt.Errorf("comparison %q: %w", dotPath, errPathNotFound)
		}

		gotF, err := toFloat64(got)
		if err != nil {
			return fmt.Errorf("comparison %q got: %w", dotPath, err)
		}

		wantF, err := toFloat64(wantRaw)
		if err != nil {
			return fmt.Errorf("comparison %q want: %w", dotPath, err)
		}

		if !cmp(gotF, wantF) {
			return fmt.Errorf("comparison %q: got %g vs %g: %w", dotPath, gotF, wantF, errValueMismatch)
		}
	}

	return nil
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
