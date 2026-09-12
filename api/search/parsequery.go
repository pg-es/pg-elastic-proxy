// Package search translates Elasticsearch query DSL objects into PostgreSQL
// WHERE expressions built with go-sqlbuilder.
package search

import (
	"regexp"
	"slices"

	"github.com/huandu/go-sqlbuilder"

	"github.com/pg-es/pg-es-proxy/utils"
)

// condMarkerRe matches the "$<n>" placeholder tokens produced by
// sqlbuilder.Cond.Var (sqlbuilder.Args.Add). Those tokens are the only "$"
// characters that can appear in an expression built by this package: the SQL
// keywords concatenated into clauses (to_tsvector, document->, the tsquery
// function names) contain no "$", and field names, analyzers, and search
// values are injected exclusively through Cond.Var, which returns a
// placeholder token and registers the value as a query arg.
var condMarkerRe = regexp.MustCompile(`\$\d+`)

// ParseSearchQuery parses an Elasticsearch query DSL object into a WHERE
// expression for db.ProcessSearchQuery.
//
// cond must be a fresh Cond from sqlbuilder.NewCond with no values
// registered yet. Every field name, analyzer, and search value is registered
// on cond via Cond.Var; the returned expression embeds only placeholder
// tokens, rewritten to "$?" successive markers that reference cond's values
// in order. Retrieve them with:
//
//	_, args := cond.Args.Compile(whereExpr)
//
// and pass both to db.ProcessSearchQuery(ctx, tableName, whereExpr, args).
// An empty query yields an empty expression, which selects all documents.
func ParseSearchQuery(rawQuery map[string]any, cond *sqlbuilder.Cond, mapping map[string]any) string {
	return toSuccessiveExpr(cond.And(parseQueryClauses(rawQuery, cond, mapping)...))
}

// toSuccessiveExpr rewrites the "$<n>" placeholder tokens embedded in expr
// into "$?" successive markers, the format expected by
// db.ProcessSearchQuery's whereExpr parameter. Cond.Var tokens cannot be
// used there directly: a Cond from sqlbuilder.NewCond numbers its markers
// from 256, and sqlbuilder.Build would resolve "$256" as a positional
// reference to the 257th argument. Each token is embedded exactly once, in
// the order its value was registered on the Cond, so rewriting the tokens in
// place yields "$?" markers that reference the Cond's args in order.
func toSuccessiveExpr(expr string) string {
	return condMarkerRe.ReplaceAllLiteralString(expr, "$?")
}

// parseQueryClauses builds the WHERE clause for each supported query type in
// rawQuery. Query types are visited in a fixed key order so the generated
// SQL is deterministic; unsupported keys and non-object values are ignored,
// matching the previous ORM implementation.
func parseQueryClauses(rawQuery map[string]any, cond *sqlbuilder.Cond, mapping map[string]any) []string {
	clauses := make([]string, 0, len(rawQuery))

	for _, key := range []string{"match_all", "match", "match_phrase", "bool"} {
		val, ok := rawQuery[key]
		if !ok {
			continue
		}

		queryMap, ok := val.(map[string]any)
		if !ok {
			continue
		}

		switch key {
		case "match_all":
			// match_all imposes no constraint: the empty clause is dropped
			// by Cond.And, leaving no WHERE restriction.
		case "match":
			clauses = append(clauses, parseMatchQuery(queryMap, cond, mapping))
		case "match_phrase":
			clauses = append(clauses, parseMatchPhraseQuery(queryMap, cond, mapping))
		case "bool":
			clauses = append(clauses, parseBoolQuery(queryMap, cond, mapping))
		}
	}

	return clauses
}

