// Package server defines the interface between the HTTP routing layer and
// the Elasticsearch-compatible API handlers.
package server

import (
	"context"
	"net/http"

	"github.com/pg-es/pg-es-proxy/db"
)

// Handler processes an API request and returns a response body or error.
type Handler func(*http.Request, PGElasticServer) (any, error)

// PGElasticServer represents an HTTP server which provides an ElasticSearch like API.
type PGElasticServer interface {
	GetDBClient() *db.Client
	Start(ctx context.Context) error
}
