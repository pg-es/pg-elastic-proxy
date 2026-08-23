package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pg-es/pg-es-proxy/internal/server"
)

const wantElasticProduct = "Elasticsearch"

func TestElasticProductMiddleware(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		handler         http.HandlerFunc
		wantProduct     string
		wantContentType string
	}{
		{
			name: "sets X-Elastic-Product header",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
			wantProduct:     wantElasticProduct,
			wantContentType: "application/json; charset=UTF-8",
		},
		{
			name: "sets default Content-Type",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Write([]byte(`{"ok":true}`)) //nolint:errcheck,gosec // test stub
			},
			wantProduct:     wantElasticProduct,
			wantContentType: "application/json; charset=UTF-8",
		},
		{
			name: "handler overrides Content-Type",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusOK)
			},
			wantProduct:     wantElasticProduct,
			wantContentType: "text/plain",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			wrapped := server.ElasticProductMiddleware(tc.handler)
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/", http.NoBody)
			rec := httptest.NewRecorder()

			wrapped.ServeHTTP(rec, req)

			if got := rec.Header().Get("X-Elastic-Product"); got != tc.wantProduct {
				t.Errorf("X-Elastic-Product = %q, want %q", got, tc.wantProduct)
			}

			if got := rec.Header().Get("Content-Type"); got != tc.wantContentType {
				t.Errorf("Content-Type = %q, want %q", got, tc.wantContentType)
			}
		})
	}
}
