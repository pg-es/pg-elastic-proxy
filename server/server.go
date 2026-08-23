package server

import (
	"github.com/pg-es/pg-es-proxy/db"
	"github.com/pg-es/pg-es-proxy/utils"
)

// PGElasticServer represents an HTTP server which provides an ElasticSearch like API.
type PGElasticServer interface {
	GetConfiguration() utils.PGElasticConfig
	GetDBClient() *db.Client
	Start()
}
