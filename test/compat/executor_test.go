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
	fieldCount        = "count"
	fieldOther        = "other"
	fieldModules      = "modules"
	fieldName         = "name"
	fieldFound        = "found"
	fieldID           = "_id"
	fieldNumber       = "number"
	fieldVersion      = "version"
	versionNumber     = "8.0"
	entSearchModule   = "x-pack-ent-search"
	catchIllegalArg   = "/illegal_argument_exception/"
	testMsg           = "msg"
	testDocID         = "abc123"
	testStashID       = "$id"
	testLogsIndex     = "logs"
	testIdxIndex      = "idx"
	testFlavour       = "es9"
	testNodeSelector  = "node_selector"
	testAuthHeader    = "Authorization"
	testCreated       = "created"
	testRatio         = "ratio"
	testErrField      = "error"
	testTags          = "tags"
	testHits          = "hits"
	testValue         = "value"
	testBodyStash     = "$body"
	testUnknownAction = "nonexistent.action"
)

type capturedRequest struct {
	method      string
	path        string
	query       string
	body        []byte
	contentType string
}

func newTestServer(tb testing.TB, response map[string]any) (*httptest.Server, *capturedRequest) {
	tb.Helper()

	captured := &capturedRequest{}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.method = r.Method
		captured.path = r.URL.Path
		captured.query = r.URL.RawQuery
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

	srv, captured := newTestServer(t, map[string]any{"result": testCreated})
	executor := compat.NewExecutor(srv.URL, srv.Client())

	action := compat.DoAction{
		Name: compat.ActionIndex,
		Params: map[string]any{
			compat.ParamIndex: "myindex",
			compat.ParamID:    "doc1",
			compat.ParamBody:  map[string]any{"subject": "hello"},
		},
	}

	resp, err := executor.Execute(t.Context(), &action, testFlavour)

	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.Status)
	assert.Equal(t, http.MethodPut, captured.method)
	assert.Equal(t, "/myindex/_doc/doc1", captured.path)
	assert.Equal(t, "application/json", captured.contentType)
}

func TestExecutorGet(t *testing.T) {
	t.Parallel()

	srv, captured := newTestServer(t, map[string]any{"_ref": "item1"})
	executor := compat.NewExecutor(srv.URL, srv.Client())

	action := compat.DoAction{
		Name:   compat.ActionGet,
		Params: map[string]any{compat.ParamIndex: "products", compat.ParamID: "item1"},
	}

	resp, err := executor.Execute(t.Context(), &action, testFlavour)

	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, captured.method)
	assert.Equal(t, "/products/_doc/item1", captured.path)

	body, ok := resp.Body.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "item1", body["_ref"])
}

func TestExecutorDelete(t *testing.T) {
	t.Parallel()

	srv, captured := newTestServer(t, map[string]any{"result": "deleted"})
	executor := compat.NewExecutor(srv.URL, srv.Client())

	action := compat.DoAction{
		Name:   compat.ActionDelete,
		Params: map[string]any{compat.ParamIndex: testLogsIndex, compat.ParamID: "entry5"},
	}

	_, err := executor.Execute(t.Context(), &action, testFlavour)

	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, captured.method)
	assert.Equal(t, "/logs/_doc/entry5", captured.path)
}

func TestExecutorSearch(t *testing.T) {
	t.Parallel()

	srv, captured := newTestServer(t, map[string]any{"matches": map[string]any{"cnt": float64(0)}})
	executor := compat.NewExecutor(srv.URL, srv.Client())

	action := compat.DoAction{
		Name:   compat.ActionSearch,
		Params: map[string]any{compat.ParamIndex: "events"},
	}

	_, err := executor.Execute(t.Context(), &action, testFlavour)

	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, captured.method)
	assert.Equal(t, "/events/_search", captured.path)
}

func TestExecutorSearchNoIndex(t *testing.T) {
	t.Parallel()

	srv, captured := newTestServer(t, map[string]any{"output": map[string]any{"num": float64(0)}})
	executor := compat.NewExecutor(srv.URL, srv.Client())

	action := compat.DoAction{
		Name:   compat.ActionSearch,
		Params: map[string]any{},
	}

	_, err := executor.Execute(t.Context(), &action, testFlavour)

	require.NoError(t, err)
	assert.Equal(t, "/_search", captured.path)
}

func TestExecutorIndicesCreate(t *testing.T) {
	t.Parallel()

	srv, captured := newTestServer(t, map[string]any{"ok": true})
	executor := compat.NewExecutor(srv.URL, srv.Client())

	action := compat.DoAction{
		Name:   compat.ActionIndicesCreate,
		Params: map[string]any{compat.ParamIndex: "fresh-index"},
	}

	_, err := executor.Execute(t.Context(), &action, testFlavour)

	require.NoError(t, err)
	assert.Equal(t, http.MethodPut, captured.method)
	assert.Equal(t, "/fresh-index", captured.path)
}

func TestExecutorIndicesDelete(t *testing.T) {
	t.Parallel()

	srv, captured := newTestServer(t, map[string]any{"ok": true})
	executor := compat.NewExecutor(srv.URL, srv.Client())

	action := compat.DoAction{
		Name:   compat.ActionIndicesDelete,
		Params: map[string]any{compat.ParamIndex: "stale-index"},
	}

	_, err := executor.Execute(t.Context(), &action, testFlavour)

	require.NoError(t, err)
	assert.Equal(t, http.MethodDelete, captured.method)
	assert.Equal(t, "/stale-index", captured.path)
}

