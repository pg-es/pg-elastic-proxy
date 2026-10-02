package search_test

import (
	"testing"

	"github.com/huandu/go-sqlbuilder"
	"github.com/stretchr/testify/assert"

	"github.com/pg-es/pg-es-proxy/api/search"
)

// Fixture strings shared by multiple test cases.
const (
	activeStatus   = "active"
	analyzerName   = "english"
	barValue       = "bar"
	bodyField      = "body"
	boolKey        = "bool"
	fooValue       = "foo"
	helloWorldText = "hello world"
	matchKey       = "match"
	matchPhraseKey = "match_phrase"
	mustKey        = "must"
	shouldKey      = "should"
	statusField    = "status"
	titleField     = "title"
)

// compileQuery parses rawQuery the way a caller does and compiles the result
// twice: expr is the raw WHERE expression (with "$?" successive markers, the
// format db.ProcessSearchQuery expects), and sql is that expression rendered
// with PostgreSQL "$n" placeholders. args holds the values in marker order,
// retrieved from the same Cond that built the expression.
func compileQuery(t *testing.T, rawQuery, mapping map[string]any) (expr, sql string, args []any) {
	t.Helper()

	cond := sqlbuilder.NewCond()
	expr = search.ParseSearchQuery(rawQuery, cond, mapping)
	sql, args = cond.Args.CompileWithFlavor(expr, sqlbuilder.PostgreSQL)

	return expr, sql, args
}

func TestParseSearchQueryMatch(t *testing.T) {
	t.Parallel()

	expr, sql, args := compileQuery(t,
		map[string]any{matchKey: map[string]any{titleField: helloWorldText}},
		map[string]any{},
	)

	assert.Equal(t, "((to_tsvector(document->$?) @@ to_tsquery($?)))", expr)
	assert.Equal(t, "((to_tsvector(document->$1) @@ to_tsquery($2)))", sql)
	assert.Equal(t, []any{titleField, helloWorldText}, args)
}

func TestParseSearchQueryMatchObjectForm(t *testing.T) {
	t.Parallel()

	expr, sql, args := compileQuery(t,
		map[string]any{matchKey: map[string]any{titleField: map[string]any{"query": helloWorldText}}},
		map[string]any{},
	)

	assert.Equal(t, "((to_tsvector(document->$?) @@ to_tsquery($?)))", expr)
	assert.Equal(t, "((to_tsvector(document->$1) @@ to_tsquery($2)))", sql)
	assert.Equal(t, []any{titleField, helloWorldText}, args)
}

func TestParseSearchQueryMatchWithAnalyzer(t *testing.T) {
	t.Parallel()

	mapping := map[string]any{
		"properties": map[string]any{
			titleField: map[string]any{"type": "text", "analyzer": analyzerName},
		},
	}

	expr, sql, args := compileQuery(t,
		map[string]any{matchKey: map[string]any{titleField: "hello"}},
		mapping,
	)

	assert.Equal(t, "((to_tsvector($?, document->$?) @@ to_tsquery($?, $?)))", expr)
	assert.Equal(t, "((to_tsvector($1, document->$2) @@ to_tsquery($3, $4)))", sql)
	assert.Equal(t, []any{analyzerName, titleField, analyzerName, "hello"}, args)
}

func TestParseSearchQueryMatchPhrase(t *testing.T) {
	t.Parallel()

	expr, sql, args := compileQuery(t,
		map[string]any{matchPhraseKey: map[string]any{titleField: "exact phrase"}},
		map[string]any{},
	)

	assert.Equal(t, "((to_tsvector(document->$?) @@ phraseto_tsquery($?)))", expr)
	assert.Equal(t, "((to_tsvector(document->$1) @@ phraseto_tsquery($2)))", sql)
	assert.Equal(t, []any{titleField, "exact phrase"}, args)
}

