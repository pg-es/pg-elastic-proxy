package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pg-es/pg-es-proxy/db"
	"github.com/pg-es/pg-es-proxy/server"
	"github.com/pg-es/pg-es-proxy/utils"
)

type bulkResponse struct {
	Took   int   `json:"took"`
	Errors bool  `json:"errors"`
	Items  []any `json:"items"`
}

type bulkPutCommandResponse struct {
	documentPutResponse
	Status int `json:"status"`
}

type bulkGetCommandResponse struct {
	documentGetResponse
	Status int `json:"status"`
}

// BulkHandler handles ElasticSearch bulk requests
func BulkHandler(_ string, r *http.Request, srv server.PGElasticServer) (response any, err error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, utils.NewInternalIOError(err.Error())
	}

	str := string(body)
	bulkCommands := strings.Split(str, "\n")
	return ProcessBulkQuery(bulkCommands, srv)
}

type bulkDescriptor struct {
	indexName string
	typeName  string
	id        string
}

func parseBulkDescriptor(v any) (*bulkDescriptor, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, utils.NewJSONWrongFormatError("Wrong JSON format")
	}
	desc := &bulkDescriptor{}
	if idx, ok := m["_index"].(string); ok {
		desc.indexName = idx
	}
	if typ, ok := m["_type"].(string); ok {
		desc.typeName = typ
	}
	if id, ok := m["_id"].(string); ok {
		desc.id = id
	}
	return desc, nil
}

func resolveAndUpsertDocument(dbClient *db.Client, desc *bulkDescriptor, document string) (*db.ElasticSearchDocument, error) {
	typeNames, err := dbClient.FindTypes(desc.indexName, desc.typeName)
	if err != nil {
		return nil, err
	}
	if len(typeNames) > 0 {
		existingDoc, err := dbClient.GetDocument(desc.indexName, desc.typeName, desc.id)
		if err != nil {
			return nil, err
		}
		if existingDoc != nil {
			return dbClient.UpdateDocument(desc.indexName, desc.typeName, document, desc.id)
		}
	}
	return dbClient.CreateDocument(desc.indexName, desc.typeName, document, desc.id)
}

func executeBulkCommand(k string, desc *bulkDescriptor, nextCommand string, srv server.PGElasticServer) (*db.ElasticSearchDocument, bool, error) {
	dbClient := srv.GetDBClient()
	switch k {
	case "index":
		doc, err := resolveAndUpsertDocument(dbClient, desc, nextCommand)
		return doc, true, err
	case "create":
		doc, err := dbClient.CreateDocument(desc.indexName, desc.typeName, nextCommand, desc.id)
		return doc, true, err
	case "update":
		doc, err := dbClient.UpdateDocument(desc.indexName, desc.typeName, nextCommand, desc.id)
		return doc, true, err
	case "delete":
		doc, err := dbClient.DeleteDocument(desc.indexName, desc.typeName, desc.id)
		return doc, false, err
	default:
		return nil, false, nil
	}
}

func buildPutCommandResponse(doc *db.ElasticSearchDocument, indexName, typeName string) bulkPutCommandResponse {
	result := "updated"
	if doc.Version == 1 {
		result = "created"
	}
	return bulkPutCommandResponse{
		documentPutResponse{
			Shards:  shardInfo{1, 0, 1},
			Index:   indexName,
			Type:    typeName,
			ID:      doc.ID,
			Version: doc.Version,
			Created: doc.Version == 1,
			Result:  result,
		},
		200,
	}
}

func buildDeleteCommandResponse(doc *db.ElasticSearchDocument, indexName, typeName string) bulkGetCommandResponse {
	cmd := bulkGetCommandResponse{
		documentGetResponse{
			Index: indexName,
			Type:  typeName,
			Found: doc != nil,
		},
		200,
	}
	if doc != nil {
		cmd.Version = doc.Version
		cmd.Document = doc.Document
		cmd.ID = doc.ID
	}
	return cmd
}

func buildBulkItemResponse(k string, doc *db.ElasticSearchDocument, indexName, typeName string, err error) (item map[string]any, hasError bool, fatalErr error) {
	responseCommand := make(map[string]any)
	if err != nil {
		if elasticErr, ok := errors.AsType[utils.ElasticError](err); ok {
			responseCommand[k] = utils.NewElasticErrorBulk(elasticErr, indexName, "1", "1").FormatErrorResponse()
			return responseCommand, true, nil
		}
		return nil, true, err
	}

	switch k {
	case "index", "create", "update":
		if doc != nil {
			responseCommand[k] = buildPutCommandResponse(doc, indexName, typeName)
		} else {
			responseCommand[k] = utils.NewElasticErrorBulk(utils.NewInternalError("Unknown error"), indexName, "1", "1").FormatErrorResponse()
			return responseCommand, true, nil
		}
	case "delete":
		responseCommand["delete"] = buildDeleteCommandResponse(doc, indexName, typeName)
	}

	return responseCommand, false, nil
}

// ProcessBulkQuery processes a bulk query
func ProcessBulkQuery(rawQuery []string, srv server.PGElasticServer) (any, error) {
	response := bulkResponse{}
	skip := false
	startTime := time.Now()

	for i, command := range rawQuery {
		if skip || command == "" {
			skip = false
			continue
		}

		var parsedJSON map[string]any
		if err := json.Unmarshal([]byte(command), &parsedJSON); err != nil {
			return nil, utils.NewJSONWrongFormatError(err.Error())
		}

		for k, v := range parsedJSON {
			desc, err := parseBulkDescriptor(v)
			if err != nil {
				return nil, err
			}

			nextCommand := ""
			if i+1 < len(rawQuery) {
				nextCommand = rawQuery[i+1]
			}

			doc, shouldSkip, err := executeBulkCommand(k, desc, nextCommand, srv)
			skip = shouldSkip

			item, hasError, fatalErr := buildBulkItemResponse(k, doc, desc.indexName, desc.typeName, err)
			if fatalErr != nil {
				return nil, fatalErr
			}
			if hasError {
				response.Errors = true
			}

			response.Items = append(response.Items, item)
		}
	}

	response.Took = int(time.Since(startTime).Nanoseconds() / 1000000)
	return response, nil
}