// buildTSQueryClause builds the tsvector comparison clause for one field.
//
// Structure (keywords, function names) is safe to build with string
// concatenation. Only cond.Var(x) may inject a value — it returns a
// placeholder token and registers x as a query arg.
// NEVER pass a raw value directly into the concatenated string.
func buildTSQueryClause(
	cond *sqlbuilder.Cond,
	fieldName, value, tsFunc string,
	fieldMapping *utils.FieldMapping,
	hasMapping bool,
) string {
	if hasMapping && fieldMapping.Analyzer != "" {
		// The analyzer is registered once per occurrence — a "$?" successive
		// marker cannot reference the same arg twice.
		return "to_tsvector(" + cond.Var(fieldMapping.Analyzer) + ", document->" + cond.Var(fieldName) +
			") @@ " + tsFunc + "(" + cond.Var(fieldMapping.Analyzer) + ", " + cond.Var(value) + ")"
	}

	return "to_tsvector(document->" + cond.Var(fieldName) + ") @@ " + tsFunc + "(" + cond.Var(value) + ")"
}

// extractQueryString returns the "query" string from the object form of a
// match field value (e.g. {"query": "search terms"}), or an empty string.
func extractQueryString(val map[string]any) string {
	queryString, _ := val["query"].(string)

	return queryString
}

// parseMatchGeneric builds the tsvector match clause for every field in
// rawQuery (the value of a match or match_phrase query), combined with AND.
// Fields are visited in sorted order so the generated SQL is deterministic.
// tsFunc is the tsquery constructor: to_tsquery for match, phraseto_tsquery
// for match_phrase.
func parseMatchGeneric(rawQuery map[string]any, cond *sqlbuilder.Cond, mapping map[string]any, tsFunc string) string {
	fields := make([]string, 0, len(rawQuery))
	for fieldName := range rawQuery {
		fields = append(fields, fieldName)
	}

	slices.Sort(fields)

	clauses := make([]string, 0, len(fields))
	for _, fieldName := range fields {
		var value string

		switch fieldVal := rawQuery[fieldName].(type) {
		case string:
			value = fieldVal
		case map[string]any:
			value = extractQueryString(fieldVal)
		default:
			continue
		}

		fieldMapping, ok := utils.GetFieldMapping(mapping, fieldName)
		clauses = append(clauses, buildTSQueryClause(cond, fieldName, value, tsFunc, fieldMapping, ok))
	}

	return cond.And(clauses...)
}

// parseMatchQuery builds the clause for a match query: a to_tsquery
// full-text lookup per field, combined with AND.
func parseMatchQuery(rawQuery map[string]any, cond *sqlbuilder.Cond, mapping map[string]any) string {
	return parseMatchGeneric(rawQuery, cond, mapping, "to_tsquery")
}

// parseMatchPhraseQuery builds the clause for a match_phrase query: a
// phraseto_tsquery phrase lookup per field, combined with AND.
func parseMatchPhraseQuery(rawQuery map[string]any, cond *sqlbuilder.Cond, mapping map[string]any) string {
	return parseMatchGeneric(rawQuery, cond, mapping, "phraseto_tsquery")
}

// parseBoolQuery builds the clause for a bool query. The must, filter, and
// must_not sub-queries are combined with AND. Per the ES bool-query default,
// minimum_should_match is 1 when there are should clauses and no must/filter
// clauses, and 0 otherwise; so should clauses are ORed among themselves and
// combined with AND only when there is no must or filter clause. Keys are
// visited in a fixed order so the generated SQL is deterministic.
func parseBoolQuery(rawQuery map[string]any, cond *sqlbuilder.Cond, mapping map[string]any) string {
	clauses := make([]string, 0, len(rawQuery))
	hasMandatory := false

	for _, key := range []string{"must", "filter", "should", "must_not"} {
		val, ok := rawQuery[key]
		if !ok {
			continue
		}

		queryMap, ok := val.(map[string]any)
		if !ok {
			continue
		}

		switch key {
		case "must", "filter":
			hasMandatory = true

			clauses = append(clauses, cond.And(parseQueryClauses(queryMap, cond, mapping)...))
		case "should":
			if !hasMandatory {
				clauses = append(clauses, cond.Or(parseQueryClauses(queryMap, cond, mapping)...))
			}
		case "must_not":
			// TODO: must_not query negation.
			clauses = append(clauses, cond.And(parseQueryClauses(queryMap, cond, mapping)...))
		}
	}

	return cond.And(clauses...)
}
