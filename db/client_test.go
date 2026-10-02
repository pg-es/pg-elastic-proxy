//nolint:testpackage // white-box tests verify the unexported SQL builders used by Client methods.
package db

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pg-es/pg-es-proxy/internal/pg"
	"github.com/pg-es/pg-es-proxy/utils"
)

// buildQueryCase describes one SQL-generation test case for a query builder.
type buildQueryCase struct {
	name      string
	build     func() (string, []any)
	wantSQL   string
	wantArgs  []any
	forbidden []string
}

// runBuildQueryTests runs the SQL-generation cases: the built statement must
// match exactly, use placeholders for every forbidden literal, and pass the
// values as query arguments.
func runBuildQueryTests(t *testing.T, tests []buildQueryCase) {
	t.Helper()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sql, args := tt.build()

			assert.Equal(t, tt.wantSQL, sql)
			assert.Equal(t, tt.wantArgs, args)

			for _, literal := range tt.forbidden {
				assert.NotContains(t, sql, literal)
			}
		})
	}
}

func TestNewClient(t *testing.T) {
	t.Parallel()

	client := NewClient(nil)

	assert.NotNil(t, client)
}

func TestBuildSystemTableDDL(t *testing.T) {
	t.Parallel()

	statements := buildSystemTableDDL()

	require.Len(t, statements, 2)
	assert.Equal(t,
		`CREATE TABLE IF NOT EXISTS "index_records" ("name" text, "options" text, PRIMARY KEY ("name"))`,
		statements[0],
	)
	assert.Equal(t,
		`CREATE TABLE IF NOT EXISTS "type_records" ("name" text, "index_name" text, "options" text, PRIMARY KEY ("name", "index_name"))`,
		statements[1],
	)
}

func TestBuildSearchQueries(t *testing.T) {
	t.Parallel()

	const (
		table     = `"task2_docs"`
		whereExpr = "version > $? AND document @> $?"
	)

	searchArgs := []any{1, `{"title": "hello"}`}

	countSQL, countArgs := buildSearchCount(table, whereExpr, searchArgs)
	assert.Equal(t, `SELECT COUNT(*) FROM "task2_docs" WHERE version > $1 AND document @> $2`, countSQL)
	assert.Equal(t, searchArgs, countArgs)

	selectSQL, selectArgs := buildSearchSelect(table, whereExpr, searchArgs)
	assert.Equal(t, `SELECT id, document, version FROM "task2_docs" WHERE version > $1 AND document @> $2`, selectSQL)
	assert.Equal(t, searchArgs, selectArgs)
}

func TestBuildSearchQueriesEmptyWhere(t *testing.T) {
	t.Parallel()

	const table = `"task2_docs"`

	countSQL, countArgs := buildSearchCount(table, "", nil)
	assert.Equal(t, `SELECT COUNT(*) FROM "task2_docs"`, countSQL)
	assert.Empty(t, countArgs)

	selectSQL, selectArgs := buildSearchSelect(table, "", nil)
	assert.Equal(t, `SELECT id, document, version FROM "task2_docs"`, selectSQL)
	assert.Empty(t, selectArgs)
}

func TestBuildIndexQueries(t *testing.T) {
	t.Parallel()

	const (
		indexName = "task2"
		options   = `{"settings": {}}`
		pattern   = "task%"
	)

	tests := []buildQueryCase{
		{
			name: "index count",
			build: func() (string, []any) {
				return buildIndexCount(indexName)
			},
			wantSQL:   `SELECT COUNT(*) FROM "index_records" WHERE name = $1`,
			wantArgs:  []any{indexName},
			forbidden: []string{indexName},
		},
		{
			name: "index select",
			build: func() (string, []any) {
				return buildIndexSelect(indexName)
			},
			wantSQL:   `SELECT name, options FROM "index_records" WHERE name = $1`,
			wantArgs:  []any{indexName},
			forbidden: []string{indexName},
		},
		{
			name: "index insert",
			build: func() (string, []any) {
				return buildIndexInsert(indexName, options)
			},
			wantSQL:   `INSERT INTO "index_records" (name, options) VALUES ($1, $2)`,
			wantArgs:  []any{indexName, options},
			forbidden: []string{indexName, options},
		},
		{
			name: "index names like",
			build: func() (string, []any) {
				return buildIndexNamesLike(pattern)
			},
			wantSQL:   `SELECT name FROM "index_records" WHERE name LIKE $1`,
			wantArgs:  []any{pattern},
			forbidden: []string{pattern},
		},
	}

	runBuildQueryTests(t, tests)
}