func TestParseSearchQueryMatchMultipleFields(t *testing.T) {
	t.Parallel()

	// Fields are processed in sorted order so the generated SQL is
	// deterministic regardless of Go map iteration order.
	expr, _, args := compileQuery(t,
		map[string]any{
			matchKey: map[string]any{
				titleField: fooValue,
				bodyField:  barValue,
			},
		},
		map[string]any{},
	)

	assert.Equal(t,
		"((to_tsvector(document->$?) @@ to_tsquery($?)"+
			" AND to_tsvector(document->$?) @@ to_tsquery($?)))",
		expr,
	)
	assert.Equal(t, []any{bodyField, barValue, titleField, fooValue}, args)
}

func TestParseSearchQueryBool(t *testing.T) {
	t.Parallel()

	rawQuery := map[string]any{
		boolKey: map[string]any{
			mustKey:   map[string]any{matchKey: map[string]any{titleField: fooValue}},
			"filter":  map[string]any{matchKey: map[string]any{statusField: activeStatus}},
			shouldKey: map[string]any{matchKey: map[string]any{bodyField: barValue}},
		},
	}

	expr, sql, args := compileQuery(t, rawQuery, map[string]any{})

	// The must and filter groups are ANDed together. Because the bool query
	// has a must clause, ES defaults minimum_should_match to 0, so the should
	// group does not constrain matching and is omitted from the WHERE clause.
	// Every combining level (fields, group, bool, top level) parenthesizes.
	assert.Equal(t,
		"((((to_tsvector(document->$?) @@ to_tsquery($?)))"+
			" AND ((to_tsvector(document->$?) @@ to_tsquery($?)))))",
		expr,
	)
	assert.Equal(t,
		"((((to_tsvector(document->$1) @@ to_tsquery($2)))"+
			" AND ((to_tsvector(document->$3) @@ to_tsquery($4)))))",
		sql,
	)
	assert.Equal(t, []any{titleField, fooValue, statusField, activeStatus}, args)
}

func TestParseSearchQueryBoolShouldOr(t *testing.T) {
	t.Parallel()

	rawQuery := map[string]any{
		boolKey: map[string]any{
			shouldKey: map[string]any{
				matchKey:       map[string]any{titleField: fooValue},
				matchPhraseKey: map[string]any{bodyField: "bar baz"},
			},
		},
	}

	expr, sql, args := compileQuery(t, rawQuery, map[string]any{})

	assert.Equal(t,
		"((((to_tsvector(document->$?) @@ to_tsquery($?))"+
			" OR (to_tsvector(document->$?) @@ phraseto_tsquery($?)))))",
		expr,
	)
	assert.Equal(t,
		"((((to_tsvector(document->$1) @@ to_tsquery($2))"+
			" OR (to_tsvector(document->$3) @@ phraseto_tsquery($4)))))",
		sql,
	)
	assert.Equal(t, []any{titleField, fooValue, bodyField, "bar baz"}, args)
}

func TestParseSearchQueryNestedBool(t *testing.T) {
	t.Parallel()

	rawQuery := map[string]any{
		boolKey: map[string]any{
			mustKey: map[string]any{
				boolKey: map[string]any{
					shouldKey: map[string]any{
						matchKey:       map[string]any{titleField: fooValue},
						matchPhraseKey: map[string]any{bodyField: barValue},
					},
				},
			},
		},
	}

	expr, sql, args := compileQuery(t, rawQuery, map[string]any{})

	// The inner should clauses are ORed, then each enclosing combining level
	// (inner bool, must group, outer bool, top level) adds one AND wrap.
	assert.Equal(t,
		"((((((to_tsvector(document->$?) @@ to_tsquery($?))"+
			" OR (to_tsvector(document->$?) @@ phraseto_tsquery($?)))))))",
		expr,
	)
	assert.Equal(t,
		"((((((to_tsvector(document->$1) @@ to_tsquery($2))"+
			" OR (to_tsvector(document->$3) @@ phraseto_tsquery($4)))))))",
		sql,
	)
	assert.Equal(t, []any{titleField, fooValue, bodyField, barValue}, args)
}

