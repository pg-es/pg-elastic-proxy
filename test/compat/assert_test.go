package compat_test

import (
	"testing"

	"github.com/pg-es/pg-es-proxy/test/compat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssertMatch(t *testing.T) {
	t.Parallel()

	t.Run("pass", func(t *testing.T) {
		t.Parallel()

		response := map[string]any{
			"results": map[string]any{
				"count": map[string]any{"qty": float64(1)},
			},
		}

		assertion := compat.Assertion{
			Type:  assertMatch,
			Value: map[string]any{"results.count.qty": float64(1)},
		}

		err := compat.CheckAssertion(response, assertion)
		require.NoError(t, err)
	})

	t.Run("fail", func(t *testing.T) {
		t.Parallel()

		response := map[string]any{"amount": float64(5)}

		assertion := compat.Assertion{
			Type:  assertMatch,
			Value: map[string]any{"amount": float64(10)},
		}

		err := compat.CheckAssertion(response, assertion)
		assert.Error(t, err)
	})

	t.Run("int vs float", func(t *testing.T) {
		t.Parallel()

		response := map[string]any{"quantity": float64(42)}

		assertion := compat.Assertion{
			Type:  assertMatch,
			Value: map[string]any{"quantity": 42},
		}

		err := compat.CheckAssertion(response, assertion)
		require.NoError(t, err, "int from YAML should match float64 from JSON")
	})
}

func TestAssertIsTrue(t *testing.T) {
	t.Parallel()

	t.Run("pass", func(t *testing.T) {
		t.Parallel()

		response := map[string]any{"data": map[string]any{"heading": "hello"}}

		assertion := compat.Assertion{Type: assertIsTrue, Value: "data.heading"}

		err := compat.CheckAssertion(response, assertion)
		require.NoError(t, err)
	})

	t.Run("falsy bool", func(t *testing.T) {
		t.Parallel()

		response := map[string]any{"flag": false}

		assertion := compat.Assertion{Type: assertIsTrue, Value: "flag"}

		err := compat.CheckAssertion(response, assertion)
		assert.Error(t, err)
	})

	t.Run("nil", func(t *testing.T) {
		t.Parallel()

		response := map[string]any{"empty": nil}

		assertion := compat.Assertion{Type: assertIsTrue, Value: "empty"}

		err := compat.CheckAssertion(response, assertion)
		assert.Error(t, err)
	})

	t.Run("zero number", func(t *testing.T) {
		t.Parallel()

		response := map[string]any{"rank": float64(0)}

		assertion := compat.Assertion{Type: assertIsTrue, Value: "rank"}

		err := compat.CheckAssertion(response, assertion)
		assert.Error(t, err)
	})

	t.Run("empty string", func(t *testing.T) {
		t.Parallel()

		response := map[string]any{"caption": ""}

		assertion := compat.Assertion{Type: assertIsTrue, Value: "caption"}

		err := compat.CheckAssertion(response, assertion)
		assert.Error(t, err)
	})

	t.Run("non-empty slice", func(t *testing.T) {
		t.Parallel()

		response := map[string]any{"entries": []any{"one"}}

		assertion := compat.Assertion{Type: assertIsTrue, Value: "entries"}

		err := compat.CheckAssertion(response, assertion)
		require.NoError(t, err)
	})
}

func TestAssertIsFalse(t *testing.T) {
	t.Parallel()

	t.Run("pass", func(t *testing.T) {
		t.Parallel()

		response := map[string]any{"disabled": false}

		assertion := compat.Assertion{Type: assertIsFalse, Value: "disabled"}

		err := compat.CheckAssertion(response, assertion)
		require.NoError(t, err)
	})

	t.Run("missing path", func(t *testing.T) {
		t.Parallel()

		response := map[string]any{"other": "present"}

		assertion := compat.Assertion{Type: assertIsFalse, Value: "nonexistent"}

		err := compat.CheckAssertion(response, assertion)
		require.NoError(t, err)
	})

	t.Run("truthy value", func(t *testing.T) {
		t.Parallel()

		response := map[string]any{"active": true}

		assertion := compat.Assertion{Type: assertIsFalse, Value: "active"}

		err := compat.CheckAssertion(response, assertion)
		assert.Error(t, err)
	})
}

func TestAssertLength(t *testing.T) {
	t.Parallel()

	t.Run("pass", func(t *testing.T) {
		t.Parallel()

		response := map[string]any{
			"records": map[string]any{
				"rows": []any{"a", "b", "c"},
			},
		}

		assertion := compat.Assertion{
			Type:  assertLength,
			Value: map[string]any{"records.rows": float64(3)},
		}

		err := compat.CheckAssertion(response, assertion)
		require.NoError(t, err)
	})

	t.Run("fail", func(t *testing.T) {
		t.Parallel()

		response := map[string]any{"tags": []any{"go"}}

		assertion := compat.Assertion{
			Type:  assertLength,
			Value: map[string]any{"tags": float64(5)},
		}

		err := compat.CheckAssertion(response, assertion)
		assert.Error(t, err)
	})
}

func TestAssertComparison(t *testing.T) {
	t.Parallel()

	t.Run("gte pass", func(t *testing.T) {
		t.Parallel()

		response := map[string]any{"latency": float64(10)}

		assertion := compat.Assertion{
			Type:  assertGTE,
			Value: map[string]any{"latency": float64(0)},
		}

		err := compat.CheckAssertion(response, assertion)
		require.NoError(t, err)
	})

	t.Run("gte fail", func(t *testing.T) {
		t.Parallel()

		response := map[string]any{"delay": float64(3)}

		assertion := compat.Assertion{
			Type:  assertGTE,
			Value: map[string]any{"delay": float64(100)},
		}

		err := compat.CheckAssertion(response, assertion)
		assert.Error(t, err)
	})

	t.Run("lte pass", func(t *testing.T) {
		t.Parallel()

		response := map[string]any{"weight": float64(50)}

		assertion := compat.Assertion{
			Type:  assertLTE,
			Value: map[string]any{"weight": float64(1000)},
		}

		err := compat.CheckAssertion(response, assertion)
		require.NoError(t, err)
	})

	t.Run("lte fail", func(t *testing.T) {
		t.Parallel()

		response := map[string]any{"size": float64(999)}

		assertion := compat.Assertion{
			Type:  assertLTE,
			Value: map[string]any{"size": float64(10)},
		}

		err := compat.CheckAssertion(response, assertion)
		assert.Error(t, err)
	})
}
