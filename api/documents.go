package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/pg-es/pg-es-proxy/api/search"
	"github.com/pg-es/pg-es-proxy/db"
	"github.com/pg-es/pg-es-proxy/server"
	"github.com/pg-es/pg-es-proxy/utils"
)

type shardInfo struct {
	Total      int `json:"total"`
	Failed     int `json:"failed"`
	Successful int `json:"successful"`
}

type searchHits struct {
	MaxScore float32                  `json:"max_score"`
	Total    int                      `json:"total"`
	Hits     []documentSearchResponse `json:"hits"`
}

type documentPutResponse struct {
	Shards  shardInfo `json:"_shards"`
	Index   string    `json:"_index"`
	Type    string    `json:"_type"`
	ID      string    `json:"_id"`
	Version int       `json:"_version"`
	Created bool      `json:"created"`
	Result  string    `json:"result"`
}

type documentGetResponse struct {
	Index    string `json:"_index"`
	Type     string `json:"_type"`
	ID       string `json:"_id"`
	Version  int    `json:"_version"`
	Found    bool   `json:"found"`
	Document any    `json:"_source"`
}

type documentSearchResponse struct {
	Index    string  `json:"_index"`
	Type     string  `json:"_type"`
	ID       string  `json:"_id"`
	Score    float32 `json:"_score"`
	Document any     `json:"_source"`
}

type searchResponse struct {
	Took     int        `json:"took"`
	TimedOut bool       `json:"timed_out"`
	Shards   shardInfo  `json:"_shards"`
	Hits     searchHits `json:"hits"`
}

func formatDocumentSearchResponse(index, typeName string, doc db.ElasticSearchDocument) documentSearchResponse {
	return documentSearchResponse{
		Index:    index,
		Type:     typeName,
		ID:       doc.ID,
		Score:    1.0,
		Document: doc.Document,
	}
}

func putOrUpdateDocument(s server.PGElasticServer, index, typeName, body, documentID string) (*db.ElasticSearchDocument, error) {
	exists, err := s.GetDBClient().IsDocumentExists(index, typeName, documentID)
	if err != nil {
		return nil, err
	}
	if exists {
		return s.GetDBClient().UpdateDocument(index, typeName, body, documentID)
	}
	return s.GetDBClient().CreateDocument(index, typeName, body, documentID)
}

// PutDocumentHandler handles request to put document into storage
func PutDocumentHandler(index, typeName, endpoint string, r *http.Request, s server.PGElasticServer) (any, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, utils.NewInternalIOError(err.Error())
	}

	var documentObject *db.ElasticSearchDocument
	if endpoint == "" {
		documentObject, err = s.GetDBClient().CreateDocument(index, typeName, string(body), "")
	} else {
		documentObject, err = putOrUpdateDocument(s, index, typeName, string(body), endpoint)
	}
	if err != nil {
		return nil, err
	}

	response := documentPutResponse{
		Shards:  shardInfo{1, 0, 1},
		Index:   index,
		Type:    typeName,
		ID:      documentObject.ID,
		Version: documentObject.Version,
		Created: documentObject.Version == 1,
		Result: func() string {
			if documentObject.Version == 1 {
				return "created"
			}
			return "updated"
		}(),
	}

	return response, nil
}

// GetDocumentHandler handles request to get document from storage
func GetDocumentHandler(index, typeName, endpoint string, _ *http.Request, s server.PGElasticServer) (response any, err error) {
	documentID := endpoint
	documentObject, err := s.GetDBClient().GetDocument(index, typeName, documentID)
	if err != nil {
		return nil, err
	}
	if documentObject != nil {
		response = documentGetResponse{
			Index:    index,
			Type:     typeName,
			ID:       documentObject.ID,
			Version:  documentObject.Version,
			Found:    true,
			Document: documentObject.Document,
		}
	} else {
		response = documentGetResponse{
			Index: index,
			Type:  typeName,
			Found: false,
		}
	}
	return response, nil
}