func TestBuildTypeQueries(t *testing.T) {
	t.Parallel()

	const (
		indexName = "task2"
		typeName  = "docs"
		options   = `{"mappings": {}}`
		pattern   = "do%"
	)

	tests := []buildQueryCase{
		{
			name: "type count",
			build: func() (string, []any) {
				return buildTypeCount(indexName, typeName)
			},
			wantSQL:   `SELECT COUNT(*) FROM "type_records" WHERE name = $1 AND index_name = $2`,
			wantArgs:  []any{typeName, indexName},
			forbidden: []string{typeName, indexName},
		},
		{
			name: "type select",
			build: func() (string, []any) {
				return buildTypeSelect(indexName, typeName)
			},
			wantSQL:   `SELECT name, index_name, options FROM "type_records" WHERE name = $1 AND index_name = $2`,
			wantArgs:  []any{typeName, indexName},
			forbidden: []string{typeName, indexName},
		},
		{
			name: "type insert",
			build: func() (string, []any) {
				return buildTypeInsert(indexName, typeName, options)
			},
			wantSQL:   `INSERT INTO "type_records" (name, index_name, options) VALUES ($1, $2, $3)`,
			wantArgs:  []any{typeName, indexName, options},
			forbidden: []string{typeName, indexName, options},
		},
		{
			name: "type options update",
			build: func() (string, []any) {
				return buildTypeOptionsUpdate(indexName, typeName, options)
			},
			wantSQL:   `UPDATE "type_records" SET options = $1 WHERE name = $2 AND index_name = $3`,
			wantArgs:  []any{options, typeName, indexName},
			forbidden: []string{options, typeName, indexName},
		},
		{
			name: "type names like",
			build: func() (string, []any) {
				return buildTypeNamesLike(indexName, pattern)
			},
			wantSQL:   `SELECT name FROM "type_records" WHERE name LIKE $1 AND index_name = $2`,
			wantArgs:  []any{pattern, indexName},
			forbidden: []string{pattern, indexName},
		},
	}

	runBuildQueryTests(t, tests)
}

func TestBuildDocumentQueries(t *testing.T) {
	t.Parallel()

	const (
		table        = `"task2_docs"`
		document     = `{"title": "hello world"}`
		documentID   = "doc-1"
		firstVersion = 1
	)

	tests := []buildQueryCase{
		{
			name: "document count",
			build: func() (string, []any) {
				return buildDocumentCount(table, documentID)
			},
			wantSQL:   `SELECT COUNT(*) FROM "task2_docs" WHERE id = $1`,
			wantArgs:  []any{documentID},
			forbidden: []string{documentID},
		},
		{
			name: "document select",
			build: func() (string, []any) {
				return buildDocumentSelect(table, documentID)
			},
			wantSQL:   `SELECT id, document, version FROM "task2_docs" WHERE id = $1`,
			wantArgs:  []any{documentID},
			forbidden: []string{documentID},
		},
		{
			name: "document insert with generated id",
			build: func() (string, []any) {
				return buildDocumentInsert(table, document)
			},
			wantSQL:   `INSERT INTO "task2_docs" (id, document, version) VALUES (DEFAULT, $1, $2) RETURNING id`,
			wantArgs:  []any{document, firstVersion},
			forbidden: []string{document},
		},
		{
			name: "document insert with explicit id",
			build: func() (string, []any) {
				return buildDocumentInsertWithID(table, document, documentID)
			},
			wantSQL:   `INSERT INTO "task2_docs" (id, document, version) VALUES ($1, $2, $3)`,
			wantArgs:  []any{documentID, document, firstVersion},
			forbidden: []string{documentID, document},
		},
		{
			name: "document update",
			build: func() (string, []any) {
				return buildDocumentUpdate(table, document, documentID)
			},
			wantSQL:   `UPDATE "task2_docs" SET document = $1, version = version + 1 WHERE id = $2`,
			wantArgs:  []any{document, documentID},
			forbidden: []string{document, documentID},
		},
		{
			name: "document delete",
			build: func() (string, []any) {
				return buildDocumentDelete(table, documentID)
			},
			wantSQL:   `DELETE FROM "task2_docs" WHERE id = $1`,
			wantArgs:  []any{documentID},
			forbidden: []string{documentID},
		},
	}

	runBuildQueryTests(t, tests)
}

