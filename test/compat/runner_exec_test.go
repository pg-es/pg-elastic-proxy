package compat_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/pg-es/pg-es-proxy/test/compat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newJSONServer(tb testing.TB, response map[string]any) *httptest.Server {
	tb.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	}))

	tb.Cleanup(srv.Close)

	return srv
}

func TestRunFileAllPass(t *testing.T) {
	t.Parallel()

	srv := newJSONServer(t, map[string]any{
		"ok": true,
		"output": map[string]any{
			"level": float64(1),
		},
	})
	executor := compat.NewExecutor(srv.URL, srv.Client())

	idxName := "run-all"

	file := compat.TestFile{
		Setup: []compat.Step{
			{Do: &compat.DoAction{Name: actionGet, Params: map[string]any{paramIndex: idxName, paramID: "1"}}},
		},
		Teardown: []compat.Step{
			{Do: &compat.DoAction{Name: actionGet, Params: map[string]any{paramIndex: idxName, paramID: "1"}}},
		},
		Tests: []compat.TestSection{
			{
				Name: "verify level",
				Steps: []compat.Step{
					{Do: &compat.DoAction{Name: actionGet, Params: map[string]any{paramIndex: idxName, paramID: "1"}}},
					{Assert: &compat.Assertion{Type: assertMatch, Value: map[string]any{"output.level": float64(1)}}},
				},
			},
		},
	}

	result := compat.RunFile(t.Context(), file, compat.Skiplist{}, executor)

	assert.Equal(t, 1, result.Passed)
	assert.Equal(t, 0, result.Failed)
	assert.Equal(t, 0, result.Skipped)
	assert.Empty(t, result.Failures)
}

func TestRunFileSkiplist(t *testing.T) {
	t.Parallel()

	srv := newJSONServer(t, map[string]any{"ok": true})
	executor := compat.NewExecutor(srv.URL, srv.Client())

	file := compat.TestFile{
		Tests: []compat.TestSection{
			{Name: "skipped test", Steps: []compat.Step{}},
			{
				Name: "passing test",
				Steps: []compat.Step{
					{Do: &compat.DoAction{Name: actionGet, Params: map[string]any{paramIndex: "skipidx", paramID: "2"}}},
				},
			},
		},
	}

	skiplist := compat.Skiplist{
		Skips: []compat.SkipEntry{{File: "skipped test", Reason: "not implemented yet"}},
	}

	result := compat.RunFile(t.Context(), file, skiplist, executor)

	assert.Equal(t, 1, result.Passed)
	assert.Equal(t, 0, result.Failed)
	assert.Equal(t, 1, result.Skipped)
}

func TestRunFileAssertionFailure(t *testing.T) {
	t.Parallel()

	srv := newJSONServer(t, map[string]any{"rating": float64(1)})
	executor := compat.NewExecutor(srv.URL, srv.Client())

	file := compat.TestFile{
		Tests: []compat.TestSection{
			{
				Name: "failing assertion",
				Steps: []compat.Step{
					{Do: &compat.DoAction{Name: actionGet, Params: map[string]any{paramIndex: "failidx", paramID: "3"}}},
					{Assert: &compat.Assertion{Type: assertMatch, Value: map[string]any{"rating": float64(999)}}},
				},
			},
		},
	}

	result := compat.RunFile(t.Context(), file, compat.Skiplist{}, executor)

	assert.Equal(t, 0, result.Passed)
	assert.Equal(t, 1, result.Failed)
	require.Len(t, result.Failures, 1)
	assert.Equal(t, "failing assertion", result.Failures[0].TestName)
}

func TestRunFileTeardownAlwaysRuns(t *testing.T) {
	t.Parallel()

	var teardownCalled atomic.Bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			teardownCalled.Store(true)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	t.Cleanup(srv.Close)

	executor := compat.NewExecutor(srv.URL, srv.Client())

	file := compat.TestFile{
		Setup: []compat.Step{
			{Do: &compat.DoAction{Name: actionGet, Params: map[string]any{paramIndex: "tdtest", paramID: "4"}}},
		},
		Teardown: []compat.Step{
			{Do: &compat.DoAction{Name: "delete", Params: map[string]any{paramIndex: "tdtest", paramID: "4"}}},
		},
		Tests: []compat.TestSection{
			{
				Name: "will fail",
				Steps: []compat.Step{
					{Assert: &compat.Assertion{Type: assertMatch, Value: map[string]any{"missing": "field"}}},
				},
			},
		},
	}

	result := compat.RunFile(t.Context(), file, compat.Skiplist{}, executor)

	assert.Equal(t, 1, result.Failed)
	assert.True(t, teardownCalled.Load(), "teardown must run even when tests fail")
}
