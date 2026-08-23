package db

import (
	"fmt"
	"strings"

	"github.com/go-pg/pg"
	"github.com/go-pg/pg/orm"
	"github.com/pg-es/pg-es-proxy/utils"
)

// Client is a database client connection.
type Client struct {
	connection *pg.DB
}

// Query is alias for orm.Query type.
// Used to implement other packages of the project without linking it to pg/orm.
type Query = orm.Query

// IndexRecord contains information about index stored in database.
type IndexRecord struct {
	Name    string
	Options string
}

// TypeRecord contains information about type stored in database.
type TypeRecord struct {
	Name      string
	IndexName string
	Options   string
}

// ElasticSearchDocument represents ElasticSearch document stored in database.
type ElasticSearchDocument struct {
	ID       string
	Document any
	Version  int
}

/*
 * General Client API
 */

// CreateClient creates an instance of Client with specified parameters.
// Doesn't check connection to DB server.
func CreateClient(config utils.PostgresConnectionConfig) (result *Client) {
	result = new(Client)
	result.connection = pg.Connect(&pg.Options{
		User:     config.User,
		Addr:     config.ServerAddress,
		Password: config.Password,
		Database: config.DBName,
	})

	return result
}

// InitializeSchema initializes system tables used by pg_elastic.
// Doesn't affect existing tables.
func (dbc *Client) InitializeSchema() error {
	for _, model := range []any{&IndexRecord{}, &TypeRecord{}} {
		err := dbc.connection.CreateTable(model, &orm.CreateTableOptions{IfNotExists: true})
		if err != nil {
			return fmt.Errorf("creating table: %w", err)
		}
	}

	return nil
}

// NewQuery creates Query instance for specified index and type.
func (dbc *Client) NewQuery(indexName, typeName string) *Query {
	tableName := fmt.Sprintf("%s_%s", indexName, typeName)

	return dbc.connection.Model().Table(tableName)
}

// ProcessSearchQuery does a processing of ElasticSearch-format query.
func (dbc *Client) ProcessSearchQuery(_, _ string, query *Query) ([]ElasticSearchDocument, error) {
	var documentObject []ElasticSearchDocument

	count, err := query.Count()
	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	if count == 0 {
		return nil, nil
	}

	err = query.Select(&documentObject)
	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	return documentObject, nil
}

/*
 * Indices API
 */

// CreateIndex creates an index record with specified options.
func (dbc *Client) CreateIndex(indexName, options string) (*IndexRecord, error) {
	var indexRecord IndexRecord

	indexSelectQuery := dbc.connection.Model(&IndexRecord{}).Where("Name = ?", indexName)

	count, err := indexSelectQuery.Count()
	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	if count == 0 {
		indexRecord = IndexRecord{Name: indexName, Options: options}

		err = dbc.connection.Insert(&indexRecord)
		if err != nil {
			return nil, utils.NewDBQueryError(err.Error())
		}
	} else {
		return nil, utils.NewResourceAlreadyExistsError("Index already exists")
	}

	err = indexSelectQuery.Select(&indexRecord)
	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	return &indexRecord, nil
}

// GetIndex gets an instance of existing index.
func (dbc *Client) GetIndex(indexName string) (*IndexRecord, error) {
	var indexRecord IndexRecord

	indexSelectQuery := dbc.connection.Model(&IndexRecord{}).Where("Name = ?", indexName)

	count, err := indexSelectQuery.Count()
	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	if count == 0 {
		return nil, nil
	}

	err = indexSelectQuery.Select(&indexRecord)
	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	return &indexRecord, nil
}

// FindIndices searches for indices using name pattern in ElasticSearch wildcard format.
func (dbc *Client) FindIndices(indexPattern string) ([]string, error) {
	var records []IndexRecord

	indexPattern = strings.ReplaceAll(indexPattern, "?", "_")
	indexPattern = strings.ReplaceAll(indexPattern, "*", "%")

	query := dbc.connection.Model(&IndexRecord{}).Where("name LIKE ?", indexPattern)

	err := query.Select(&records)
	if err != nil {
		return nil, fmt.Errorf("selecting indices: %w", err)
	}

	results := make([]string, 0, len(records))
	for _, v := range records {
		results = append(results, v.Name)
	}

	return results, nil
}

