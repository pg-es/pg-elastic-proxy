package compat_test

import (
	"testing"

	"github.com/pg-es/pg-es-proxy/test/compat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAssertMatchRegex(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		response  map[string]any
		assertion compat.Assertion
		wantErr   bool
	}{
		{
			name:     "regex matches the value",
			response: map[string]any{testMsg: "Hot threads at"},
			assertion: compat.Assertion{
				Type:  compat.AssertMatch,
				Value: map[string]any{testMsg: `/Hot\ threads\ at/`},
			},
		},
		{
			name:     "anchored regex mismatch fails",
			response: map[string]any{testMsg: "something else"},
			assertion: compat.Assertion{
				Type:  compat.AssertMatch,
				Value: map[string]any{testMsg: `/^Hot$/`},
			},
			wantErr: true,
		},
		{
			name:     "string that is not a regex literal compares exactly",
			response: map[string]any{"path": "/etc/hosts"},
			assertion: compat.Assertion{
				Type:  compat.AssertMatch,
				Value: map[string]any{"path": "/etc/hosts"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := compat.CheckAssertion(tt.response, tt.assertion, nil)

			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestAssertMatchStashValue(t *testing.T) {
	t.Parallel()

	response := map[string]any{fieldID: testDocID}
	vars := map[string]any{"id": testDocID}

	require.NoError(t, compat.CheckAssertion(response, compat.Assertion{
		Type:  compat.AssertMatch,
		Value: map[string]any{fieldID: testStashID},
	}, vars))

	assert.Error(t, compat.CheckAssertion(response, compat.Assertion{
		Type:  compat.AssertMatch,
		Value: map[string]any{fieldID: "$other"},
	}, vars))
}

func TestAssertLT(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		response  map[string]any
		assertion compat.Assertion
		wantErr   bool
	}{
		{
			name:      "lesser value passes",
			response:  map[string]any{fieldCount: float64(1)},
			assertion: compat.Assertion{Type: compat.AssertLT, Value: map[string]any{fieldCount: float64(5)}},
		},
		{
			name:      "equal value fails",
			response:  map[string]any{fieldCount: float64(5)},
			assertion: compat.Assertion{Type: compat.AssertLT, Value: map[string]any{fieldCount: float64(5)}},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := compat.CheckAssertion(tt.response, tt.assertion, nil)

			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestAssertIsAfter(t *testing.T) {
	t.Parallel()

	response := map[string]any{testCreated: "2023-05-25T12:30:01Z"}

	require.NoError(t, compat.CheckAssertion(response, compat.Assertion{
		Type:  compat.AssertIsAfter,
		Value: map[string]any{testCreated: "2023-05-25T12:30:00Z"},
	}, nil))

	assert.Error(t, compat.CheckAssertion(response, compat.Assertion{
		Type:  compat.AssertIsAfter,
		Value: map[string]any{testCreated: "2023-05-25T12:30:02Z"},
	}, nil))
}

func TestAssertCloseTo(t *testing.T) {
	t.Parallel()

	response := map[string]any{testRatio: 0.12345678}

	require.NoError(t, compat.CheckAssertion(response, compat.Assertion{
		Type: compat.AssertCloseTo,
		Value: map[string]any{
			testRatio: map[string]any{testValue: 0.12345679, testErrField: 0.000001},
		},
	}, nil))

	assert.Error(t, compat.CheckAssertion(response, compat.Assertion{
		Type: compat.AssertCloseTo,
		Value: map[string]any{
			testRatio: map[string]any{testValue: 0.2, testErrField: 0.000001},
		},
	}, nil))
}

func TestAssertMatchesInAnyOrder(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		response  map[string]any
		assertion compat.Assertion
		wantErr   bool
	}{
		{
			name:      "same scalars any order",
			response:  map[string]any{testTags: []any{"b", "a"}},
			assertion: compat.Assertion{Type: compat.AssertMatchesInAnyOrder, Value: map[string]any{testTags: []any{"a", "b"}}},
		},
		{
			name:     "objects matched by subset",
			response: map[string]any{testHits: []any{map[string]any{fieldID: "2"}, map[string]any{fieldID: "1"}}},
			assertion: compat.Assertion{
				Type:  compat.AssertMatchesInAnyOrder,
				Value: map[string]any{testHits: []any{map[string]any{fieldID: "1"}, map[string]any{fieldID: "2"}}},
			},
		},
		{
			name:      "duplicates respected",
			response:  map[string]any{testTags: []any{"a", "a"}},
			assertion: compat.Assertion{Type: compat.AssertMatchesInAnyOrder, Value: map[string]any{testTags: []any{"a", "b"}}},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := compat.CheckAssertion(tt.response, tt.assertion, nil)

			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestAssertMatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		response  map[string]any
		assertion compat.Assertion
		wantErr   bool
	}{
		{
			name: "nested value matches",
			response: map[string]any{
				"results": map[string]any{
					fieldCount: map[string]any{"qty": float64(1)},
				},
			},
			assertion: compat.Assertion{
				Type:  compat.AssertMatch,
				Value: map[string]any{"results.count.qty": float64(1)},
			},
		},
		{
			name:     "mismatched value fails",
			response: map[string]any{"amount": float64(5)},
			assertion: compat.Assertion{
				Type:  compat.AssertMatch,
				Value: map[string]any{"amount": float64(10)},
			},
			wantErr: true,
		},
		{
			name:     "int from yaml matches float64 from json",
			response: map[string]any{"quantity": float64(42)},
			assertion: compat.Assertion{
				Type:  compat.AssertMatch,
				Value: map[string]any{"quantity": 42},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := compat.CheckAssertion(tt.response, tt.assertion, nil)

			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestAssertIsTrue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		response  map[string]any
		assertion compat.Assertion
		wantErr   bool
	}{
		{
			name:      "truthy string passes",
			response:  map[string]any{"data": map[string]any{"heading": "hello"}},
			assertion: compat.Assertion{Type: compat.AssertIsTrue, Value: "data.heading"},
		},
		{
			name:      "false bool fails",
			response:  map[string]any{"flag": false},
			assertion: compat.Assertion{Type: compat.AssertIsTrue, Value: "flag"},
			wantErr:   true,
		},
		{
			name:      "nil value fails",
			response:  map[string]any{"empty": nil},
			assertion: compat.Assertion{Type: compat.AssertIsTrue, Value: "empty"},
			wantErr:   true,
		},
		{
			name:      "zero number fails",
			response:  map[string]any{"rank": float64(0)},
			assertion: compat.Assertion{Type: compat.AssertIsTrue, Value: "rank"},
			wantErr:   true,
		},
		{
			name:      "empty string fails",
			response:  map[string]any{"caption": ""},
			assertion: compat.Assertion{Type: compat.AssertIsTrue, Value: "caption"},
			wantErr:   true,
		},
		{
			name:      "non-empty slice passes",
			response:  map[string]any{"entries": []any{"one"}},
			assertion: compat.Assertion{Type: compat.AssertIsTrue, Value: "entries"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := compat.CheckAssertion(tt.response, tt.assertion, nil)

			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestAssertIsFalse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		response  map[string]any
		assertion compat.Assertion
		wantErr   bool
	}{
		{
			name:      "false value passes",
			response:  map[string]any{"disabled": false},
			assertion: compat.Assertion{Type: compat.AssertIsFalse, Value: "disabled"},
		},
		{
			name:      "missing path passes",
			response:  map[string]any{fieldOther: "present"},
			assertion: compat.Assertion{Type: compat.AssertIsFalse, Value: "nonexistent"},
		},
		{
			name:      "truthy value fails",
			response:  map[string]any{"active": true},
			assertion: compat.Assertion{Type: compat.AssertIsFalse, Value: "active"},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := compat.CheckAssertion(tt.response, tt.assertion, nil)

			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestAssertLength(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		response  map[string]any
		assertion compat.Assertion
		wantErr   bool
	}{
		{
			name: "matching length passes",
			response: map[string]any{
				"records": map[string]any{
					"rows": []any{"a", "b", "c"},
				},
			},
			assertion: compat.Assertion{
				Type:  compat.AssertLength,
				Value: map[string]any{"records.rows": float64(3)},
			},
		},
		{
			name:     "wrong length fails",
			response: map[string]any{testTags: []any{"go"}},
			assertion: compat.Assertion{
				Type:  compat.AssertLength,
				Value: map[string]any{testTags: float64(5)},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := compat.CheckAssertion(tt.response, tt.assertion, nil)

			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestAssertComparison(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		response  map[string]any
		assertion compat.Assertion
		wantErr   bool
	}{
		{
			name:     "gte pass",
			response: map[string]any{"latency": float64(10)},
			assertion: compat.Assertion{
				Type:  compat.AssertGTE,
				Value: map[string]any{"latency": float64(0)},
			},
		},
		{
			name:     "gte fail",
			response: map[string]any{"delay": float64(3)},
			assertion: compat.Assertion{
				Type:  compat.AssertGTE,
				Value: map[string]any{"delay": float64(100)},
			},
			wantErr: true,
		},
		{
			name:     "lte pass",
			response: map[string]any{"weight": float64(50)},
			assertion: compat.Assertion{
				Type:  compat.AssertLTE,
				Value: map[string]any{"weight": float64(1000)},
			},
		},
		{
			name:     "lte fail",
			response: map[string]any{"size": float64(999)},
			assertion: compat.Assertion{
				Type:  compat.AssertLTE,
				Value: map[string]any{"size": float64(10)},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := compat.CheckAssertion(tt.response, tt.assertion, nil)

			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestAssertGT(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		response  map[string]any
		assertion compat.Assertion
		wantErr   bool
	}{
		{
			name:     "greater value passes",
			response: map[string]any{fieldCount: float64(5)},
			assertion: compat.Assertion{
				Type:  compat.AssertGT,
				Value: map[string]any{fieldCount: float64(0)},
			},
		},
		{
			name:     "equal value fails",
			response: map[string]any{fieldCount: float64(0)},
			assertion: compat.Assertion{
				Type:  compat.AssertGT,
				Value: map[string]any{fieldCount: float64(0)},
			},
			wantErr: true,
		},
		{
			name:     "lesser value fails",
			response: map[string]any{fieldCount: float64(1)},
			assertion: compat.Assertion{
				Type:  compat.AssertGT,
				Value: map[string]any{fieldCount: float64(5)},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := compat.CheckAssertion(tt.response, tt.assertion, nil)

			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestAssertExists(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		response  map[string]any
		assertion compat.Assertion
		wantErr   bool
	}{
		{
			name:      "existing nested path passes",
			response:  map[string]any{fieldVersion: map[string]any{fieldNumber: versionNumber}},
			assertion: compat.Assertion{Type: compat.AssertExists, Value: "version.number"},
		},
		{
			name:      "local prefix passes",
			response:  map[string]any{fieldVersion: map[string]any{fieldNumber: versionNumber}},
			assertion: compat.Assertion{Type: compat.AssertExists, Value: "(local).version.number"},
		},
		{
			name:      "missing path fails",
			response:  map[string]any{fieldVersion: map[string]any{fieldNumber: versionNumber}},
			assertion: compat.Assertion{Type: compat.AssertExists, Value: "version.missing"},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := compat.CheckAssertion(tt.response, tt.assertion, nil)

			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}

func TestAssertContains(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		response  map[string]any
		assertion compat.Assertion
		wantErr   bool
	}{
		{
			name: "slice of maps contains subset",
			response: map[string]any{
				"nodes": map[string]any{
					"n1": map[string]any{
						fieldModules: []any{
							map[string]any{fieldName: fieldOther, fieldVersion: "1"},
							map[string]any{fieldName: entSearchModule, fieldVersion: "2"},
						},
					},
				},
			},
			assertion: compat.Assertion{
				Type:  compat.AssertContains,
				Value: map[string]any{"nodes.n1.modules": map[string]any{fieldName: entSearchModule}},
			},
		},
		{
			name:     "map subset passes",
			response: map[string]any{"settings": map[string]any{"x": float64(1), "y": float64(2)}},
			assertion: compat.Assertion{
				Type:  compat.AssertContains,
				Value: map[string]any{"settings": map[string]any{"x": float64(1)}},
			},
		},
		{
			name:     "string substring passes",
			response: map[string]any{"message": "hello world"},
			assertion: compat.Assertion{
				Type:  compat.AssertContains,
				Value: map[string]any{"message": "world"},
			},
		},
		{
			name:     "slice missing element fails",
			response: map[string]any{fieldModules: []any{map[string]any{fieldName: fieldOther}}},
			assertion: compat.Assertion{
				Type:  compat.AssertContains,
				Value: map[string]any{fieldModules: map[string]any{fieldName: entSearchModule}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := compat.CheckAssertion(tt.response, tt.assertion, nil)

			if tt.wantErr {
				assert.Error(t, err)

				return
			}

			require.NoError(t, err)
		})
	}
}
