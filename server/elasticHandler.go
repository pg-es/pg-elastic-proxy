package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"slices"

	"github.com/pg-es/pg-es-proxy/utils"
)

// ElasticEndpointRequestHandler is a handler function for requests with URL: /<index>/<type>/<endpoint>. Index, type and endpoint are automatically extracted from URL
type ElasticEndpointRequestHandler func(string, string, string, *http.Request, PGElasticServer) (any, error)

// ElasticRequestHandler is a handler function for any request
type ElasticRequestHandler func(string, *http.Request, PGElasticServer) (any, error)

type regexpRoute struct {
	pattern *regexp.Regexp
	handler ElasticRequestHandler
	methods []string
}

type regexpEndpointRoute struct {
	pattern *regexp.Regexp
	handler ElasticEndpointRequestHandler
	methods []string
}

// ElasticHandler contains information how to precess all routes of the server
type ElasticHandler struct {
	specialRoutes   []*regexpRoute
	endpointPattern *regexp.Regexp
	endpointRoutes  []*regexpEndpointRoute
	server          PGElasticServer
}

// NewElasticHandler creates a new instance of handler-router for PGElasticServer
func NewElasticHandler(s PGElasticServer) (result *ElasticHandler) {
	result = new(ElasticHandler)
	result.server = s
	result.endpointPattern = regexp.MustCompile(`/(?P<index>\w+)/(?P<type>[^_]\w+)/(?P<endpoint>.*)`)
	return result
}

// HandleFunc adds a handler for request. URL is described via pattern
func (h *ElasticHandler) HandleFunc(pattern *regexp.Regexp, handler ElasticRequestHandler, methods []string) {
	h.specialRoutes = append(h.specialRoutes, &regexpRoute{pattern, handler, methods})
}

// HandleFuncEndpoint adds a handler for request with URL: /<index>/<type>/<endpoint>. Endpoint format is described via pattern
func (h *ElasticHandler) HandleFuncEndpoint(pattern *regexp.Regexp, handler ElasticEndpointRequestHandler, methods []string) {
	h.endpointRoutes = append(h.endpointRoutes, &regexpEndpointRoute{pattern, handler, methods})
}

// ServeHTTP handles and route request to appropriate handler
func (h *ElasticHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if h.endpointPattern.MatchString(r.URL.Path) {
		indexName := h.endpointPattern.ReplaceAllString(r.URL.Path, "${index}")
		typeName := h.endpointPattern.ReplaceAllString(r.URL.Path, "${type}")
		endpoint := h.endpointPattern.ReplaceAllString(r.URL.Path, "${endpoint}")

		for _, route := range h.endpointRoutes {
			if route.pattern.MatchString(endpoint) && supportMethod(r.Method, route.methods) {
				output, err := route.handler(indexName, typeName, endpoint, r, h.server)
				h.processRequestOutput(w, r, output, err)
				return
			}
		}

		// Return JSON error for unsupported endpoint
		w.WriteHeader(http.StatusNotFound)
		h.writeOutput(w, r, map[string]any{
			"error":    "endpoint not supported",
			"index":    indexName,
			"type":     typeName,
			"endpoint": endpoint,
		})
	} else {
		for _, route := range h.specialRoutes {
			if route.pattern.MatchString(r.URL.Path) && supportMethod(r.Method, route.methods) {
				output, err := route.handler(r.URL.Path, r, h.server)
				h.processRequestOutput(w, r, output, err)
				return
			}
		}
		// Print message about unsupported request
		fmt.Println(r.URL)
		r.Write(os.Stderr) //nolint:errcheck,gosec // best-effort request dump to stderr for debugging
		http.NotFound(w, r)
	}
}

// Print output structure in JSON format to ResponseWriter
func (h *ElasticHandler) writeOutput(w http.ResponseWriter, r *http.Request, output any) {
	var err error
	var b []byte
	pretty := r.URL.Query().Get("pretty")
	if pretty == "true" || pretty == "" {
		b, err = json.MarshalIndent(output, "", "    ")
	} else {
		b, err = json.Marshal(output)
	}
	if err != nil {
		panic(err)
	}
	w.Write(b) //nolint:errcheck,gosec // best-effort JSON write to ResponseWriter
}

// Process output of request processing with respect to errors
func (h *ElasticHandler) processRequestOutput(w http.ResponseWriter, r *http.Request, output any, err error) {
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		if elasticErr, ok := errors.AsType[utils.ElasticError](err); ok {
			output = elasticErr.FormatErrorResponse()
		} else {
			panic(err)
		}
	}
	if recovered := recover(); recovered != nil {
		output = nil
	}
	h.writeOutput(w, r, output)
}

// Check if target method is presented in supportedMethods slice
func supportMethod(method string, supportedMethods []string) bool {
	return slices.Contains(supportedMethods, method)
}
