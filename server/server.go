package server

import (
	"net/http"

	"github.com/pg-es/pg-es-proxy/db"
	"github.com/pg-es/pg-es-proxy/utils"
)

// Handler processes an API request and returns a response body or error.
type Handler func(*http.Request, PGElasticServer) (any, error)

// PGElasticServer represents an HTTP server which provides an ElasticSearch like API.
type PGElasticServer interface {
	GetConfiguration() utils.PGElasticConfig
	GetDBClient() *db.Client
	Start()
}
