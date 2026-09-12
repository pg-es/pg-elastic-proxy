# Elasticsearch Compatibility

This document describes what pg_es (`pg-es-proxy`) actually implements of the
Elasticsearch REST API, what it deliberately does not, and the trade-offs of
running it instead of a real Elasticsearch cluster.

It is **the source of truth for compatibility claims**. When you change the
API surface, the query DSL, or a response shape, update this file in the same
change. The maintenance convention lives in [`AGENTS.md`](../AGENTS.md).

## Source of truth

pg_es reimplements parts of the Elasticsearch REST API. When matching ES
behaviour — error shapes, status codes, query DSL, response fields — the
**official Elasticsearch documentation** is the contract, not the Java source
code. Implementation details in the Java codebase are not stable and not ours
to copy.

**Never** reverse-engineer behaviour from the Elasticsearch Java source. If
the docs are ambiguous, test against a running ES instance or ask.

## Trade-offs: pg_es vs a real Elasticsearch cluster

pg_es is a *compatibility* play, not a *capability* play. It buys "clients that
speak ES keep working against data already in Postgres" — a pragmatic choice
when the search workload is modest, the query surface is narrow, and you would
rather not operate a second system. It trades search fidelity and query
breadth for operational simplicity and transactional consistency.

### What you give up (why you might want real ES)

- **Search quality.** ES is Lucene under the hood: BM25 scoring, pluggable
  analyzers, per-language stemming, synonyms, fuzzy matching, phrase
  proximity. Postgres `tsvector` uses a fixed parser with weak CJK
  segmentation and a much cruder ranking model. If you need relevance tuning,
  typo tolerance, or non-Latin text, pg_es falls short and the gap is hard to
  close.
