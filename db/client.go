// Package db provides PostgreSQL-backed storage for indices, types, and
// documents exposed through the Elasticsearch-compatible API.
package db

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/huandu/go-sqlbuilder"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/pg-es/pg-es-proxy/internal/pg"
	"github.com/pg-es/pg-es-proxy/utils"
)

// Client is a database client connection.
type Client struct {
	pool *pgxpool.Pool
}

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

// NewClient creates an instance of Client backed by the given connection pool.
// Doesn't check connection to the DB server.
func NewClient(pool *pgxpool.Pool) *Client {
	return &Client{pool: pool}
}

// Close closes the underlying connection pool.
func (dbc *Client) Close() {
	dbc.pool.Close()
}

// InitializeSchema initializes system tables used by pg_elastic.
// Doesn't affect existing tables.
func (dbc *Client) InitializeSchema(ctx context.Context) error {
	for _, createSQL := range buildSystemTableDDL() {
		_, err := dbc.pool.Exec(ctx, createSQL)
		if err != nil {
			return fmt.Errorf("creating table: %w", err)
		}
	}

	return nil
}

// buildSystemTableDDL returns the CREATE TABLE statements for the system
// tables. Column names and types mirror what the previous ORM generated
// before the pgx migration: snake_case column names and text columns for
// string struct fields.
func buildSystemTableDDL() []string {
	indexRecords := sqlbuilder.PostgreSQL.NewCreateTableBuilder()
	indexRecords.CreateTable(`"index_records"`).IfNotExists()
	indexRecords.Define(`"name" text`)
	indexRecords.Define(`"options" text`)
	indexRecords.Define(`PRIMARY KEY ("name")`)

	typeRecords := sqlbuilder.PostgreSQL.NewCreateTableBuilder()
	typeRecords.CreateTable(`"type_records"`).IfNotExists()
	typeRecords.Define(`"name" text`)
	typeRecords.Define(`"index_name" text`)
	typeRecords.Define(`"options" text`)
	typeRecords.Define(`PRIMARY KEY ("name", "index_name")`)

	return []string{indexRecords.String(), typeRecords.String()}
}

// ProcessSearchQuery does a processing of ElasticSearch-format query.
//
// tableName is the combined "<index>_<type>" document table name; it is
// validated as a single SQL identifier before use. whereExpr is a
// go-sqlbuilder format expression: "$?" placeholders reference the values in
// args in order (e.g. "document @> $? AND version > $?"). A literal "$" must
// be written as "$$" inside whereExpr.
func (dbc *Client) ProcessSearchQuery(ctx context.Context, tableName, whereExpr string, args []any) ([]ElasticSearchDocument, error) {
	table, err := quotedTableName(tableName)
	if err != nil {
		return nil, err
	}

	countSQL, countArgs := buildSearchCount(table, whereExpr, args)

	var count int

	err = dbc.pool.QueryRow(ctx, countSQL, countArgs...).Scan(&count)
	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	if count == 0 {
		return nil, nil
	}

	selectSQL, selectArgs := buildSearchSelect(table, whereExpr, args)

	rows, err := dbc.pool.Query(ctx, selectSQL, selectArgs...)
	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}
	defer rows.Close()

	documents := make([]ElasticSearchDocument, 0, count)

	for rows.Next() {
		var document ElasticSearchDocument

		err = rows.Scan(&document.ID, &document.Document, &document.Version)
		if err != nil {
			return nil, utils.NewDBQueryError(err.Error())
		}

		documents = append(documents, document)
	}

	err = rows.Err()
	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	return documents, nil
}

// buildSearchCount builds the COUNT query for a search.
func buildSearchCount(table, whereExpr string, args []any) (sql string, sqlArgs []any) {
	sb := sqlbuilder.PostgreSQL.NewSelectBuilder()
	sb.Select("COUNT(*)").From(table)

	if whereExpr != "" {
		sb.Where(sb.Var(sqlbuilder.Build(whereExpr, args...)))
	}

	sql, sqlArgs = sb.Build()

	return sql, sqlArgs
}

// buildSearchSelect builds the SELECT query for a search.
func buildSearchSelect(table, whereExpr string, args []any) (sql string, sqlArgs []any) {
	sb := sqlbuilder.PostgreSQL.NewSelectBuilder()
	sb.Select("id", "document", "version").From(table)

	if whereExpr != "" {
		sb.Where(sb.Var(sqlbuilder.Build(whereExpr, args...)))
	}

	sql, sqlArgs = sb.Build()

	return sql, sqlArgs
}

/*
 * Indices API
 */