// DeleteDocumentHandler handles request to delete document from storage
func DeleteDocumentHandler(index, typeName, endpoint string, _ *http.Request, s server.PGElasticServer) (response any, err error) {
	documentID := endpoint
	documentObject, err := s.GetDBClient().DeleteDocument(index, typeName, documentID)
	if err != nil {
		return nil, err
	}
	if documentObject != nil {
		response = documentGetResponse{
			Index:    index,
			Type:     typeName,
			ID:       documentObject.ID,
			Version:  documentObject.Version,
			Found:    true,
			Document: documentObject.Document,
		}
	} else {
		response = documentGetResponse{
			Index: index,
			Type:  typeName,
			Found: false,
		}
	}
	return response, nil
}

func executeSearchQuery(s server.PGElasticServer, index, typeName string, queryBody map[string]any, startTime time.Time) (*searchResponse, error) {
	var typeMapping map[string]any

	// Get type mapping from system type record
	docType, err := s.GetDBClient().GetType(index, typeName)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(docType.Options), &typeMapping); err != nil {
		return nil, fmt.Errorf("unmarshaling type mapping: %w", err)
	}

	query := s.GetDBClient().NewQuery(index, typeName)
	search.ParseSearchQuery(queryBody, query, typeMapping)
	docs, err := s.GetDBClient().ProcessSearchQuery(index, typeName, query)
	if err != nil {
		return nil, err
	}

	resp := searchResponse{
		Took:     1,
		TimedOut: false,
		Shards:   shardInfo{1, 0, 1},
		Hits: searchHits{
			MaxScore: 0,
			Total:    0,
			Hits:     []documentSearchResponse{},
		},
	}
	for _, doc := range docs {
		docResponse := formatDocumentSearchResponse(index, typeName, doc)
		resp.Hits.Hits = append(resp.Hits.Hits, docResponse)
		if resp.Hits.MaxScore < docResponse.Score {
			resp.Hits.MaxScore = docResponse.Score
		}
		resp.Hits.Total++
	}
	resp.Took = int(time.Since(startTime).Nanoseconds() / 1000000)
	return &resp, nil
}

// FindDocumentHandler handles request to find document on storage
func FindDocumentHandler(indexPattern, typePattern, _ string, r *http.Request, s server.PGElasticServer) (response any, err error) {
	startTime := time.Now()
	var parsedQuery any
	indices, err := s.GetDBClient().FindIndices(indexPattern)
	if err != nil {
		return nil, utils.NewInternalError(err.Error())
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, utils.NewInternalIOError(err.Error())
	}

	err = json.Unmarshal(body, &parsedQuery)
	if err != nil {
		return nil, utils.NewJSONWrongFormatError(err.Error())
	}

	queryMap, ok := parsedQuery.(map[string]any)
	if !ok {
		return nil, utils.NewJSONWrongFormatError("expected JSON object")
	}

	for _, index := range indices {
		types, err := s.GetDBClient().FindTypes(index, typePattern)
		if err != nil {
			return nil, utils.NewInternalError(err.Error())
		}
		for _, typeName := range types {
			queryBody, ok := queryMap["query"].(map[string]any)
			if !ok {
				continue
			}
			resp, err := executeSearchQuery(s, index, typeName, queryBody, startTime)
			if err != nil {
				return nil, err
			}
			return resp, nil
		}
	}
	return nil, utils.NewIllegalQueryError("Illegal search query")
}

// FindIndexDocumentHandler handles request to find document of any type on storage
func FindIndexDocumentHandler(endpoint string, r *http.Request, s server.PGElasticServer) (response any, err error) {
	indexHandlerPattern := regexp.MustCompile(`^/(?P<index>\w+)/_search`)
	indexName := indexHandlerPattern.ReplaceAllString(endpoint, "${index}")
	return FindDocumentHandler(indexName, "*", endpoint, r, s)
}