- **Query DSL coverage.** ES has `nested`, `parent-child`, `geo_*`, `script`,
  `match_phrase_prefix`, `fuzzy`, `regexp`, wildcard, and rich aggregations.
  pg_es translates a small subset (`match`, `match_phrase`, `bool`,
  `match_all`) into SQL. Queries that do not map cleanly to a `tsvector`/JSONB
  operation are either unsupported or silently return everything (see
  [Query DSL](#query-dsl-support)).
- **Scaling and indexing performance.** ES is a distributed inverted index —
  shard across nodes, near-real-time search, bulk ingest tuned for search
  workloads. Postgres is row-oriented; even with a GIN index on `tsvector`,
  large-scale search-heavy reads are a different beast, and you pay ACID /
  transaction overhead that ES does not have.
- **Aggregations and analytics.** ES's aggregation framework and Painless
  scripting are purpose-built. Postgres can do it with SQL `GROUP BY` /
  window functions, but the semantics differ enough that ES-specific
  analytics queries will not translate.

### What you keep (why pg_es is appealing)

- **Operational simplicity.** You already run Postgres. No second JVM cluster,
  no shard rebalancing, no snapshot/restore tooling, no memory-hungry nodes,
  no Elastic licence considerations. Backups are `pg_dump`; observability is
  the database you already monitor.
- **ACID + relational integrity.** Transactions, joins, foreign keys,
  consistent reads. ES is eventually consistent with a refresh-interval
  trade-off and no joins (you denormalize). If the data also lives
  relationally, keeping search in the same store avoids sync / dual-write
  pipelines.
- **Cost.** One database footprint instead of a dedicated search cluster. For
  modest data volumes this is a clear win.

### Decision framework

The choice comes down to three questions:

1. **How sophisticated does search need to be?** Relevance scoring, fuzzy,
   multi-language → lean ES. Simple keyword match → pg_es is fine.
2. **How big is the corpus?** Search-heavy at scale → ES.
3. **How much do you value single-store consistency + not running another
   cluster?** → pg_es.

## Implemented REST API surface

All routes are registered in `internal/server/router.go:17-35` using Go 1.22+
method/pattern syntax. There are exactly 11 routes; nothing else exists. No
endpoint returns `501` — unsupported features are either **unrouted**
(ServeMux → 404/405) or **silently ignored**.

Path parameters must be non-empty and not `_`-prefixed, else 404
(`internal/server/router.go:37-57`). Panics are recovered to 500
(`router.go:44-50`). Every response carries `X-Elastic-Product:
Elasticsearch` (`internal/server/middleware.go:17`).

| Method | Path | Handler | Location |
|---|---|---|---|
| GET | `/_cluster/health` | `api.HealthHandler` | `router.go:20` / `api/cluster.go:28` |
| POST | `/_bulk` | `api.BulkHandler` | `router.go:21` / `api/bulk.go:36` |
| PUT | `/{index}/_mapping/{type}` | `api.PutTypeMapping` | `router.go:23` / `api/indices.go:54` |
| GET | `/{index}/_search` | `api.FindIndexDocumentHandler` | `router.go:24` / `api/documents.go:299` |
| PUT | `/{index}` | `api.PutIndexHandler` | `router.go:25` / `api/indices.go:22` |
| HEAD | `/{index}` | `api.HeadIndexHandler` | `router.go:26` / `api/indices.go:41` |
| GET | `/{index}/{type}/_search` | `api.FindDocumentHandler` | `router.go:28` / `api/documents.go:294` |
| PUT/POST | `/{index}/{type}/{id}` | `api.PutDocumentHandler` | `router.go:29-30` / `api/documents.go:91` |
| GET | `/{index}/{type}/{id}` | `api.GetDocumentHandler` | `router.go:31` / `api/documents.go:132` |
| DELETE | `/{index}/{type}/{id}` | `api.DeleteDocumentHandler` | `router.go:32` / `api/documents.go:163` |

**Not routed (404/405):** root info `GET /`, document-exists `HEAD
/{index}/{type}/{id}`, auto-ID index `POST /{index}/{type}`, partial update
`POST /{index}/{type}/{id}/_update`, POST-form search `POST /{index}/_search`,
`DELETE /{index}`, `GET /{index}`, `/{index}/_settings`,
`/{index}/_mapping` (typeless), `/_count`, `/_mget`, `/_msearch`, `/_scroll`,
`/_cat/*`, `/_nodes`, `/_aliases`, `/_refresh`, `/_flush`, `/_forcemerge`,
`/_template/*`, `/_index_template/*`, `/_source/{...}`.

### Cluster health — GET /_cluster/health

`api/cluster.go:28-34`. Returns `cluster_name` (hardcoded `"pg_elastic_cluster"`),
`status` (hardcoded `"yellow"`), `number_of_nodes` (1), and zeroed shard
counters. Query params (`wait_for_status`, `timeout`, `level`, `local`) are
ignored; status never reflects real state.

### Bulk — POST /_bulk

`api/bulk.go:36-46`, processing `api/bulk.go:194-242`. NDJSON split on `\n`;
each action line is paired with its payload line. Descriptor fields parsed:
`_index`, `_type`, `_id` only (`bulk.go:62-72`).

- `index` — upsert: update if the doc exists, else create
  (`resolveAndUpsertDocument`, `bulk.go:77-97`). Matches ES.
- `create` — `CreateDocument`; existing ID → 409
  `version_conflict_engine_exception` (`db/client.go:443-445`). Matches ES.
- `update` — **gap**: the payload is stored verbatim. ES expects a
  `{"doc": {...}}` partial-merge wrapper; here the wrapper is stored as the
  document itself. Missing doc → 500 `db_query_exception` (`db/client.go:504`);
  ES returns 404 `document_missing_exception`.
- `delete` — returns `found` + `_source` in the item (`bulk.go:142-159`); ES
  omits `_source` unless requested.

Per-item errors are wrapped into `ElasticBulkError` (`api/bulk.go:161-191`,
`utils/errors.go:66-93`), top-level `errors: true`, processing continues.
Malformed JSON in any line aborts the whole request with 400
`json_parse_exception` (`bulk.go:207-210`). **Gaps:** unknown action yields an
empty item with no error; no `?refresh`; no per-item status codes beyond 200;
no `max_retries`; delete of a missing doc reports `found:false` 200.

### Document CRUD

- **PUT/POST `/{index}/{type}/{id}`** — `api/documents.go:91-129`. Upsert
  (`putOrUpdateDocument`, `documents.go:75-88`); auto-creates a missing
  index/type with empty options (`db/client.go:410-432`). Response:
  `_shards`, `_index`, `_type`, `_id`, `_version`, `created`, `result`.
  **Gaps:** the `id==""` auto-ID branch (`documents.go:102-103`) is
  unreachable because `{id}` must be non-empty; no `?refresh`, `op_type`,
  `version`, `if_seq_no`, `routing`, `timeout`.
- **GET `/{index}/{type}/{id}`** — `api/documents.go:132-160`. Found →
  `found:true` + `_source`; missing → `found:false` 200 (matches ES).
  **Gaps:** no `_source`/`_source_includes`/`_source_excludes`, `stored_fields`,
  `realtime`, `preference`, `refresh` params; no HEAD variant.
- **DELETE `/{index}/{type}/{id}`** — `api/documents.go:163-191`. **Gap:**
  missing doc → 200 `found:false`; ES returns 404 `result:"not_found"`. No
  `result`/`_shards` fields, no `?refresh`, `if_seq_no`, `version`.

`_source` is stored verbatim as JSONB and returned as-is; no filtering or
inclusion/exclusion anywhere. **Refresh** is not modelled — there is no
refresh parameter on any write, and Postgres visibility is immediate.
**Versioning** uses an internal `version` column incremented on update
(`db/client.go:594`), starting at 1; no external versioning or optimistic
concurrency (`if_seq_no`/`if_primary_term`).

### Index management

- **PUT `/{index}`** — `api/indices.go:22-38`. Body stored **raw** as
  `options` text (`indices.go:30-32`); settings/mappings/aliases are not
  parsed, validated, or applied. Duplicate → 400
  `resource_already_exists_exception` (`db/client.go:192-194`). Response:
  `{acknowledged:true, shards_acknowledged:true}`.
- **HEAD `/{index}`** — `api/indices.go:41-51`. **Gap:** always 200; a missing
  index should be 404 (the body is suppressed for HEAD regardless).
- **PUT `/{index}/_mapping/{type}`** — `api/indices.go:54-81`. Stores the raw
  body as type options. Only `properties.<field>.{type,analyzer}` are ever read
  back (`utils/mapping.go:10-37`). **Gaps:** no typeless mapping endpoint; no
  GET mapping; no field-type validation; no dynamic mapping on insert
  (auto-created types get empty options, `db/client.go:428`).

### Search — GET /{index}/_search and GET /{index}/{type}/_search

Flow: `api/documents.go:244-291` → `api/documents.go:193-242` →
`api/search/parsequery.go:36-38` → `db/client.go:101-147`. Index/type
wildcards `*` and `?` are translated to SQL `LIKE` (`db/client.go:225-232`,
`340-347`).

**Gaps (major):** only the **first** matching index/type is searched — the
handler returns inside the loop (`documents.go:286`); multi-index/multi-type
results are never merged. The body must contain a `query` object, else 400
`illegal_argument_exception` "Illegal search query" (`documents.go:276-290`);
an empty body → 400 `json_parse_exception` (`documents.go:259-262`) — ES
treats an empty body as `match_all`.

**Response** (`documents.go:58-63`, `217-241`): `took`, `timed_out:false`,
`_shards{1,0,1}`, `hits{max_score, total, hits[]}`. Each hit:
`_index`, `_type`, `_id`, `_score` (constant 1.0, `documents.go:70`),
`_source`. **Gaps:** `hits.total` is a plain int, not the ES 7+
`{value, relation}` object; `max_score` is 0 when there are no hits (ES:
`null`).

## Query DSL support

All query parsing lives in `api/search/parsequery.go`. `parseQueryClauses`
(`parsequery.go:56-84`) dispatches on a **fixed key list**
(`parsequery.go:59`): `match_all`, `match`, `match_phrase`, `bool`.
Everything else is **silently ignored** (`parsequery.go:54-55`).

### Supported clauses

| Clause | Location | SQL mapping |
|---|---|---|
| `match_all` | `parsequery.go:71-73` | No constraint; empty clause dropped → no WHERE → all docs |
| `match` | `parsequery.go:74-75`, `151-153` | `to_tsvector(document->field) @@ to_tsquery(value)` per field; fields ANDed, visited in sorted order (`parsequery.go:121-147`) |
| `match_phrase` | `parsequery.go:76-77`, `157-159` | Same shape with `phraseto_tsquery(value)` |
| `bool` | `parsequery.go:78-79`, `167-198` | See below |

**`match` value forms** (`parsequery.go:133-140`): shorthand string
(`{"match": {"f": "v"}}`) or object form, but the object form only extracts
the `"query"` key (`extractQueryString`, `parsequery.go:110-114`). Other match
options (`operator`, `analyzer`, `fuzziness`, per-field
`minimum_should_match`) are ignored. Non-string, non-object field values
(numbers, bools) are skipped (`parsequery.go:138-139`).

**Analyzer support** (`buildTSQueryClause`, `parsequery.go:92-106`): if the
type mapping declares an `analyzer` for the field (read by
`utils.GetFieldMapping`, `utils/mapping.go:10-37`), the clause becomes
`to_tsvector(analyzer, document->field) @@ tsfunc(analyzer, value)`;
otherwise the default parser is used.

**Parameterization**: every field name, analyzer, and value is bound via
`cond.Var` → `$?` markers (`toSuccessiveExpr`, `parsequery.go:48-50`); nothing
is string-interpolated.

### bool query logic

`parseBoolQuery` (`parsequery.go:167-198`):

- Sub-clause keys visited in fixed order: `must`, `filter`, `should`,
  `must_not` (`parsequery.go:171`).
- `must` and `filter`: each group's sub-queries ANDed; groups ANDed together
  (`parsequery.go:183-186`). `filter` gets no special treatment — there is no
  scoring distinction at all.
- `should`: ORed among themselves **only when there is no `must`/`filter`**
  (`parsequery.go:187-190`), emulating the ES default `minimum_should_match`
  (1 when should-only, 0 otherwise). The explicit `minimum_should_match`
  parameter is **not parsed**.
- `must_not`: **NOT negated** — parsed and ANDed like `must`
  (`parsequery.go:191-193`, marked `// TODO: must_not query negation`). A
  `must_not` clause currently *requires* the condition instead of excluding
  it.
- Nesting: `bool` inside `bool` recurses through `parseQueryClauses`.

### Unsupported clauses — silently ignored, NOT errors

Anything outside the fixed key list is skipped (`parsequery.go:54-55`). This
includes at minimum: `term`, `terms`, `range`, `exists`, `wildcard`,
`prefix`, `regexp`, `fuzzy`, `multi_match`, `query_string`,
`simple_query_string`, `ids`, `match_none`, `nested`, `geo_*`, `script`,
`function_score`, and all others.

> **Critical caveat:** an unsupported clause does **not** fail the request — it
> degrades to an unconstrained search returning every document. There is no
> error path for unknown query types anywhere in the package. A query built
> only from unsupported clauses matches **all** documents.

### Scoring

There is none. `_score` is hardcoded `1.0` for every hit
(`formatDocumentSearchResponse`, `api/documents.go:65-73`); `max_score` is the
max over hits (`documents.go:232-234`); `hits.total` is a plain int count
(`documents.go:236`). Results are returned in Postgres natural order — there
is no relevance ordering.

## Request parameters

**Handled:**
- Body key `"query"` — the only body key read (`findDocuments`,
  `api/documents.go:276`).
- URL param `pretty` — indented JSON output when `"true"` **or absent**
  (`internal/server/router.go:91-102`). **Gap:** ES defaults to compact
  output; here an absent `pretty` yields indented output.
- Path wildcards `*` / `?` for index and type patterns.

**Ignored (never read):** `size`, `from`, `sort`, `_source` /
`_source_includes` / `_source_excludes`, `highlight`, `aggs` /
`aggregations`, `track_total_hits`, `timeout`, `min_score`, `post_filter`,
`collapse`, `explain`, `version`, `seq_no_primary_term`, `stored_fields`,
`docvalue_fields`, `terminate_after`, `profile`.

**No pagination whatsoever**: `buildSearchSelect` (`db/client.go:164-175`)
emits `SELECT id, document, version FROM <table> [WHERE ...]` with no
`LIMIT`/`OFFSET`/`ORDER BY`; `buildSearchCount` (`db/client.go:150-161`) runs
a `COUNT(*)` first (`db/client.go:107`). Every matching row is returned.

## Error model

`utils/errors.go` — ES-shaped `{error:{root_cause[],type,reason},status}`
(`errors.go:44-57`):

| Error type | Status | Location |
|---|---|---|
| `json_parse_exception` | 400 | `errors.go:101-108` |
| `illegal_argument_exception` | 400 | `errors.go:145-153` |
| `resource_already_exists_exception` | 400 | `errors.go:160-168` |
| `version_conflict_engine_exception` | 409 | `errors.go:175-183` |
| `db_query_exception` | 500 | `errors.go:115-118` |
| `internal_io_exception` | 500 | `errors.go:125-128` |
| `internal_exception` | 500 | `errors.go:135-138` |

**Gaps:** there is no `index_not_found_exception` (404) anywhere — searching
or GETting a nonexistent index/type does not produce ES's 404 shape.
`db_query_exception` is a non-ES type name used where ES would return 404
`document_missing_exception` or `index_not_found_exception`. Non-ElasticError
failures degrade to a plain 500 `{"error":"internal server error"}`
(`router.go:78-80`), not ES-shaped.

## Known limitations (summary)

The highest-impact gaps, in order:

1. **Unsupported query clauses silently match-all** instead of erroring.
2. **`must_not` is not negated** — it requires the condition
   (`parsequery.go:192`).
3. **No POST `/_search`** — only GET search is routed, so clients that POST a
   search body get 405/404.
4. **No pagination, sort, aggregations, or `_source` filtering** — every
   matching row is returned in natural order with `_score` always 1.0.
5. **Single-index search only** — multi-index/multi-type results are not
   merged (`documents.go:286`).
6. **Bulk `update` ignores the `{"doc": {...}}` wrapper** and stores it
   verbatim.
7. **No 404 `index_not_found_exception`** — nonexistent indices/types do not
   return ES's 404 shape.
8. **HEAD `/{index}` never 404s** and document-delete of a missing doc returns
   200 `found:false`.
9. **`hits.total` is a plain int**, not the ES 7+ `{value, relation}` object.
10. **Search quality is bounded by Postgres `tsvector`** — no BM25 relevance,
    no fuzzy/typo tolerance, weak CJK segmentation, no scoring.

## Compatibility testing

Automated compatibility is exercised against the
[elasticsearch-clients-tests](https://github.com/elastic/elasticsearch-clients-tests)
suite. See [`test/compat/README.md`](../test/compat/README.md) for setup and
run instructions.

- **`test/compat/`** — the Go test runner that executes the ES client tests
  against a running pg_es proxy.
- **`test/compat/skiplist.yaml`** — manually maintained list of test files
  skipped because the feature is not yet implemented. Each entry has a `file`
  and a `reason`. Remove entries in the PR that implements the feature; never
  remove speculatively. This file is the machine-readable counterpart to the
  "not implemented" sections above.
- **`test/compat/testdata/`** — the pinned upstream test suite (git submodule).

**Keep this doc and `skiplist.yaml` in sync.** When you add an endpoint or DSL
clause, update the relevant section above *and* remove the matching skiplist
entry.
