package api

import (
	"fmt"
	"io"
	"net/http"
	"regexp"

	"github.com/pg-es/pg-es-proxy/server"
	"github.com/pg-es/pg-es-proxy/utils"
)

type indexPutResponse struct {
	Acknowledged       bool `json:"acknowledged"`
	ShardsAcknowledged bool `json:"shards_acknowledged"`
}

type typePutResponse struct {
	Acknowledged bool `json:"acknowledged"`
}

var (
	putTypeMappingPattern = regexp.MustCompile(`/(?P<index>\w+)/_mapping/(?P<type>\w+)`)
	indexHandlerPattern   = regexp.MustCompile(`/(?P<index>\w+)`)
)

// PutIndexHandler process a response to put a new index into database
func PutIndexHandler(endpoint string, r *http.Request, srv server.PGElasticServer) (any, error) {
	indexName := indexHandlerPattern.ReplaceAllString(endpoint, "${index}")
	optionsBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, utils.NewInternalIOError(err.Error())
	}
	options := string(optionsBytes)

	_, err = srv.GetDBClient().CreateIndex(indexName, options)
	if err != nil {
		return nil, err
	}

	return indexPutResponse{true, true}, nil
}

// HeadIndexHandler process a response to check is an index exists in database
func HeadIndexHandler(endpoint string, _ *http.Request, srv server.PGElasticServer) (any, error) {
	indexName := indexHandlerPattern.ReplaceAllString(endpoint, "${index}")
	indexRecord, err := srv.GetDBClient().GetIndex(indexName)
	if err != nil {
		fmt.Println(err)
		return false, err
	}
	return indexRecord != nil, nil
}

// PutTypeMapping process a response to put a type mapping into database
func PutTypeMapping(endpoint string, r *http.Request, srv server.PGElasticServer) (any, error) {
	indexName := putTypeMappingPattern.ReplaceAllString(endpoint, "${index}")
	typeName := putTypeMappingPattern.ReplaceAllString(endpoint, "${type}")
	optionsBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, utils.NewInternalIOError(err.Error())
	}
	options := string(optionsBytes)

	typeObject, err := srv.GetDBClient().GetType(indexName, typeName)
	if err != nil {
		return nil, err
	}

	if typeObject != nil {
		_, err = srv.GetDBClient().UpdateTypeOptions(indexName, typeName, options)
	} else {
		_, err = srv.GetDBClient().CreateType(indexName, typeName, options)
	}

	if err != nil {
		return nil, err
	}
	return typePutResponse{true}, nil
}
