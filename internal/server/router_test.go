package server_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pg-es/pg-es-proxy/db"
	"github.com/pg-es/pg-es-proxy/internal/server"
	"github.com/pg-es/pg-es-proxy/utils"
)

type mockServer struct{}

func (*mockServer) GetConfiguration() utils.PGElasticConfig {
	return utils.PGElasticConfig{}
}

func (*mockServer) GetDBClient() *db.Client {
	return nil
}

func (*mockServer) Start() {}

type routeTestCase struct {
	name       string
	method     string
	path       string
	wantStatus int
	wantPanic  bool
	wantBody   string
}

// serveAndRecover calls handler.ServeHTTP, recovering any panic from nil-DB
// handler dispatch. Returns the recorder and whether the handler panicked.
func serveAndRecover(tb testing.TB, handler http.Handler, req *http.Request) (*httptest.ResponseRecorder, bool) {
	tb.Helper()

	rec := httptest.NewRecorder()
	didPanic := false

	func() {
		defer func() {
			if rv := recover(); rv != nil {
				didPanic = true
			}
		}()

		handler.ServeHTTP(rec, req)
	}()

	return rec, didPanic
}

func assertRouteResult(t *testing.T, tc *routeTestCase, rec *httptest.ResponseRecorder, panicked bool) {
	t.Helper()

	if tc.wantPanic {
		if !panicked {
			t.Errorf("expected panic (nil-DB dispatch proof), got status %d", rec.Code)
		}

		return
	}

	if panicked {
		t.Fatal("unexpected panic")
	}

	if rec.Code != tc.wantStatus {
		t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
	}

	if tc.wantBody != "" && !strings.Contains(rec.Body.String(), tc.wantBody) {
		t.Errorf("body missing %q:\n%s", tc.wantBody, rec.Body.String())
	}
}

func TestNewRouter(t *testing.T) {
	t.Parallel()

	mux := server.NewRouter(&mockServer{})

	tests := []routeTestCase{
		// System routes: work without DB.
		{
			name:       "cluster health",
			method:     http.MethodGet,
			path:       "/_cluster/health",
			wantStatus: http.StatusOK,
			wantBody:   "cluster_name",
		},
		{
			name:       "bulk empty body",
			method:     http.MethodPost,
			path:       "/_bulk",
			wantStatus: http.StatusOK,
			wantBody:   "took",
		},

		// Index routes: nil-DB panic proves correct dispatch.
		{name: "put type mapping", method: http.MethodPut, path: "/idx/_mapping/mytype", wantPanic: true},
		{name: "put index", method: http.MethodPut, path: "/testidx", wantPanic: true},
		{name: "head index", method: http.MethodHead, path: "/testidx2", wantPanic: true},
		{name: "index search", method: http.MethodGet, path: "/idx/_search", wantPanic: true},

		// Type+endpoint routes: nil-DB panic proves correct dispatch.
		{name: "type search", method: http.MethodGet, path: "/idx/tp/_search", wantPanic: true},
		{name: "put document", method: http.MethodPut, path: "/idx/tp/putdoc", wantPanic: true},
		{name: "post document", method: http.MethodPost, path: "/idx/tp/postdoc", wantPanic: true},
		{name: "get document", method: http.MethodGet, path: "/idx/tp/getdoc", wantPanic: true},
		{name: "delete document", method: http.MethodDelete, path: "/idx/tp/deldoc", wantPanic: true},

		// Underscore-prefixed index → 404 from validation.
		{name: "put underscore index", method: http.MethodPut, path: "/_reserved", wantStatus: http.StatusNotFound},
		{name: "head underscore index", method: http.MethodHead, path: "/_sys", wantStatus: http.StatusNotFound},
		{name: "mapping underscore index", method: http.MethodPut, path: "/_sys/_mapping/tp", wantStatus: http.StatusNotFound},
		{name: "search underscore index", method: http.MethodGet, path: "/_sys/_search", wantStatus: http.StatusNotFound},

		// Underscore-prefixed type → 404 from validation.
		{name: "search underscore type", method: http.MethodGet, path: "/idx/_badtype/_search", wantStatus: http.StatusNotFound},
		{name: "get doc underscore type", method: http.MethodGet, path: "/idx/_badtype/doc1", wantStatus: http.StatusNotFound},

		// Unknown routes → 404.
		{name: "deep unknown path", method: http.MethodGet, path: "/a/b/c/d/e", wantStatus: http.StatusNotFound},

		// Wrong method → 405.
		{name: "health wrong method", method: http.MethodPost, path: "/_cluster/health", wantStatus: http.StatusMethodNotAllowed},
		{name: "bulk wrong method", method: http.MethodGet, path: "/_bulk", wantStatus: http.StatusMethodNotAllowed},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequestWithContext(t.Context(), tc.method, tc.path, http.NoBody)
			rec, panicked := serveAndRecover(t, mux, req)

			assertRouteResult(t, &tc, rec, panicked)
		})
	}
}