func TestExecutorIndicesExists(t *testing.T) {
	t.Parallel()

	srv, captured := newTestServer(t, nil)
	executor := compat.NewExecutor(srv.URL, srv.Client())

	action := compat.DoAction{
		Name:   compat.ActionIndicesExists,
		Params: map[string]any{compat.ParamIndex: "check-index"},
	}

	result, err := executor.Execute(t.Context(), &action, testFlavour)

	require.NoError(t, err)
	assert.Equal(t, http.MethodHead, captured.method)
	assert.Equal(t, "/check-index", captured.path)
	assert.Equal(t, http.StatusOK, result.Status)
	assert.Empty(t, result.Body)
}

func TestExecutorBulk(t *testing.T) {
	t.Parallel()

	srv, captured := newTestServer(t, map[string]any{"records": []any{}})
	executor := compat.NewExecutor(srv.URL, srv.Client())

	action := compat.DoAction{
		Name: compat.ActionBulk,
		Params: map[string]any{
			compat.ParamBody: []any{
				map[string]any{compat.ParamIndex: map[string]any{"_index": "bulkidx"}},
				map[string]any{"heading": "bulk document"},
			},
		},
	}

	_, err := executor.Execute(t.Context(), &action, testFlavour)

	require.NoError(t, err)
	assert.Equal(t, http.MethodPost, captured.method)
	assert.Equal(t, "/_bulk", captured.path)
	assert.Equal(t, "application/x-ndjson", captured.contentType)
	assert.NotEmpty(t, captured.body)
}

func TestExecutorQueryParams(t *testing.T) {
	t.Parallel()

	srv, captured := newTestServer(t, map[string]any{"status": "yellow"})
	executor := compat.NewExecutor(srv.URL, srv.Client())

	action := compat.DoAction{
		Name:   "cluster.health",
		Params: map[string]any{"level": "shards", "wait_for_status": "green"},
	}

	_, err := executor.Execute(t.Context(), &action, testFlavour)

	require.NoError(t, err)
	assert.Equal(t, http.MethodGet, captured.method)
	assert.Equal(t, "/_cluster/health", captured.path)
	assert.Contains(t, captured.query, "level=shards")
	assert.Contains(t, captured.query, "wait_for_status=green")
}

func TestExecutorPathVariantSelection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		action     compat.DoAction
		wantMethod string
		wantPath   string
	}{
		{
			name:       "count without index",
			action:     compat.DoAction{Name: "count", Params: map[string]any{}},
			wantMethod: http.MethodPost,
			wantPath:   "/_count",
		},
		{
			name:       "count with index",
			action:     compat.DoAction{Name: "count", Params: map[string]any{compat.ParamIndex: testLogsIndex}},
			wantMethod: http.MethodPost,
			wantPath:   "/logs/_count",
		},
		{
			name:       "index without id",
			action:     compat.DoAction{Name: compat.ActionIndex, Params: map[string]any{compat.ParamIndex: testLogsIndex}},
			wantMethod: http.MethodPost,
			wantPath:   "/logs/_doc",
		},
		{
			name:       "index with id",
			action:     compat.DoAction{Name: compat.ActionIndex, Params: map[string]any{compat.ParamIndex: testLogsIndex, compat.ParamID: "1"}},
			wantMethod: http.MethodPut,
			wantPath:   "/logs/_doc/1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv, captured := newTestServer(t, map[string]any{"ok": true})
			executor := compat.NewExecutor(srv.URL, srv.Client())

			_, err := executor.Execute(t.Context(), &tt.action, testFlavour)

			require.NoError(t, err)
			assert.Equal(t, tt.wantMethod, captured.method)
			assert.Equal(t, tt.wantPath, captured.path)
		})
	}
}

func TestExecutorHeaders(t *testing.T) {
	t.Parallel()

	var got string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Get(testAuthHeader)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	executor := compat.NewExecutor(srv.URL, srv.Client())

	_, err := executor.Execute(t.Context(), &compat.DoAction{
		Name:    compat.ActionGet,
		Params:  map[string]any{compat.ParamIndex: "i", compat.ParamID: "1"},
		Headers: map[string]string{testAuthHeader: "Basic xyz"},
	}, testFlavour)

	require.NoError(t, err)
	assert.Equal(t, "Basic xyz", got)
}

func TestExecutorStringBodySentVerbatim(t *testing.T) {
	t.Parallel()

	srv, captured := newTestServer(t, map[string]any{"ok": true})
	executor := compat.NewExecutor(srv.URL, srv.Client())

	raw := "{\n  \"query\": { \"match_all\": {} }\n}"

	_, err := executor.Execute(t.Context(), &compat.DoAction{
		Name:   compat.ActionSearch,
		Params: map[string]any{compat.ParamIndex: "i", compat.ParamBody: raw},
	}, testFlavour)

	require.NoError(t, err)
	assert.Equal(t, raw, string(captured.body))
}

func TestExecutorBulkStringBodyVerbatim(t *testing.T) {
	t.Parallel()

	srv, captured := newTestServer(t, map[string]any{"ok": true})
	executor := compat.NewExecutor(srv.URL, srv.Client())

	raw := "{\"index\":{\"_index\":\"i\"}}\n{\"a\":1}\n"

	_, err := executor.Execute(t.Context(), &compat.DoAction{
		Name:   compat.ActionBulk,
		Params: map[string]any{compat.ParamBody: raw},
	}, testFlavour)

	require.NoError(t, err)
	assert.Equal(t, raw, string(captured.body))
	assert.Equal(t, "application/x-ndjson", captured.contentType)
}

func TestExecutorUnknownAction(t *testing.T) {
	t.Parallel()

	srv, _ := newTestServer(t, nil)
	executor := compat.NewExecutor(srv.URL, srv.Client())

	action := compat.DoAction{
		Name:   "aggregations.terms",
		Params: map[string]any{},
	}

	_, err := executor.Execute(t.Context(), &action, testFlavour)

	require.ErrorIs(t, err, compat.ErrSkip)
}
