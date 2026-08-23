package compat_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pg-es/pg-es-proxy/test/compat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	paramIndex      = "index"
	paramID         = "id"
	assertMatch     = "match"
	assertIsTrue    = "is_true"
	assertIsFalse   = "is_false"
	assertLength    = "length"
	assertGTE       = "gte"
	assertLTE       = "lte"
	actionGet       = "get"
	actionSearch    = "search"
	actionIdxCreate = "indices.create"
	actionIdxDelete = "indices.delete"
)

type capturedRequest struct {
	method      string
	path        string
	body        []byte
	contentType string
}

func newTestServer(tb testing.TB, response map[string]any) (*httptest.Server, *capturedRequest) {
	tb.Helper()

	captured := &capturedRequest{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.method = r.Method
		captured.path = r.URL.Path
		captured.contentType = r.Header.Get("Content-Type")

		reqBody, readErr := io.ReadAll(r.Body)
		if readErr == nil {
			captured.body = reqBody
		}

		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)

			return
		}

		w.WriteHeader(http.StatusOK)

		if response != nil {
			_ = json.NewEncoder(w).Encode(response)
		}
	}))

	tb.Cleanup(srv.Close)

	return srv, captured
}

func TestExecutorIndex(t *testing.T) {
	t.Parallel()

	srv, captured := newTestServer(t, map[string]any{"result": "created"})
	executor := compat.NewExecutor(srv.URL, srv.Client())

	action := compat.DoAction{
		Name: "index",
		Params: map[string]any{
			paramIndex: "myindex",
			paramID:    "doc1",
			"body":     map[string]any{"subject": "hello"},
		},
	}

	_, status, err := executor.Execute(t.Context(), action)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, status)
	assert.Equal(t, http.MethodPut, captured.method)
	assert.Equal(t, "/myindex/_doc/doc1", captured.path)
	assert.Equal(t, "application/json", captured.contentType)
}

func TestExecutorGet(t *testing.T) {
	t.Parallel()

	srv, captured := newTestServer(t, map[string]any{"_ref": "item1"})
	executor := compat.NewExecutor(srv.URL, srv.Client())

	action := compat.DoAction{
		Name:   actionGet,
		Params: map[string]any{paramIndex: "products", paramID: "item1"},
	}

	resp, _, err := executor.Execute(t.Context(), action)

	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, captured.method)
	assert.Equal(t, "/products/_doc/item1", captured.path)
	assert.Equal(t, "item1", resp["_ref"])
}

func TestExecutorDelete(t *testing.T) {
	t.Parallel()

	srv, captured := newTestServer(t, map[string]any{"result": "deleted"})
	executor := compat.NewExecutor(srv.URL, srv.Client())

	action := compat.DoAction{
		Name:   "delete",
		Params: map[string]any{paramIndex: "logs", paramID: "entry5"},
	}

	_, _, err := executor.Execute(t.Context(), action)

	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, captured.method)
	assert.Equal(t, "/logs/_doc/entry5", captured.path)
}

func TestExecutorSearch(t *testing.T) {
	t.Parallel()

	srv, captured := newTestServer(t, map[string]any{"matches": map[string]any{"cnt": float64(0)}})
	executor := compat.NewExecutor(srv.URL, srv.Client())

	action := compat.DoAction{
		Name:   actionSearch,
		Params: map[string]any{paramIndex: "events"},
	}

	_, _, err := executor.Execute(t.Context(), action)

	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, captured.method)
	assert.Equal(t, "/events/_search", captured.path)
}

func TestExecutorSearchNoIndex(t *testing.T) {
	t.Parallel()

	srv, captured := newTestServer(t, map[string]any{"output": map[string]any{"num": float64(0)}})
	executor := compat.NewExecutor(srv.URL, srv.Client())

	action := compat.DoAction{
		Name:   actionSearch,
		Params: map[string]any{},
	}

	_, _, err := executor.Execute(t.Context(), action)

	require.NoError(t, err)
	assert.Equal(t, "/_search", captured.path)
}

func TestExecutorIndicesCreate(t *testing.T) {
	t.Parallel()

	srv, captured := newTestServer(t, map[string]any{"ok": true})
	executor := compat.NewExecutor(srv.URL, srv.Client())

	action := compat.DoAction{
		Name:   actionIdxCreate,
		Params: map[string]any{paramIndex: "fresh-index"},
	}

	_, _, err := executor.Execute(t.Context(), action)

	require.NoError(t, err)
	assert.Equal(t, http.MethodPut, captured.method)
	assert.Equal(t, "/fresh-index", captured.path)
}

func TestExecutorIndicesDelete(t *testing.T) {
	t.Parallel()

	srv, captured := newTestServer(t, map[string]any{"ok": true})
	executor := compat.NewExecutor(srv.URL, srv.Client())

	action := compat.DoAction{
		Name:   actionIdxDelete,
		Params: map[string]any{paramIndex: "stale-index"},
	}

	_, _, err := executor.Execute(t.Context(), action)

	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, captured.method)
	assert.Equal(t, "/stale-index", captured.path)
}

func TestExecutorIndicesExists(t *testing.T) {
	t.Parallel()

	srv, captured := newTestServer(t, nil)
	executor := compat.NewExecutor(srv.URL, srv.Client())

	action := compat.DoAction{
		Name:   "indices.exists",
		Params: map[string]any{paramIndex: "check-index"},
	}

	result, status, err := executor.Execute(t.Context(), action)

	require.NoError(t, err)
	assert.Equal(t, http.MethodHead, captured.method)
	assert.Equal(t, "/check-index", captured.path)
	assert.Equal(t, http.StatusOK, status)
	assert.Empty(t, result)
}

func TestExecutorBulk(t *testing.T) {
	t.Parallel()

	srv, captured := newTestServer(t, map[string]any{"records": []any{}})
	executor := compat.NewExecutor(srv.URL, srv.Client())

	action := compat.DoAction{
		Name: "bulk",
		Params: map[string]any{
			"body": []any{
				map[string]any{"index": map[string]any{"_index": "bulkidx"}},
				map[string]any{"heading": "bulk document"},
			},
		},
	}

	_, _, err := executor.Execute(t.Context(), action)

	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, captured.method)
	assert.Equal(t, "/_bulk", captured.path)
	assert.Equal(t, "application/x-ndjson", captured.contentType)
	assert.NotEmpty(t, captured.body)
}

func TestExecutorUnknownAction(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(t, nil)
	executor := compat.NewExecutor(srv.URL, srv.Client())

	action := compat.DoAction{
		Name:   "aggregations.terms",
		Params: map[string]any{},
	}

	_, _, err := executor.Execute(t.Context(), action)

	require.ErrorIs(t, err, compat.ErrSkip)
}