func TestParseSearchQueryValuesAreParameterized(t *testing.T) {
	t.Parallel()

	const maliciousValue = `'; DROP TABLE documents; --`

	// A malicious search value must never appear in the generated SQL: it is
	// bound through a placeholder and passed as a query arg.
	_, sql, args := compileQuery(t,
		map[string]any{matchKey: map[string]any{titleField: maliciousValue}},
		map[string]any{},
	)

	assert.NotContains(t, sql, maliciousValue)
	assert.NotContains(t, sql, "DROP TABLE")
	assert.Equal(t, []any{titleField, maliciousValue}, args)

	// A malicious field name is parameterized the same way.
	_, sql, args = compileQuery(t,
		map[string]any{matchKey: map[string]any{"'; DROP TABLE documents; --": "value"}},
		map[string]any{},
	)

	assert.NotContains(t, sql, "DROP TABLE")
	assert.Equal(t, []any{"'; DROP TABLE documents; --", "value"}, args)
}

// TestParseSearchQueryProcessSearchQueryIntegration verifies that the
// expression and args produced by ParseSearchQuery flow through the exact
// wrapping db.ProcessSearchQuery performs in its buildSearchCount and
// buildSearchSelect helpers (sqlbuilder.Build(whereExpr, args...) embedded in
// a PostgreSQL SelectBuilder via sb.Var).
func TestParseSearchQueryProcessSearchQueryIntegration(t *testing.T) {
	t.Parallel()

	const table = `"twitter_tweet"`

	cond := sqlbuilder.NewCond()
	whereExpr := search.ParseSearchQuery(
		map[string]any{matchKey: map[string]any{titleField: helloWorldText}},
		cond,
		map[string]any{},
	)
	_, args := cond.Args.Compile(whereExpr)

	assert.Equal(t, "((to_tsvector(document->$?) @@ to_tsquery($?)))", whereExpr)
	assert.Equal(t, []any{titleField, helloWorldText}, args)

	tests := []struct {
		name    string
		columns []string
		wantSQL string
	}{
		{
			name:    "count query",
			columns: []string{"COUNT(*)"},
			wantSQL: `SELECT COUNT(*) FROM "twitter_tweet" WHERE ((to_tsvector(document->$1) @@ to_tsquery($2)))`,
		},
		{
			name:    "select query",
			columns: []string{"id", "document", "version"},
			wantSQL: `SELECT id, document, version FROM "twitter_tweet" WHERE ((to_tsvector(document->$1) @@ to_tsquery($2)))`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			sb := sqlbuilder.PostgreSQL.NewSelectBuilder()
			sb.Select(tt.columns...).From(table)
			sb.Where(sb.Var(sqlbuilder.Build(whereExpr, args...)))

			sql, sqlArgs := sb.Build()

			assert.Equal(t, tt.wantSQL, sql)
			assert.Equal(t, []any{titleField, helloWorldText}, sqlArgs)
		})
	}
}

func TestParseSearchQueryEmptyClauses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		rawQuery map[string]any
	}{
		{name: "empty query", rawQuery: map[string]any{}},
		{name: "match_all", rawQuery: map[string]any{"match_all": map[string]any{}}},
		{name: "unsupported key", rawQuery: map[string]any{"term": map[string]any{statusField: activeStatus}}},
		{name: "non-object match", rawQuery: map[string]any{matchKey: "not a map"}},
		{name: "non-object bool", rawQuery: map[string]any{boolKey: "not a map"}},
		{
			name: "bool with empty groups",
			rawQuery: map[string]any{
				boolKey: map[string]any{
					mustKey:   map[string]any{},
					shouldKey: map[string]any{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			expr, sql, args := compileQuery(t, tt.rawQuery, map[string]any{})

			// An empty expression means no WHERE constraint, which selects
			// all documents — the same behavior the previous ORM had.
			assert.Empty(t, expr)
			assert.Empty(t, sql)
			assert.Empty(t, args)
		})
	}
}
