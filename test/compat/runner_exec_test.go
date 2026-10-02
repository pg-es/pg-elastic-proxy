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

func newErrorServer(tb testing.TB, status int, errType string) *httptest.Server {
	tb.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{
			testErrField: map[string]any{"type": errType},
		})
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
			{Do: &compat.DoAction{Name: compat.ActionGet, Params: map[string]any{compat.ParamIndex: idxName, compat.ParamID: "1"}}},
		},
		Teardown: []compat.Step{
			{Do: &compat.DoAction{Name: compat.ActionGet, Params: map[string]any{compat.ParamIndex: idxName, compat.ParamID: "1"}}},
		},
		Tests: []compat.TestSection{
			{
				Name: "verify level",
				Steps: []compat.Step{
					{Do: &compat.DoAction{Name: compat.ActionGet, Params: map[string]any{compat.ParamIndex: idxName, compat.ParamID: "1"}}},
					{Assert: &compat.Assertion{Type: compat.AssertMatch, Value: map[string]any{"output.level": float64(1)}}},
				},
			},
		},
	}

	result := compat.RunFile(t.Context(), &file, executor, compat.Target{Flavour: testFlavour})

	assert.Equal(t, 1, result.Passed)
	assert.Equal(t, 0, result.Failed)
	assert.Equal(t, 0, result.Skipped)
	assert.Empty(t, result.Failures)
}

func TestRunFileUnsupportedActionCountsSkipped(t *testing.T) {
	t.Parallel()

	srv := newJSONServer(t, map[string]any{"ok": true})
	executor := compat.NewExecutor(srv.URL, srv.Client())

	file := compat.TestFile{
		Tests: []compat.TestSection{
			{
				Name: "unsupported test",
				Steps: []compat.Step{
					{Do: &compat.DoAction{Name: testUnknownAction, Params: map[string]any{}}},
				},
			},
			{
				Name: "passing test",
				Steps: []compat.Step{
					{Do: &compat.DoAction{Name: compat.ActionGet, Params: map[string]any{compat.ParamIndex: "skipidx", compat.ParamID: "2"}}},
				},
			},
		},
	}

	result := compat.RunFile(t.Context(), &file, executor, compat.Target{Flavour: testFlavour})

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
					{Do: &compat.DoAction{Name: compat.ActionGet, Params: map[string]any{compat.ParamIndex: "failidx", compat.ParamID: "3"}}},
					{Assert: &compat.Assertion{Type: compat.AssertMatch, Value: map[string]any{"rating": float64(999)}}},
				},
			},
		},
	}

	result := compat.RunFile(t.Context(), &file, executor, compat.Target{Flavour: testFlavour})

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
			{Do: &compat.DoAction{Name: compat.ActionGet, Params: map[string]any{compat.ParamIndex: "tdtest", compat.ParamID: "4"}}},
		},
		Teardown: []compat.Step{
			{Do: &compat.DoAction{Name: compat.ActionDelete, Params: map[string]any{compat.ParamIndex: "tdtest", compat.ParamID: "4"}}},
		},
		Tests: []compat.TestSection{
			{
				Name: "will fail",
				Steps: []compat.Step{
					{Assert: &compat.Assertion{Type: compat.AssertMatch, Value: map[string]any{"missing": "field"}}},
				},
			},
		},
	}

	result := compat.RunFile(t.Context(), &file, executor, compat.Target{Flavour: testFlavour})

	assert.Equal(t, 1, result.Failed)
	assert.True(t, teardownCalled.Load(), "teardown must run even when tests fail")
}

func TestRunFileSetSubstitution(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		steps        []compat.Step
		wantPassed   int
		wantRequests int
		wantLastPath string
	}{
		{
			name: "captures and reuses id",
			steps: []compat.Step{
				{Do: &compat.DoAction{Name: compat.ActionIndex, Params: map[string]any{compat.ParamIndex: testIdxIndex, compat.ParamBody: map[string]any{"title": "x"}}}},
				{Set: &compat.SetStep{Path: fieldID, Name: "id"}},
				{Do: &compat.DoAction{Name: compat.ActionGet, Params: map[string]any{compat.ParamIndex: testIdxIndex, compat.ParamID: testStashID}}},
				{Assert: &compat.Assertion{Type: compat.AssertIsTrue, Value: fieldFound}},
			},
			wantPassed:   1,
			wantRequests: 2,
			wantLastPath: "/idx/_doc/abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var paths []string

			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/_all" {
					paths = append(paths, r.URL.Path)
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)

				if r.Method == http.MethodPut || r.Method == http.MethodPost {
					_ = json.NewEncoder(w).Encode(map[string]any{fieldID: testDocID})

					return
				}

				_ = json.NewEncoder(w).Encode(map[string]any{fieldFound: true})
			}))
			t.Cleanup(srv.Close)

			executor := compat.NewExecutor(srv.URL, srv.Client())
			file := compat.TestFile{
				Tests: []compat.TestSection{{Name: tt.name, Steps: tt.steps}},
			}

			result := compat.RunFile(t.Context(), &file, executor, compat.Target{Flavour: testFlavour})

			assert.Equal(t, tt.wantPassed, result.Passed)
			require.Len(t, paths, tt.wantRequests)
			assert.Equal(t, tt.wantLastPath, paths[len(paths)-1])
		})
	}
}