// CreateIndex creates an index record with specified options.
func (dbc *Client) CreateIndex(ctx context.Context, indexName, options string) (*IndexRecord, error) {
	countSQL, countArgs := buildIndexCount(indexName)

	var count int

	err := dbc.pool.QueryRow(ctx, countSQL, countArgs...).Scan(&count)
	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	if count > 0 {
		return nil, utils.NewResourceAlreadyExistsError("Index already exists")
	}

	insertSQL, insertArgs := buildIndexInsert(indexName, options)

	_, err = dbc.pool.Exec(ctx, insertSQL, insertArgs...)
	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	return dbc.GetIndex(ctx, indexName)
}

// GetIndex gets an instance of existing index.
func (dbc *Client) GetIndex(ctx context.Context, indexName string) (*IndexRecord, error) {
	selectSQL, selectArgs := buildIndexSelect(indexName)

	var indexRecord IndexRecord

	err := dbc.pool.QueryRow(ctx, selectSQL, selectArgs...).Scan(&indexRecord.Name, &indexRecord.Options)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	return &indexRecord, nil
}

// FindIndices searches for indices using name pattern in ElasticSearch wildcard format.
func (dbc *Client) FindIndices(ctx context.Context, indexPattern string) ([]string, error) {
	indexPattern = strings.ReplaceAll(indexPattern, "?", "_")
	indexPattern = strings.ReplaceAll(indexPattern, "*", "%")

	selectSQL, selectArgs := buildIndexNamesLike(indexPattern)

	return dbc.selectNames(ctx, selectSQL, selectArgs, "selecting indices")
}

// buildIndexCount builds the existence-count query for an index.
func buildIndexCount(indexName string) (sql string, args []any) {
	sb := sqlbuilder.PostgreSQL.NewSelectBuilder()
	sb.Select("COUNT(*)").From(`"index_records"`).Where(sb.Equal("name", indexName))

	sql, args = sb.Build()

	return sql, args
}

// buildIndexSelect builds the SELECT query for a single index.
func buildIndexSelect(indexName string) (sql string, args []any) {
	sb := sqlbuilder.PostgreSQL.NewSelectBuilder()
	sb.Select("name", "options").From(`"index_records"`).Where(sb.Equal("name", indexName))

	sql, args = sb.Build()

	return sql, args
}

// buildIndexInsert builds the INSERT statement for a new index.
func buildIndexInsert(indexName, options string) (sql string, args []any) {
	ib := sqlbuilder.PostgreSQL.NewInsertBuilder()
	ib.InsertInto(`"index_records"`).Cols("name", "options").Values(indexName, options)

	sql, args = ib.Build()

	return sql, args
}

// buildIndexNamesLike builds the SELECT query for indices matching a
// SQL LIKE pattern.
func buildIndexNamesLike(pattern string) (sql string, args []any) {
	sb := sqlbuilder.PostgreSQL.NewSelectBuilder()
	sb.Select("name").From(`"index_records"`).Where(sb.Like("name", pattern))

	sql, args = sb.Build()

	return sql, args
}

/*
 * Types API
 */

// CreateType creates a type record with specified options.
func (dbc *Client) CreateType(ctx context.Context, indexName, typeName, options string) (*TypeRecord, error) {
	countSQL, countArgs := buildTypeCount(indexName, typeName)

	var count int

	err := dbc.pool.QueryRow(ctx, countSQL, countArgs...).Scan(&count)
	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	if count > 0 {
		return nil, utils.NewResourceAlreadyExistsError("Type already exists")
	}

	insertSQL, insertArgs := buildTypeInsert(indexName, typeName, options)

	_, err = dbc.pool.Exec(ctx, insertSQL, insertArgs...)
	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	err = dbc.createDataTable(ctx, indexName, typeName)
	if err != nil {
		return nil, err
	}

	return &TypeRecord{Name: typeName, IndexName: indexName, Options: options}, nil
}

// GetType gets an instance of existing type.
func (dbc *Client) GetType(ctx context.Context, indexName, typeName string) (*TypeRecord, error) {
	selectSQL, selectArgs := buildTypeSelect(indexName, typeName)

	var typeRecord TypeRecord

	err := dbc.pool.QueryRow(ctx, selectSQL, selectArgs...).Scan(&typeRecord.Name, &typeRecord.IndexName, &typeRecord.Options)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	return &typeRecord, nil
}

