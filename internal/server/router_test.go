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

func TestNewRouter(t *testing.T) {
	t.Parallel()

	mux := server.NewRouter(&mockServer{})

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantBody   string
	}{
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

		// Index routes: nil-DB triggers recovered panic, returns 500.
		{name: "put type mapping", method: http.MethodPut, path: "/idx/_mapping/mytype", wantStatus: http.StatusInternalServerError},
		{name: "put index", method: http.MethodPut, path: "/testidx", wantStatus: http.StatusInternalServerError},
		{name: "head index", method: http.MethodHead, path: "/testidx2", wantStatus: http.StatusInternalServerError},
		{name: "index search", method: http.MethodGet, path: "/idx/_search", wantStatus: http.StatusInternalServerError},

		// Type+endpoint routes: nil-DB triggers recovered panic, returns 500.
		{name: "type search", method: http.MethodGet, path: "/idx/tp/_search", wantStatus: http.StatusInternalServerError},
		{name: "put document", method: http.MethodPut, path: "/idx/tp/putdoc", wantStatus: http.StatusInternalServerError},
		{name: "post document", method: http.MethodPost, path: "/idx/tp/postdoc", wantStatus: http.StatusInternalServerError},
		{name: "get document", method: http.MethodGet, path: "/idx/tp/getdoc", wantStatus: http.StatusInternalServerError},
		{name: "delete document", method: http.MethodDelete, path: "/idx/tp/deldoc", wantStatus: http.StatusInternalServerError},

		// Underscore-prefixed index: 404 from validation.
		{name: "put underscore index", method: http.MethodPut, path: "/_reserved", wantStatus: http.StatusNotFound},
		{name: "head underscore index", method: http.MethodHead, path: "/_sys", wantStatus: http.StatusNotFound},
		{name: "mapping underscore index", method: http.MethodPut, path: "/_sys/_mapping/tp", wantStatus: http.StatusNotFound},
		{name: "search underscore index", method: http.MethodGet, path: "/_sys/_search", wantStatus: http.StatusNotFound},

		// Underscore-prefixed type: 404 from validation.
		{name: "search underscore type", method: http.MethodGet, path: "/idx/_badtype/_search", wantStatus: http.StatusNotFound},
		{name: "get doc underscore type", method: http.MethodGet, path: "/idx/_badtype/doc1", wantStatus: http.StatusNotFound},

		// Unknown routes: 404.
		{name: "deep unknown path", method: http.MethodGet, path: "/a/b/c/d/e", wantStatus: http.StatusNotFound},

		// Wrong method: 405.
		{name: "health wrong method", method: http.MethodPost, path: "/_cluster/health", wantStatus: http.StatusMethodNotAllowed},
		{name: "bulk wrong method", method: http.MethodGet, path: "/_bulk", wantStatus: http.StatusMethodNotAllowed},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequestWithContext(t.Context(), tc.method, tc.path, http.NoBody)
			rec := httptest.NewRecorder()

			mux.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}

			if tc.wantBody != "" && !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Errorf("body missing %q:\n%s", tc.wantBody, rec.Body.String())
			}
		})
	}
}