/*
 * Types API
 */

// CreateType creates a type record with specified options.
func (dbc *Client) CreateType(indexName, typeName, options string) (*TypeRecord, error) {
	var typeRecord TypeRecord

	typeSelectQuery := dbc.connection.Model(&TypeRecord{}).Where("Name = ?", typeName).Where("Index_Name = ?", indexName)

	count, err := typeSelectQuery.Count()
	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	if count == 0 {
		typeRecord = TypeRecord{Name: typeName, IndexName: indexName, Options: options}

		err = dbc.connection.Insert(&typeRecord)
		if err != nil {
			return nil, utils.NewDBQueryError(err.Error())
		}

		err = dbc.createDataTable(indexName, typeName)
		if err != nil {
			return nil, utils.NewDBQueryError(err.Error())
		}
	} else {
		return nil, utils.NewResourceAlreadyExistsError("Type already exists")
	}

	return &typeRecord, nil
}

// GetType gets an instance of existing type.
func (dbc *Client) GetType(indexName, typeName string) (*TypeRecord, error) {
	var typeRecord TypeRecord

	typeSelectQuery := dbc.connection.Model(&TypeRecord{}).Where("Name = ?", typeName).Where("Index_Name = ?", indexName)

	count, err := typeSelectQuery.Count()
	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	if count == 0 {
		return nil, nil
	}

	err = typeSelectQuery.Select(&typeRecord)
	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	return &typeRecord, nil
}

// UpdateTypeOptions updates options for existing type.
func (dbc *Client) UpdateTypeOptions(indexName, typeName, options string) (*TypeRecord, error) {
	_, err := dbc.connection.Model(&TypeRecord{}).Where("Name = ?", typeName).Where("Index_Name = ?", indexName).Set("options = ?", options).Update()
	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	return dbc.GetType(indexName, typeName)
}

// FindTypes searches for types using name pattern in ElasticSearch wildcard format.
func (dbc *Client) FindTypes(index, typePattern string) ([]string, error) {
	var records []TypeRecord

	typePattern = strings.ReplaceAll(typePattern, "?", "_")
	typePattern = strings.ReplaceAll(typePattern, "*", "%")

	query := dbc.connection.Model(&TypeRecord{}).Where("name LIKE ?", typePattern).Where("index_name = ?", index)

	err := query.Select(&records)
	if err != nil {
		return nil, fmt.Errorf("selecting types: %w", err)
	}

	results := make([]string, 0, len(records))
	for _, v := range records {
		results = append(results, v.Name)
	}

	return results, nil
}

/*
 * Documents API
 */

// CreateDocument creates a new document in database.
func (dbc *Client) CreateDocument(indexName, typeName, document, documentID string) (result *ElasticSearchDocument, err error) {
	index, err := dbc.GetIndex(indexName)
	if err != nil {
		return nil, err
	} else if index == nil {
		_, err = dbc.CreateIndex(indexName, "")
		if err != nil {
			return nil, err
		}
	}

	typeObject, err := dbc.GetType(indexName, typeName)
	if err != nil {
		return nil, err
	} else if typeObject == nil {
		_, err = dbc.CreateType(indexName, typeName, "")
		if err != nil {
			return nil, err
		}
	}

	if documentID == "" {
		result, err = dbc.insertDocument(indexName, typeName, document)
		if err != nil {
			return nil, err
		}
	} else {
		var documentExist bool

		documentExist, err = dbc.IsDocumentExists(indexName, typeName, documentID)
		if err != nil {
			return nil, err
		}

		if documentExist {
			return nil, utils.NewVersionConflictError(fmt.Sprintf("Document with ID %s already exists", documentID))
		}

		result, err = dbc.insertDocumentID(indexName, typeName, document, documentID)
	}

	return result, err
}

// GetDocument gets document specified by index, type, and ID.
func (dbc *Client) GetDocument(indexName, typeName, documentID string) (*ElasticSearchDocument, error) {
	var documentObject ElasticSearchDocument

	tableName := fmt.Sprintf("%s_%s", indexName, typeName)
	query := dbc.connection.Model().TableExpr(tableName).Where("id = ?", documentID)

	count, err := query.Count()
	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	if count == 0 {
		return nil, nil
	}

	err = query.Select(&documentObject)
	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	return &documentObject, nil
}

