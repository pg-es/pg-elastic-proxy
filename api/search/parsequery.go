package search

import (
	"fmt"

	"github.com/pg-es/pg-es-proxy/db"
	"github.com/pg-es/pg-es-proxy/utils"
)

// ParseSearchQuery parses a query and converts it into db.Query.
func ParseSearchQuery(rawQuery map[string]any, query *db.Query, mapping map[string]any) {
	for key, val := range rawQuery {
		queryMap, ok := val.(map[string]any)
		if !ok {
			continue
		}

		switch key {
		case "match_all":
			parseMatchAllQuery(query, mapping)
		case "match":
			parseMatchQuery(queryMap, query, mapping)
		case "match_phrase":
			parseMatchPhraseQuery(queryMap, query, mapping)
		case "bool":
			parseBoolQuery(queryMap, query, mapping)
		}
	}
}

func parseMatchAllQuery(_ *db.Query, _ map[string]any) {
}

func buildTSQueryClause(
	fieldName, value, tsFunc string, fieldMapping *utils.FieldMapping, hasMapping bool,
) string {
	if hasMapping && fieldMapping.Analyzer != "" {
		return fmt.Sprintf(
			"to_tsvector('%s', document->'%s') @@ %s('%s', '%s')",
			fieldMapping.Analyzer, fieldName, tsFunc, fieldMapping.Analyzer, value,
		)
	}

	return fmt.Sprintf("to_tsvector(document->'%s') @@ %s('%s')", fieldName, tsFunc, value)
}

func extractQueryString(val map[string]any) string {
	for key, vv := range val {
		if key == "query" {
			if qs, ok := vv.(string); ok {
				return qs
			}
		}
	}

	return ""
}

func parseMatchGeneric(rawQuery map[string]any, query *db.Query, mapping map[string]any, tsFunc string) {
	for fieldName, fieldVal := range rawQuery {
		switch val := fieldVal.(type) {
		case string:
			fieldMapping, ok := utils.GetFieldMapping(mapping, fieldName)
			query.Where(buildTSQueryClause(fieldName, val, tsFunc, fieldMapping, ok))
		case map[string]any:
			queryString := extractQueryString(val)
			fieldMapping, ok := utils.GetFieldMapping(mapping, fieldName)
			query.Where(buildTSQueryClause(fieldName, queryString, tsFunc, fieldMapping, ok))
		}
	}
}

func parseMatchPhraseQuery(rawQuery map[string]any, query *db.Query, mapping map[string]any) {
	parseMatchGeneric(rawQuery, query, mapping, "phraseto_tsquery")
}

func parseMatchQuery(rawQuery map[string]any, query *db.Query, mapping map[string]any) {
	parseMatchGeneric(rawQuery, query, mapping, "to_tsquery")
}

func parseBoolQuery(rawQuery map[string]any, query *db.Query, mapping map[string]any) {
	for key, val := range rawQuery {
		queryMap, ok := val.(map[string]any)
		if !ok {
			continue
		}

		switch key {
		case "must":
			query.WhereGroup(func(q *db.Query) (*db.Query, error) {
				ParseSearchQuery(queryMap, q, mapping)
				return q, nil
			})
		case "filter":
			query.WhereGroup(func(q *db.Query) (*db.Query, error) {
				ParseSearchQuery(queryMap, q, mapping)
				return q, nil
			})
		case "must_not":
			// TODO: must_not query negation.
			query.WhereGroup(func(q *db.Query) (*db.Query, error) {
				ParseSearchQuery(queryMap, q, mapping)
				return q, nil
			})
		case "should":
			query.WhereOrGroup(func(q *db.Query) (*db.Query, error) {
				ParseSearchQuery(queryMap, q, mapping)
				return q, nil
			})
		}
	}
}
