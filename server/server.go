package server

import (
	"net/http"

	"github.com/pg-es/pg-es-proxy/db"
	"github.com/pg-es/pg-es-proxy/utils"
)

// ElasticRequestHandler handles requests where the full URL path is passed through.
type ElasticRequestHandler func(string, *http.Request, PGElasticServer) (any, error)

// ElasticEndpointRequestHandler handles requests with URL: /<index>/<type>/<endpoint>.
type ElasticEndpointRequestHandler func(string, string, string, *http.Request, PGElasticServer) (any, error)

// PGElasticServer represents an HTTP server which provides an ElasticSearch like API.
type PGElasticServer interface {
	GetConfiguration() utils.PGElasticConfig
	GetDBClient() *db.Client
	Start()
}