func TestDocumentTableName(t *testing.T) {
	t.Parallel()

	const (
		validIndexName = "task2"
		validTypeName  = "docs"
	)

	tests := []struct {
		name      string
		indexName string
		typeName  string
		want      string
		wantErr   bool
	}{
		{name: "valid names", indexName: validIndexName, typeName: validTypeName, want: `"task2_docs"`, wantErr: false},
		{name: "dash in index", indexName: "task-2", typeName: validTypeName, want: `"task-2_docs"`, wantErr: false},
		{name: "space in type", indexName: validIndexName, typeName: "my docs", wantErr: true},
		{name: "leading digit in index", indexName: "1task", typeName: validTypeName, want: `"1task_docs"`, wantErr: false},
		{name: "empty index", indexName: "", typeName: validTypeName, wantErr: true},
		{name: "empty type", indexName: validIndexName, typeName: "", wantErr: true},
		{name: "quote attempt", indexName: `task"; DROP TABLE x`, typeName: validTypeName, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			table, err := documentTableName(tt.indexName, tt.typeName)

			if tt.wantErr {
				require.Error(t, err)

				var illegal *utils.IllegalQueryError
				require.ErrorAs(t, err, &illegal)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, table)
		})
	}
}

func TestQuotedTableName(t *testing.T) {
	t.Parallel()

	const validTable = "task2_docs"

	tests := []struct {
		name    string
		table   string
		want    string
		wantErr bool
	}{
		{name: "valid table", table: validTable, want: `"task2_docs"`, wantErr: false},
		{name: "dash", table: "task2-docs", want: `"task2-docs"`, wantErr: false},
		{name: "leading digit", table: "1task_docs", want: `"1task_docs"`, wantErr: false},
		{name: "empty", table: "", wantErr: true},
		{name: "semicolon injection attempt", table: "task2; DROP TABLE x", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			table, err := quotedTableName(tt.table)

			if tt.wantErr {
				require.Error(t, err)

				var illegal *utils.IllegalQueryError
				require.ErrorAs(t, err, &illegal)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.want, table)
		})
	}
}

func TestBuildDataTableDDL(t *testing.T) {
	t.Parallel()

	const (
		validIndexName = "task2"
		validTypeName  = "docs"
	)

	tests := []struct {
		name      string
		indexName string
		typeName  string
		wantSeq   string
		wantTable string
		wantErr   bool
	}{
		{
			name:      "valid ddl",
			indexName: validIndexName,
			typeName:  validTypeName,
			wantSeq:   `CREATE SEQUENCE "task2_docs_id_seq"`,
			wantTable: `CREATE TABLE "task2_docs"(id VARCHAR(128) PRIMARY KEY DEFAULT nextval('"task2_docs_id_seq"'), document JSONB NOT NULL, version integer)`,
			wantErr:   false,
		},
		{
			name: "dash in index", indexName: "task-2", typeName: validTypeName, wantErr: false,
			wantSeq:   `CREATE SEQUENCE "task-2_docs_id_seq"`,
			wantTable: `CREATE TABLE "task-2_docs"(id VARCHAR(128) PRIMARY KEY DEFAULT nextval('"task-2_docs_id_seq"'), document JSONB NOT NULL, version integer)`,
		},
		{name: "space in type", indexName: validIndexName, typeName: "my docs", wantErr: true},
		{name: "empty index", indexName: "", typeName: validTypeName, wantErr: true},
		{name: "empty type", indexName: validIndexName, typeName: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			createSequenceSQL, createTableSQL, err := buildDataTableDDL(tt.indexName, tt.typeName)

			if tt.wantErr {
				require.Error(t, err)

				var illegal *utils.IllegalQueryError
				require.ErrorAs(t, err, &illegal)

				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantSeq, createSequenceSQL)
			assert.Equal(t, tt.wantTable, createTableSQL)
		})
	}
}

