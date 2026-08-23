package server

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"

	"github.com/pg-es/pg-es-proxy/api"
	"github.com/pg-es/pg-es-proxy/server"
	"github.com/pg-es/pg-es-proxy/utils"
)

// NewRouter builds an [http.ServeMux] with all Elasticsearch-compatible routes
// registered using Go 1.22+ method/pattern syntax.
func NewRouter(srv server.PGElasticServer) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /_cluster/health", handle(api.HealthHandler, srv))
	mux.HandleFunc("POST /_bulk", handle(api.BulkHandler, srv))

	mux.HandleFunc("PUT /{index}/_mapping/{type}", handle(api.PutTypeMapping, srv, "index"))
	mux.HandleFunc("GET /{index}/_search", handle(api.FindIndexDocumentHandler, srv, "index"))
	mux.HandleFunc("PUT /{index}", handle(api.PutIndexHandler, srv, "index"))
	mux.HandleFunc("HEAD /{index}", handle(api.HeadIndexHandler, srv, "index"))

	mux.HandleFunc("GET /{index}/{type}/_search", handle(api.FindDocumentHandler, srv, "index", "type"))
	mux.HandleFunc("PUT /{index}/{type}/{id}", handle(api.PutDocumentHandler, srv, "index", "type"))
	mux.HandleFunc("POST /{index}/{type}/{id}", handle(api.PutDocumentHandler, srv, "index", "type"))
	mux.HandleFunc("GET /{index}/{type}/{id}", handle(api.GetDocumentHandler, srv, "index", "type"))
	mux.HandleFunc("DELETE /{index}/{type}/{id}", handle(api.DeleteDocumentHandler, srv, "index", "type"))

	return mux
}

// validName returns true when name is a legal user-facing name (non-empty, no "_" prefix).
func validName(name string) bool {
	return name != "" && !strings.HasPrefix(name, "_")
}

func handle(handler server.Handler, srv server.PGElasticServer, validateParams ...string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rv := recover(); rv != nil {
				log.Printf("handler panic: %v", rv)
				w.WriteHeader(http.StatusInternalServerError)
				writeJSON(w, r, errorBody{Error: "internal server error"})
			}
		}()

		for _, param := range validateParams {
			if !validName(r.PathValue(param)) {
				http.NotFound(w, r)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")

		output, err := handler(r, srv)
		processResponse(w, r, output, err)
	}
}

type errorBody struct {
	Error string `json:"error"`
}

// processResponse handles the output of a handler invocation, formatting
// [utils.ElasticError] values as JSON error bodies.
func processResponse(w http.ResponseWriter, r *http.Request, output any, err error) {
	if err != nil {
		if elasticErr, ok := errors.AsType[utils.ElasticError](err); ok {
			w.WriteHeader(elasticErr.StatusCode())
			output = elasticErr.FormatErrorResponse()
		} else {
			w.WriteHeader(http.StatusInternalServerError)
			log.Printf("internal error: %v", err)
			writeJSON(w, r, errorBody{Error: "internal server error"})

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
		w.WriteHeader(http.StatusInternalServerError)
		log.Printf("json marshal error: %v", err)

		fallback, _ := json.Marshal(errorBody{Error: "response serialization failed"})
		_, _ = w.Write(fallback)

		return
	}

	_, _ = w.Write(data)
}