// UpdateTypeOptions updates options for existing type.
func (dbc *Client) UpdateTypeOptions(ctx context.Context, indexName, typeName, options string) (*TypeRecord, error) {
	updateSQL, updateArgs := buildTypeOptionsUpdate(indexName, typeName, options)

	_, err := dbc.pool.Exec(ctx, updateSQL, updateArgs...)
	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	return dbc.GetType(ctx, indexName, typeName)
}

// FindTypes searches for types using name pattern in ElasticSearch wildcard format.
func (dbc *Client) FindTypes(ctx context.Context, indexName, typePattern string) ([]string, error) {
	typePattern = strings.ReplaceAll(typePattern, "?", "_")
	typePattern = strings.ReplaceAll(typePattern, "*", "%")

	selectSQL, selectArgs := buildTypeNamesLike(indexName, typePattern)

	return dbc.selectNames(ctx, selectSQL, selectArgs, "selecting types")
}

// buildTypeCount builds the existence-count query for a type.
func buildTypeCount(indexName, typeName string) (sql string, args []any) {
	sb := sqlbuilder.PostgreSQL.NewSelectBuilder()
	sb.Select("COUNT(*)").From(`"type_records"`)
	sb.Where(sb.Equal("name", typeName), sb.Equal("index_name", indexName))

	sql, args = sb.Build()

	return sql, args
}

// buildTypeSelect builds the SELECT query for a single type.
func buildTypeSelect(indexName, typeName string) (sql string, args []any) {
	sb := sqlbuilder.PostgreSQL.NewSelectBuilder()
	sb.Select("name", "index_name", "options").From(`"type_records"`)
	sb.Where(sb.Equal("name", typeName), sb.Equal("index_name", indexName))

	sql, args = sb.Build()

	return sql, args
}

// buildTypeInsert builds the INSERT statement for a new type.
func buildTypeInsert(indexName, typeName, options string) (sql string, args []any) {
	ib := sqlbuilder.PostgreSQL.NewInsertBuilder()
	ib.InsertInto(`"type_records"`).Cols("name", "index_name", "options").Values(typeName, indexName, options)

	sql, args = ib.Build()

	return sql, args
}

// buildTypeOptionsUpdate builds the UPDATE statement for type options.
func buildTypeOptionsUpdate(indexName, typeName, options string) (sql string, args []any) {
	ub := sqlbuilder.PostgreSQL.NewUpdateBuilder()
	ub.Update(`"type_records"`).Set(ub.Assign("options", options))
	ub.Where(ub.Equal("name", typeName), ub.Equal("index_name", indexName))

	sql, args = ub.Build()

	return sql, args
}

// buildTypeNamesLike builds the SELECT query for types matching a SQL LIKE
// pattern within an index.
func buildTypeNamesLike(indexName, pattern string) (sql string, args []any) {
	sb := sqlbuilder.PostgreSQL.NewSelectBuilder()
	sb.Select("name").From(`"type_records"`)
	sb.Where(sb.Like("name", pattern), sb.Equal("index_name", indexName))

	sql, args = sb.Build()

	return sql, args
}

/*
 * Documents API
 */

// CreateDocument creates a new document in database.
func (dbc *Client) CreateDocument(ctx context.Context, indexName, typeName, document, documentID string) (*ElasticSearchDocument, error) {
	index, err := dbc.GetIndex(ctx, indexName)
	if err != nil {
		return nil, err
	}

	if index == nil {
		_, err = dbc.CreateIndex(ctx, indexName, "")
		if err != nil {
			return nil, err
		}
	}

	typeObject, err := dbc.GetType(ctx, indexName, typeName)
	if err != nil {
		return nil, err
	}

	if typeObject == nil {
		_, err = dbc.CreateType(ctx, indexName, typeName, "")
		if err != nil {
			return nil, err
		}
	}

	if documentID == "" {
		return dbc.insertDocument(ctx, indexName, typeName, document)
	}

	documentExist, err := dbc.IsDocumentExists(ctx, indexName, typeName, documentID)
	if err != nil {
		return nil, err
	}

	if documentExist {
		return nil, utils.NewVersionConflictError(fmt.Sprintf("Document with ID %s already exists", documentID))
	}

	return dbc.insertDocumentID(ctx, indexName, typeName, document, documentID)
}

// GetDocument gets document specified by index, type, and ID.
func (dbc *Client) GetDocument(ctx context.Context, indexName, typeName, documentID string) (*ElasticSearchDocument, error) {
	table, err := documentTableName(indexName, typeName)
	if err != nil {
		return nil, err
	}

	selectSQL, selectArgs := buildDocumentSelect(table, documentID)

	var document ElasticSearchDocument

	err = dbc.pool.QueryRow(ctx, selectSQL, selectArgs...).Scan(&document.ID, &document.Document, &document.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}

	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	return &document, nil
}