// IsDocumentExists checks for document existence in database.
func (dbc *Client) IsDocumentExists(indexName, typeName, documentID string) (bool, error) {
	if documentID == "" {
		return false, nil
	}

	tableName := fmt.Sprintf("%s_%s", indexName, typeName)

	count, err := dbc.connection.Model().TableExpr(tableName).Where("id = ?", documentID).Count()
	if err != nil {
		return false, utils.NewDBQueryError(err.Error())
	}

	return count == 1, nil
}

// UpdateDocument updates existing document in database.
func (dbc *Client) UpdateDocument(indexName, typeName, document, documentID string) (result *ElasticSearchDocument, err error) {
	documentExist, err := dbc.IsDocumentExists(indexName, typeName, documentID)
	if err != nil {
		return nil, err
	}

	if documentID != "" && documentExist {
		tableName := fmt.Sprintf("%s_%s", indexName, typeName)

		_, err := dbc.connection.Model().TableExpr(tableName).Set("document = ?", document).Set("version = version + 1").Where("id = ?", documentID).Update()
		if err != nil {
			return nil, utils.NewDBQueryError(err.Error())
		}

		result, err = dbc.GetDocument(indexName, typeName, documentID)
		if err != nil {
			return nil, err
		}
	} else {
		return nil, utils.NewDBQueryError(fmt.Sprintf("Document with ID %s doesn't exists", documentID))
	}

	return result, nil
}

// DeleteDocument deletes existing document in database.
func (dbc *Client) DeleteDocument(indexName, typeName, documentID string) (*ElasticSearchDocument, error) {
	documentObject, err := dbc.GetDocument(indexName, typeName, documentID)
	if err != nil {
		return nil, err
	}

	if documentObject != nil {
		tableName := fmt.Sprintf("%s_%s", indexName, typeName)

		_, err := dbc.connection.Model().TableExpr(tableName).Where("id = ?", documentID).Delete()
		if err != nil {
			return nil, utils.NewDBQueryError(err.Error())
		}
	}

	return documentObject, nil
}

/*
 * Documents processing helpers/internal methods
 */

// createDataTable creates a document storage table for specified index and type.
func (dbc *Client) createDataTable(indexName, typeName string) error {
	queryString := fmt.Sprintf("CREATE SEQUENCE %s_%s_id_seq;", indexName, typeName)

	_, err := dbc.connection.Exec(queryString)
	if err != nil {
		return utils.NewDBQueryError(err.Error())
	}

	queryString = fmt.Sprintf(
		"CREATE TABLE %s_%s(id VARCHAR(128) PRIMARY KEY DEFAULT nextval('%s_%s_id_seq'), document JSONB NOT NULL, version integer);",
		indexName, typeName, indexName, typeName,
	)

	_, err = dbc.connection.Exec(queryString)
	if err != nil {
		return utils.NewDBQueryError(err.Error())
	}

	return nil
}

// insertDocument inserts a new document with default ID.
func (dbc *Client) insertDocument(indexName, typeName, document string) (*ElasticSearchDocument, error) {
	documentObject := &ElasticSearchDocument{Document: document, Version: 1}

	queryString := fmt.Sprintf(
		"INSERT INTO %s_%s (id, document, version) VALUES(DEFAULT, '%s', %d) RETURNING id;",
		indexName, typeName, document, 1,
	)

	_, err := dbc.connection.Query(documentObject, queryString)
	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	return documentObject, nil
}

// insertDocumentID inserts a new document with specified ID.
func (dbc *Client) insertDocumentID(indexName, typeName, document, documentID string) (*ElasticSearchDocument, error) {
	documentObject := &ElasticSearchDocument{documentID, document, 1}

	queryString := fmt.Sprintf(
		"INSERT INTO %s_%s (id, document, version) VALUES(%s, '%s', %d);",
		indexName, typeName, documentID, document, 1,
	)

	_, err := dbc.connection.Exec(queryString)
	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	return documentObject, nil
}
