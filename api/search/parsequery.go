package search

import (
	"fmt"

	"github.com/pg-es/pg-es-proxy/db"
	"github.com/pg-es/pg-es-proxy/utils"
)

// ParseSearchQuery parses a query and convert it into db.Query
func ParseSearchQuery(rawQuery map[string]any, query *db.Query, mapping map[string]any) {
	for k, v := range rawQuery {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		switch k {
		case "match_all":
			parseMatchAllQuery(query, mapping)
		case "match":
			parseMatchQuery(m, query, mapping)
		case "match_phrase":
			parseMatchPhraseQuery(m, query, mapping)
		case "bool":
			parseBoolQuery(m, query, mapping)
		}
	}
}

func parseMatchAllQuery(_ *db.Query, _ map[string]any) {
}

func parseMatchPhraseQuery(rawQuery map[string]any, query *db.Query, mapping map[string]any) {
	var whereClause string
	for k, v := range rawQuery {
		fieldName := k
		switch val := v.(type) {
		case string:
			fieldMapping, ok := utils.GetFieldMapping(mapping, fieldName)
			if ok && fieldMapping.Analyzer != "" {
				whereClause = fmt.Sprintf("to_tsvector('%s', document->'%s') @@ phraseto_tsquery('%s', '%s')", fieldMapping.Analyzer, fieldName, fieldMapping.Analyzer, val)
			} else {
				whereClause = fmt.Sprintf("to_tsvector(document->'%s') @@ phraseto_tsquery('%s')", fieldName, val)
			}
			query.Where(whereClause)
		case map[string]any:
			var queryString, operator string
			for kk, vv := range val {
				switch kk {
				case "query":
					queryString, _ = vv.(string)
				case "operator":
					operator, _ = vv.(string)
				}
			}
			_ = operator
			fieldMapping, ok := utils.GetFieldMapping(mapping, fieldName)
			if ok && fieldMapping.Analyzer != "" {
				whereClause = fmt.Sprintf("to_tsvector('%s', document->'%s') @@ phraseto_tsquery('%s', '%s')", fieldMapping.Analyzer, fieldName, fieldMapping.Analyzer, queryString)
			} else {
				whereClause = fmt.Sprintf("to_tsvector(document->'%s') @@ phraseto_tsquery('%s')", fieldName, queryString)
			}
			query.Where(whereClause)
		}
	}
}

func parseMatchQuery(rawQuery map[string]any, query *db.Query, mapping map[string]any) {
	var whereClause string
	for k, v := range rawQuery {
		fieldName := k
		switch val := v.(type) {
		case string:
			fieldMapping, ok := utils.GetFieldMapping(mapping, fieldName)
			if ok && fieldMapping.Analyzer != "" {
				whereClause = fmt.Sprintf("to_tsvector('%s', document->'%s') @@ to_tsquery('%s', '%s')", fieldMapping.Analyzer, fieldName, fieldMapping.Analyzer, val)
			} else {
				whereClause = fmt.Sprintf("to_tsvector(document->'%s') @@ to_tsquery('%s')", fieldName, val)
			}
			query.Where(whereClause)
		case map[string]any:
			var queryString, operator string
			for kk, vv := range val {
				switch kk {
				case "query":
					queryString, _ = vv.(string)
				case "operator":
					operator, _ = vv.(string)
				}
			}
			_ = operator
			fieldMapping, ok := utils.GetFieldMapping(mapping, fieldName)
			if ok && fieldMapping.Analyzer != "" {
				whereClause = fmt.Sprintf("to_tsvector('%s', document->'%s') @@ to_tsquery('%s', '%s')", fieldMapping.Analyzer, fieldName, fieldMapping.Analyzer, queryString)
			} else {
				whereClause = fmt.Sprintf("to_tsvector(document->'%s') @@ to_tsquery('%s')", fieldName, queryString)
			}
			query.Where(whereClause)
		}
	}
}

func parseBoolQuery(rawQuery map[string]any, query *db.Query, mapping map[string]any) {
	for k, v := range rawQuery {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		switch k {
		case "must":
			query.WhereGroup(func(q *db.Query) (*db.Query, error) {
				ParseSearchQuery(m, q, mapping)
				return q, nil
			})
		case "filter":
			query.WhereGroup(func(q *db.Query) (*db.Query, error) {
				ParseSearchQuery(m, q, mapping)
				return q, nil
			})
		case "must_not":
			// TODO: must_not query negation
			query.WhereGroup(func(q *db.Query) (*db.Query, error) {
				ParseSearchQuery(m, q, mapping)
				return q, nil
			})
		case "should":
			query.WhereOrGroup(func(q *db.Query) (*db.Query, error) {
				ParseSearchQuery(m, q, mapping)
				return q, nil
			})
		}
	}
}