// IsDocumentExists checks for document existence in database.
func (dbc *Client) IsDocumentExists(ctx context.Context, indexName, typeName, documentID string) (bool, error) {
	if documentID == "" {
		return false, nil
	}

	table, err := documentTableName(indexName, typeName)
	if err != nil {
		return false, err
	}

	countSQL, countArgs := buildDocumentCount(table, documentID)

	var count int

	err = dbc.pool.QueryRow(ctx, countSQL, countArgs...).Scan(&count)
	if err != nil {
		return false, utils.NewDBQueryError(err.Error())
	}

	return count == 1, nil
}

// UpdateDocument updates existing document in database.
func (dbc *Client) UpdateDocument(ctx context.Context, indexName, typeName, document, documentID string) (*ElasticSearchDocument, error) {
	documentExist, err := dbc.IsDocumentExists(ctx, indexName, typeName, documentID)
	if err != nil {
		return nil, err
	}

	if documentID == "" || !documentExist {
		return nil, utils.NewDBQueryError(fmt.Sprintf("Document with ID %s doesn't exists", documentID))
	}

	table, err := documentTableName(indexName, typeName)
	if err != nil {
		return nil, err
	}

	updateSQL, updateArgs := buildDocumentUpdate(table, document, documentID)

	_, err = dbc.pool.Exec(ctx, updateSQL, updateArgs...)
	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	return dbc.GetDocument(ctx, indexName, typeName, documentID)
}

// DeleteDocument deletes existing document in database.
func (dbc *Client) DeleteDocument(ctx context.Context, indexName, typeName, documentID string) (*ElasticSearchDocument, error) {
	documentObject, err := dbc.GetDocument(ctx, indexName, typeName, documentID)
	if err != nil {
		return nil, err
	}

	if documentObject == nil {
		return nil, nil
	}

	table, err := documentTableName(indexName, typeName)
	if err != nil {
		return nil, err
	}

	deleteSQL, deleteArgs := buildDocumentDelete(table, documentID)

	_, err = dbc.pool.Exec(ctx, deleteSQL, deleteArgs...)
	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	return documentObject, nil
}

// buildDocumentCount builds the existence-count query for a document.
func buildDocumentCount(table, documentID string) (sql string, args []any) {
	sb := sqlbuilder.PostgreSQL.NewSelectBuilder()
	sb.Select("COUNT(*)").From(table).Where(sb.Equal("id", documentID))

	sql, args = sb.Build()

	return sql, args
}

// buildDocumentSelect builds the SELECT query for a single document.
func buildDocumentSelect(table, documentID string) (sql string, args []any) {
	sb := sqlbuilder.PostgreSQL.NewSelectBuilder()
	sb.Select("id", "document", "version").From(table).Where(sb.Equal("id", documentID))

	sql, args = sb.Build()

	return sql, args
}

// buildDocumentInsert builds the INSERT statement for a document with a
// sequence-generated ID.
func buildDocumentInsert(table, document string) (sql string, args []any) {
	ib := sqlbuilder.PostgreSQL.NewInsertBuilder()
	ib.InsertInto(table).Cols("id", "document", "version")
	ib.Values(sqlbuilder.Raw("DEFAULT"), document, 1).Returning("id")

	sql, args = ib.Build()

	return sql, args
}

// buildDocumentInsertWithID builds the INSERT statement for a document with
// an explicit ID.
func buildDocumentInsertWithID(table, document, documentID string) (sql string, args []any) {
	ib := sqlbuilder.PostgreSQL.NewInsertBuilder()
	ib.InsertInto(table).Cols("id", "document", "version").Values(documentID, document, 1)

	sql, args = ib.Build()

	return sql, args
}

// buildDocumentUpdate builds the UPDATE statement for a document.
func buildDocumentUpdate(table, document, documentID string) (sql string, args []any) {
	ub := sqlbuilder.PostgreSQL.NewUpdateBuilder()
	ub.Update(table).Set(ub.Assign("document", document), "version = version + 1")
	ub.Where(ub.Equal("id", documentID))

	sql, args = ub.Build()

	return sql, args
}

// buildDocumentDelete builds the DELETE statement for a document.
func buildDocumentDelete(table, documentID string) (sql string, args []any) {
	db := sqlbuilder.PostgreSQL.NewDeleteBuilder()
	db.DeleteFrom(table).Where(db.Equal("id", documentID))

	sql, args = db.Build()

	return sql, args
}

/*
 * Documents processing helpers/internal methods
 */

