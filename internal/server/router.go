package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/pg-es/pg-es-proxy/api"
	"github.com/pg-es/pg-es-proxy/server"
	"github.com/pg-es/pg-es-proxy/utils"
)

// NewRouter builds an [http.ServeMux] with all Elasticsearch-compatible routes
// registered using Go 1.22+ method/pattern syntax.
func NewRouter(srv server.PGElasticServer) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /_cluster/health", wrapRequestHandler(api.HealthHandler, srv))
	mux.HandleFunc("POST /_bulk", wrapRequestHandler(api.BulkHandler, srv))

	mux.HandleFunc("PUT /{index}/_mapping/{type}", wrapIndexRequestHandler(api.PutTypeMapping, srv))
	mux.HandleFunc("GET /{index}/_search", wrapIndexRequestHandler(api.FindIndexDocumentHandler, srv))
	mux.HandleFunc("PUT /{index}", wrapIndexRequestHandler(api.PutIndexHandler, srv))
	mux.HandleFunc("HEAD /{index}", wrapIndexRequestHandler(api.HeadIndexHandler, srv))

	mux.HandleFunc("GET /{index}/{type}/_search", wrapTypeSearchHandler(api.FindDocumentHandler, srv))
	mux.HandleFunc("PUT /{index}/{type}/{id}", wrapDocumentHandler(api.PutDocumentHandler, srv))
	mux.HandleFunc("POST /{index}/{type}/{id}", wrapDocumentHandler(api.PutDocumentHandler, srv))
	mux.HandleFunc("GET /{index}/{type}/{id}", wrapDocumentHandler(api.GetDocumentHandler, srv))
	mux.HandleFunc("DELETE /{index}/{type}/{id}", wrapDocumentHandler(api.DeleteDocumentHandler, srv))

	return mux
}

// validName returns true when name is a legal user-facing name (non-empty, no "_" prefix).
func validName(name string) bool {
	return name != "" && !strings.HasPrefix(name, "_")
}

// wrapRequestHandler adapts an [server.ElasticRequestHandler] to [http.HandlerFunc].
// The handler receives r.URL.Path as its path argument.
func wrapRequestHandler(handler server.ElasticRequestHandler, srv server.PGElasticServer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		output, err := handler(r.URL.Path, r, srv)
		processResponse(w, r, output, err)
	}
}

// wrapIndexRequestHandler adapts an [server.ElasticRequestHandler] for routes
// containing an {index} path parameter, rejecting "_"-prefixed indices.
func wrapIndexRequestHandler(handler server.ElasticRequestHandler, srv server.PGElasticServer) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !validName(r.PathValue("index")) {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		output, err := handler(r.URL.Path, r, srv)
		processResponse(w, r, output, err)
	}
}

// wrapTypeSearchHandler adapts [api.FindDocumentHandler] for GET /{index}/{type}/_search.
func wrapTypeSearchHandler(
	handler server.ElasticEndpointRequestHandler, srv server.PGElasticServer,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		index := r.PathValue("index")
		typeName := r.PathValue("type")

		if !validName(index) || !validName(typeName) {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		output, err := handler(index, typeName, "_search", r, srv)
		processResponse(w, r, output, err)
	}
}

// wrapDocumentHandler adapts an [server.ElasticEndpointRequestHandler] for
// document CRUD routes (/{index}/{type}/{id}).
func wrapDocumentHandler(
	handler server.ElasticEndpointRequestHandler, srv server.PGElasticServer,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		index := r.PathValue("index")
		typeName := r.PathValue("type")

		if !validName(index) || !validName(typeName) {
			http.NotFound(w, r)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		endpoint := r.PathValue("id")

		output, err := handler(index, typeName, endpoint, r, srv)
		processResponse(w, r, output, err)
	}
}

// processResponse handles the output of a handler invocation, formatting
// [utils.ElasticError] values as JSON error bodies.
func processResponse(w http.ResponseWriter, r *http.Request, output any, err error) {
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)

		if elasticErr, ok := errors.AsType[utils.ElasticError](err); ok {
			output = elasticErr.FormatErrorResponse()
		} else {
			fmt.Fprintf(os.Stderr, "internal error: %v\n", err)

			_, _ = w.Write([]byte(`{"error":"internal server error"}`))

			return
		}
	}

	writeJSON(w, r, output)
}

// writeJSON serialises output as JSON into w. When the "pretty" query
// parameter is "true" or absent the output is indented.
func writeJSON(w http.ResponseWriter, r *http.Request, output any) {
	var (
		data []byte
		err  error
	)

	pretty := r.URL.Query().Get("pretty")
	if pretty == "true" || pretty == "" {
		data, err = json.MarshalIndent(output, "", "    ")
	} else {
		data, err = json.Marshal(output)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "json marshal error: %v\n", err)

		_, _ = w.Write([]byte(`{"error":"response serialization failed"}`))

		return
	}

	_, _ = w.Write(data)
}
