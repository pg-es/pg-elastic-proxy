package server

import "net/http"

const (
	elasticProductHeader      = "X-Elastic-Product"
	elasticProductValue       = "Elasticsearch"
	elasticProductContentType = "application/json; charset=UTF-8"
)

// ElasticProductMiddleware sets default response headers that real Elasticsearch
// includes on every response: X-Elastic-Product and Content-Type. Headers are
// set before the next handler runs, so individual handlers can override
// Content-Type by calling w.Header().Set before writing the body.
func ElasticProductMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(elasticProductHeader, elasticProductValue)
		w.Header().Set("Content-Type", elasticProductContentType)
		next.ServeHTTP(w, r)
	})
}