// selectNames runs a single-column SELECT and returns all values as strings.
func (dbc *Client) selectNames(ctx context.Context, sql string, args []any, operation string) ([]string, error) {
	rows, err := dbc.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	defer rows.Close()

	names := make([]string, 0)

	for rows.Next() {
		var name string

		err = rows.Scan(&name)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", operation, err)
		}

		names = append(names, name)
	}

	err = rows.Err()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}

	return names, nil
}

// documentTableName validates the index and type names and returns the
// quoted name of the table storing their documents.
func documentTableName(indexName, typeName string) (string, error) {
	err := pg.ValidateIndexName(indexName)
	if err != nil {
		return "", utils.NewIllegalQueryError("invalid index name " + strconv.Quote(indexName))
	}

	err = pg.ValidateIdentifier(typeName)
	if err != nil {
		return "", utils.NewIllegalQueryError("invalid type name " + strconv.Quote(typeName))
	}

	return sqlbuilder.PostgreSQL.Quote(indexName + "_" + typeName), nil
}

// quotedTableName validates a combined "<index>_<type>" table name and
// returns it quoted for use in a query.
func quotedTableName(tableName string) (string, error) {
	err := pg.ValidateIdentifier(tableName)
	if err != nil {
		return "", utils.NewIllegalQueryError("invalid table name " + strconv.Quote(tableName))
	}

	return sqlbuilder.PostgreSQL.Quote(tableName), nil
}

// createDataTable creates a document storage table for specified index and type.
func (dbc *Client) createDataTable(ctx context.Context, indexName, typeName string) error {
	createSequenceSQL, createTableSQL, err := buildDataTableDDL(indexName, typeName)
	if err != nil {
		return err
	}

	_, err = dbc.pool.Exec(ctx, createSequenceSQL)
	if err != nil {
		return utils.NewDBQueryError(err.Error())
	}

	_, err = dbc.pool.Exec(ctx, createTableSQL)
	if err != nil {
		return utils.NewDBQueryError(err.Error())
	}

	return nil
}

// buildDataTableDDL returns the CREATE SEQUENCE and CREATE TABLE statements
// for the document storage table of an index/type pair.
//
// DDL identifiers cannot be passed as query parameters, so this is the only
// place in the package where identifiers are formatted directly into a SQL
// string: the index name is validated with pg.ValidateIndexName and the type
// and sequence names with pg.ValidateIdentifier, then all are quoted with
// sqlbuilder.PostgreSQL.Quote first (see plan 05-pg-es-db-modernize, Task 2).
func buildDataTableDDL(indexName, typeName string) (createSequenceSQL, createTableSQL string, err error) {
	table, err := documentTableName(indexName, typeName)
	if err != nil {
		return "", "", err
	}

	sequenceName := indexName + "_" + typeName + "_id_seq"

	err = pg.ValidateIdentifier(sequenceName)
	if err != nil {
		return "", "", err
	}

	quotedSequence := sqlbuilder.PostgreSQL.Quote(sequenceName)

	createSequenceSQL = "CREATE SEQUENCE " + quotedSequence
	createTableSQL = fmt.Sprintf(
		"CREATE TABLE %s(id VARCHAR(128) PRIMARY KEY DEFAULT nextval('%s'), document JSONB NOT NULL, version integer)",
		table, quotedSequence,
	)

	return createSequenceSQL, createTableSQL, nil
}

// insertDocument inserts a new document with default ID.
func (dbc *Client) insertDocument(ctx context.Context, indexName, typeName, document string) (*ElasticSearchDocument, error) {
	table, err := documentTableName(indexName, typeName)
	if err != nil {
		return nil, err
	}

	insertSQL, insertArgs := buildDocumentInsert(table, document)

	var documentID string

	err = dbc.pool.QueryRow(ctx, insertSQL, insertArgs...).Scan(&documentID)
	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	return &ElasticSearchDocument{ID: documentID, Document: document, Version: 1}, nil
}

// insertDocumentID inserts a new document with specified ID.
func (dbc *Client) insertDocumentID(ctx context.Context, indexName, typeName, document, documentID string) (*ElasticSearchDocument, error) {
	table, err := documentTableName(indexName, typeName)
	if err != nil {
		return nil, err
	}

	insertSQL, insertArgs := buildDocumentInsertWithID(table, document, documentID)

	_, err = dbc.pool.Exec(ctx, insertSQL, insertArgs...)
	if err != nil {
		return nil, utils.NewDBQueryError(err.Error())
	}

	return &ElasticSearchDocument{ID: documentID, Document: document, Version: 1}, nil
}