func TestRunFileSkipStep(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		steps       []compat.Step
		wantPassed  int
		wantFailed  int
		wantSkipped int
	}{
		{
			name: "unsupported feature skips the section",
			steps: []compat.Step{
				{Skip: &compat.SkipStep{Features: testNodeSelector}},
				{Do: &compat.DoAction{Name: compat.ActionGet, Params: map[string]any{compat.ParamIndex: "x", compat.ParamID: "1"}}},
			},
			wantSkipped: 1,
		},
		{
			name: "supported feature does not skip",
			steps: []compat.Step{
				{Skip: &compat.SkipStep{Features: "allowed_warnings"}},
				{Do: &compat.DoAction{Name: compat.ActionGet, Params: map[string]any{compat.ParamIndex: "x", compat.ParamID: "1"}}},
			},
			wantPassed: 1,
		},
		{
			name: "awaits_fix always skips",
			steps: []compat.Step{
				{Skip: &compat.SkipStep{AwaitsFix: "https://github.com/elastic/elasticsearch/issues/1"}},
				{Do: &compat.DoAction{Name: compat.ActionGet, Params: map[string]any{compat.ParamIndex: "x", compat.ParamID: "1"}}},
			},
			wantSkipped: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := newJSONServer(t, map[string]any{"ok": true})
			executor := compat.NewExecutor(srv.URL, srv.Client())
			file := compat.TestFile{
				Tests: []compat.TestSection{{Name: tt.name, Steps: tt.steps}},
			}

			result := compat.RunFile(t.Context(), &file, executor, compat.Target{Flavour: testFlavour})

			assert.Equal(t, tt.wantPassed, result.Passed)
			assert.Equal(t, tt.wantFailed, result.Failed)
			assert.Equal(t, tt.wantSkipped, result.Skipped)
		})
	}
}

func TestRunFileAssertionStash(t *testing.T) {
	t.Parallel()

	srv := newJSONServer(t, map[string]any{fieldID: testDocID})
	executor := compat.NewExecutor(srv.URL, srv.Client())

	file := compat.TestFile{
		Tests: []compat.TestSection{
			{
				Name: "stash feeds an assertion",
				Steps: []compat.Step{
					{Do: &compat.DoAction{Name: compat.ActionGet, Params: map[string]any{compat.ParamIndex: testIdxIndex, compat.ParamID: "1"}}},
					{Set: &compat.SetStep{Path: fieldID, Name: "id"}},
					{Assert: &compat.Assertion{Type: compat.AssertMatch, Value: map[string]any{fieldID: testStashID}}},
				},
			},
		},
	}

	result := compat.RunFile(t.Context(), &file, executor, compat.Target{Flavour: testFlavour})

	assert.Equal(t, 1, result.Passed)
	assert.Empty(t, result.Failures)
}

func TestRunFileBodyStashForTextResponse(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello world"))
	}))
	t.Cleanup(srv.Close)

	executor := compat.NewExecutor(srv.URL, srv.Client())

	file := compat.TestFile{
		Tests: []compat.TestSection{
			{
				Name: "text body is stashed",
				Steps: []compat.Step{
					{Do: &compat.DoAction{Name: "cat.plugins", Params: map[string]any{}}},
					{Assert: &compat.Assertion{Type: compat.AssertIsTrue, Value: testBodyStash}},
				},
			},
		},
	}

	result := compat.RunFile(t.Context(), &file, executor, compat.Target{Flavour: testFlavour})

	assert.Equal(t, 1, result.Passed)
	assert.Empty(t, result.Failures)
}

// newAuthCaptureServer returns a JSON server and a getter for the last
// Authorization header it received.
func newAuthCaptureServer(tb testing.TB, body string) (srv *httptest.Server, lastAuth func() string) {
	tb.Helper()

	var auth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if header := r.Header.Get(testAuthHeader); header != "" {
			auth = header
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}))
	tb.Cleanup(server.Close)

	return server, func() string { return auth }
}