// TestBuildDataTableDDLRejectsMixedCase guards the ES lowercase-only rule:
// mixed-case index names are invalid and must be rejected at validation, so
// they never reach the DDL quoting logic.
func TestBuildDataTableDDLRejectsMixedCase(t *testing.T) {
	t.Parallel()

	_, _, err := buildDataTableDDL("Foo", "Bar")
	require.Error(t, err)

	var illegal *utils.IllegalQueryError
	require.ErrorAs(t, err, &illegal)
}

/*
 * Integration tests. They run only when PG_ES_TEST_DATABASE_URL points to a
 * scratch PostgreSQL instance and are skipped in short mode.
 */

// newIntegrationClient connects to the test database, initializes the
// schema, and returns a Client. The test is skipped in short mode or when
// PG_ES_TEST_DATABASE_URL is not set.
func newIntegrationClient(t *testing.T) *Client {
	t.Helper()

	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	connString := os.Getenv("PG_ES_TEST_DATABASE_URL")
	if connString == "" {
		t.Skip("PG_ES_TEST_DATABASE_URL is not set")
	}

	const (
		maxConns = 4
		minConns = 1
	)

	pool, err := pg.NewPool(t.Context(), connString, maxConns, minConns)
	require.NoError(t, err)
	t.Cleanup(pool.Close)

	client := NewClient(pool)
	require.NoError(t, client.InitializeSchema(t.Context()))

	return client
}

// cleanupIntegrationTables drops the tables and sequences created by the
// integration tests so that repeated runs start from a clean state.
func cleanupIntegrationTables(t *testing.T, client *Client) {
	t.Helper()

	t.Cleanup(func() {
		ctx := context.Background()

		_, _ = client.pool.Exec(ctx, `DROP TABLE IF EXISTS "task2_docs"`)
		_, _ = client.pool.Exec(ctx, `DROP SEQUENCE IF EXISTS "task2_docs_id_seq"`)
		_, _ = client.pool.Exec(ctx, `DROP TABLE IF EXISTS "task2_types"`)
		_, _ = client.pool.Exec(ctx, `DROP TABLE IF EXISTS "task2_indices"`)
		_, _ = client.pool.Exec(ctx, `DROP TABLE IF EXISTS "type_records"`)
		_, _ = client.pool.Exec(ctx, `DROP TABLE IF EXISTS "index_records"`)
	})
}

//nolint:paralleltest // integration tests share the system tables and must run sequentially.
func TestIntegrationIndexAndTypeLifecycle(t *testing.T) {
	client := newIntegrationClient(t)
	cleanupIntegrationTables(t, client)

	created, err := client.CreateIndex(t.Context(), "task2", `{"settings": {}}`)
	require.NoError(t, err)
	assert.Equal(t, "task2", created.Name)

	_, err = client.CreateIndex(t.Context(), "task2", "")
	require.Error(t, err)

	var alreadyExists *utils.ResourceAlreadyExistsError
	require.ErrorAs(t, err, &alreadyExists)

	fetched, err := client.GetIndex(t.Context(), "task2")
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.JSONEq(t, `{"settings": {}}`, fetched.Options)

	missing, err := client.GetIndex(t.Context(), "task2_missing")
	require.NoError(t, err)
	assert.Nil(t, missing)

	found, err := client.FindIndices(t.Context(), "task*")
	require.NoError(t, err)
	assert.Contains(t, found, "task2")

	typeRecord, err := client.CreateType(t.Context(), "task2", "docs", "")
	require.NoError(t, err)
	assert.Equal(t, "docs", typeRecord.Name)

	fetchedType, err := client.GetType(t.Context(), "task2", "docs")
	require.NoError(t, err)
	require.NotNil(t, fetchedType)

	missingType, err := client.GetType(t.Context(), "task2", "missing")
	require.NoError(t, err)
	assert.Nil(t, missingType)

	updated, err := client.UpdateTypeOptions(t.Context(), "task2", "docs", `{"mappings": {}}`)
	require.NoError(t, err)
	assert.JSONEq(t, `{"mappings": {}}`, updated.Options)

	foundTypes, err := client.FindTypes(t.Context(), "task2", "do*")
	require.NoError(t, err)
	assert.Contains(t, foundTypes, "docs")
}