func TestRunFileHeaderFromStash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		body        string
		steps       []compat.Step
		wantAuthHdr string
	}{
		{
			name: "plain stash",
			body: `{"token":"abc"}`,
			steps: []compat.Step{
				{Do: &compat.DoAction{Name: compat.ActionGet, Params: map[string]any{compat.ParamIndex: "i", compat.ParamID: "1"}}},
				{Set: &compat.SetStep{Path: "token", Name: "tok"}},
				{
					Do: &compat.DoAction{
						Name:    compat.ActionGet,
						Params:  map[string]any{compat.ParamIndex: "i", compat.ParamID: "2"},
						Headers: map[string]string{testAuthHeader: "Bearer ${tok}"},
					},
				},
			},
			wantAuthHdr: "Bearer abc",
		},
		{
			name: "base64 transform",
			body: `{"user":"u","password":"p"}`,
			steps: []compat.Step{
				{Do: &compat.DoAction{Name: compat.ActionGet, Params: map[string]any{compat.ParamIndex: "i", compat.ParamID: "1"}}},
				{Set: &compat.SetStep{Name: "login_creds", Transform: "#base64EncodeCredentials(user,password)"}},
				{
					Do: &compat.DoAction{
						Name:    compat.ActionGet,
						Params:  map[string]any{compat.ParamIndex: "i", compat.ParamID: "2"},
						Headers: map[string]string{testAuthHeader: "Basic ${login_creds}"},
					},
				},
			},
			wantAuthHdr: "Basic dTpw",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv, lastAuth := newAuthCaptureServer(t, tt.body)
			executor := compat.NewExecutor(srv.URL, srv.Client())
			file := compat.TestFile{
				Tests: []compat.TestSection{{Name: tt.name, Steps: tt.steps}},
			}

			result := compat.RunFile(t.Context(), &file, executor, compat.Target{Flavour: testFlavour})

			assert.Equal(t, 1, result.Passed)
			assert.Equal(t, tt.wantAuthHdr, lastAuth())
		})
	}
}

func newWarningServer(tb testing.TB, warnings ...string) *httptest.Server {
	tb.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		for _, warning := range warnings {
			w.Header().Add("Warning", warning)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))

	tb.Cleanup(srv.Close)

	return srv
}

func TestRunFileWarnings(t *testing.T) {
	t.Parallel()

	const warning = `299 Elasticsearch "deprecated"`

	tests := []struct {
		name       string
		server     func(tb testing.TB) *httptest.Server
		doAction   compat.DoAction
		wantPassed int
		wantFailed int
	}{
		{
			name:       "expected warning present",
			server:     func(tb testing.TB) *httptest.Server { tb.Helper(); return newWarningServer(tb, warning) },
			doAction:   compat.DoAction{Name: compat.ActionGet, Params: map[string]any{compat.ParamIndex: "i", compat.ParamID: "1"}, Warnings: []string{warning}},
			wantPassed: 1,
		},
		{
			name:       "expected warning missing",
			server:     func(tb testing.TB) *httptest.Server { tb.Helper(); return newWarningServer(tb) },
			doAction:   compat.DoAction{Name: compat.ActionGet, Params: map[string]any{compat.ParamIndex: "i", compat.ParamID: "1"}, Warnings: []string{warning}},
			wantFailed: 1,
		},
		{
			name:       "allowed warning present",
			server:     func(tb testing.TB) *httptest.Server { tb.Helper(); return newWarningServer(tb, warning) },
			doAction:   compat.DoAction{Name: compat.ActionGet, Params: map[string]any{compat.ParamIndex: "i", compat.ParamID: "1"}, AllowedWarnings: []string{warning}},
			wantPassed: 1,
		},
		{
			name:       "unexpected warning fails",
			server:     func(tb testing.TB) *httptest.Server { tb.Helper(); return newWarningServer(tb, warning) },
			doAction:   compat.DoAction{Name: compat.ActionGet, Params: map[string]any{compat.ParamIndex: "i", compat.ParamID: "1"}},
			wantFailed: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := tt.server(t)
			executor := compat.NewExecutor(srv.URL, srv.Client())
			file := compat.TestFile{
				Tests: []compat.TestSection{{Name: tt.name, Steps: []compat.Step{{Do: &tt.doAction}}}},
			}

			result := compat.RunFile(t.Context(), &file, executor, compat.Target{Flavour: testFlavour})

			assert.Equal(t, tt.wantPassed, result.Passed)
			assert.Equal(t, tt.wantFailed, result.Failed)
		})
	}
}

func TestRunFileParsedBodyStash(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[{"a":1},{"a":2},{"a":3},{"a":4}]`))
	}))
	t.Cleanup(srv.Close)

	executor := compat.NewExecutor(srv.URL, srv.Client())

	file := compat.TestFile{
		Tests: []compat.TestSection{
			{
				Name: "array body is parsed",
				Steps: []compat.Step{
					{Do: &compat.DoAction{Name: compat.ActionSearch, Params: map[string]any{compat.ParamIndex: "i"}}},
					{Assert: &compat.Assertion{Type: compat.AssertLength, Value: map[string]any{testBodyStash: 4}}},
					{Assert: &compat.Assertion{Type: compat.AssertMatch, Value: map[string]any{"0.a": float64(1)}}},
				},
			},
		},
	}

	result := compat.RunFile(t.Context(), &file, executor, compat.Target{Flavour: testFlavour})

	assert.Equal(t, 1, result.Passed)
	assert.Empty(t, result.Failures)
}

func TestRunFileSkippedActionStopsSection(t *testing.T) {
	t.Parallel()

	var paths []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	executor := compat.NewExecutor(srv.URL, srv.Client())

	file := compat.TestFile{
		Tests: []compat.TestSection{
			{
				Name: "unsupported action first",
				Steps: []compat.Step{
					{Do: &compat.DoAction{Name: testUnknownAction, Params: map[string]any{}}},
					{Do: &compat.DoAction{Name: compat.ActionGet, Params: map[string]any{compat.ParamIndex: "i", compat.ParamID: "1"}}},
					{Assert: &compat.Assertion{Type: compat.AssertMatch, Value: map[string]any{"never": "reached"}}},
				},
			},
		},
	}

	result := compat.RunFile(t.Context(), &file, executor, compat.Target{Flavour: testFlavour})

	assert.Equal(t, 1, result.Skipped)
	assert.Equal(t, 0, result.Failed)
	assert.Equal(t, []string{"/_all"}, paths, "only the inter-section cleanup may run after a skipped action")
}

func getWithCatch(pattern string) compat.DoAction {
	return compat.DoAction{
		Name:   compat.ActionGet,
		Params: map[string]any{compat.ParamIndex: "x", compat.ParamID: "1"},
		Catch:  pattern,
	}
}

type catchCase struct {
	name        string
	server      func(tb testing.TB) *httptest.Server
	action      compat.DoAction
	wantPassed  int
	wantFailed  int
	wantSkipped int
}

func catchCases() []catchCase {
	return []catchCase{
		{
			name: "matching error type passes",
			server: func(tb testing.TB) *httptest.Server {
				tb.Helper()

				return newErrorServer(tb, http.StatusBadRequest, "illegal_argument_exception")
			},
			action:     getWithCatch(catchIllegalArg),
			wantPassed: 1,
		},
		{
			name: "mismatched error type fails",
			server: func(tb testing.TB) *httptest.Server {
				tb.Helper()

				return newErrorServer(tb, http.StatusBadRequest, "resource_not_found_exception")
			},
			action:     getWithCatch(catchIllegalArg),
			wantFailed: 1,
		},
		{
			name: "shorthand accepts its status code",
			server: func(tb testing.TB) *httptest.Server {
				tb.Helper()

				return newErrorServer(tb, http.StatusNotFound, "resource_not_found_exception")
			},
			action:     getWithCatch("missing"),
			wantPassed: 1,
		},
		{
			name: "shorthand requires its status code",
			server: func(tb testing.TB) *httptest.Server {
				tb.Helper()

				return newErrorServer(tb, http.StatusBadRequest, "illegal_argument_exception")
			},
			action:     getWithCatch("missing"),
			wantFailed: 1,
		},
		{
			name: "numeric catch matches exact status",
			server: func(tb testing.TB) *httptest.Server {
				tb.Helper()

				return newErrorServer(tb, http.StatusNotFound, "resource_not_found_exception")
			},
			action:     getWithCatch("404"),
			wantPassed: 1,
		},
		{
			name: "numeric catch rejects wrong status",
			server: func(tb testing.TB) *httptest.Server {
				tb.Helper()

				return newErrorServer(tb, http.StatusBadRequest, "illegal_argument_exception")
			},
			action:     getWithCatch("404"),
			wantFailed: 1,
		},
		{
			name: "unsupported action skips",
			server: func(tb testing.TB) *httptest.Server {
				tb.Helper()

				return newJSONServer(tb, map[string]any{"ok": true})
			},
			action:      compat.DoAction{Name: testUnknownAction, Params: map[string]any{compat.ParamID: "foobar"}, Catch: catchIllegalArg},
			wantSkipped: 1,
		},
	}
}

func TestRunFileCatch(t *testing.T) {
	t.Parallel()

	for _, tt := range catchCases() {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := tt.server(t)
			executor := compat.NewExecutor(srv.URL, srv.Client())
			file := compat.TestFile{
				Tests: []compat.TestSection{{Name: tt.name, Steps: []compat.Step{{Do: &tt.action}}}},
			}

			result := compat.RunFile(t.Context(), &file, executor, compat.Target{Flavour: testFlavour})

			assert.Equal(t, tt.wantPassed, result.Passed)
			assert.Equal(t, tt.wantFailed, result.Failed)
			assert.Equal(t, tt.wantSkipped, result.Skipped)
		})
	}
}