//nolint:paralleltest // integration tests share the system tables and must run sequentially.
func TestIntegrationDocumentLifecycle(t *testing.T) {
	client := newIntegrationClient(t)
	cleanupIntegrationTables(t, client)

	// CreateDocument auto-creates the index and the type.
	doc, err := client.CreateDocument(t.Context(), "task2", "docs", `{"title": "hello"}`, "doc-1")
	require.NoError(t, err)
	require.NotNil(t, doc)
	assert.Equal(t, "doc-1", doc.ID)
	assert.Equal(t, 1, doc.Version)

	fetched, err := client.GetDocument(t.Context(), "task2", "docs", "doc-1")
	require.NoError(t, err)
	require.NotNil(t, fetched)
	assert.Equal(t, "doc-1", fetched.ID)

	exists, err := client.IsDocumentExists(t.Context(), "task2", "docs", "doc-1")
	require.NoError(t, err)
	assert.True(t, exists)

	autoDoc, err := client.CreateDocument(t.Context(), "task2", "docs", `{"title": "auto"}`, "")
	require.NoError(t, err)
	assert.NotEmpty(t, autoDoc.ID)

	_, err = client.CreateDocument(t.Context(), "task2", "docs", `{"title": "dup"}`, "doc-1")
	require.Error(t, err)

	var conflict *utils.VersionConflictError
	require.ErrorAs(t, err, &conflict)

	updated, err := client.UpdateDocument(t.Context(), "task2", "docs", `{"title": "updated"}`, "doc-1")
	require.NoError(t, err)
	assert.Equal(t, 2, updated.Version)

	deleted, err := client.DeleteDocument(t.Context(), "task2", "docs", "doc-1")
	require.NoError(t, err)
	assert.Equal(t, "doc-1", deleted.ID)

	gone, err := client.GetDocument(t.Context(), "task2", "docs", "doc-1")
	require.NoError(t, err)
	assert.Nil(t, gone)
}

//nolint:paralleltest // integration tests share the system tables and must run sequentially.
func TestIntegrationProcessSearchQuery(t *testing.T) {
	client := newIntegrationClient(t)
	cleanupIntegrationTables(t, client)

	for id, title := range map[string]string{
		"doc-1": "hello world",
		"doc-2": "goodbye world",
	} {
		_, err := client.CreateDocument(t.Context(), "task2", "docs", fmt.Sprintf(`{"title": %q}`, title), id)
		require.NoError(t, err)
	}

	docs, err := client.ProcessSearchQuery(t.Context(), "task2_docs", "version > $?", []any{0})
	require.NoError(t, err)
	require.Len(t, docs, 2)

	titles := make([]string, 0, len(docs))
	for _, doc := range docs {
		document, ok := doc.Document.(map[string]any)
		require.True(t, ok)

		title, ok := document["title"].(string)
		require.True(t, ok)

		titles = append(titles, title)
	}

	assert.ElementsMatch(t, []string{"hello world", "goodbye world"}, titles)

	none, err := client.ProcessSearchQuery(t.Context(), "task2_docs", "version > $?", []any{5})
	require.NoError(t, err)
	assert.Nil(t, none)

	_, err = client.ProcessSearchQuery(t.Context(), "task2-docs", "version > $?", []any{0})
	require.Error(t, err)

	var illegal *utils.IllegalQueryError
	require.ErrorAs(t, err, &illegal)
}
