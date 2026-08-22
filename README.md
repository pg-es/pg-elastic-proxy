# pg-es-proxy

⚠️ **Alpha — not production-ready**

An Elasticsearch-compatible REST API proxy backed by PostgreSQL. Clients speak the Elasticsearch wire protocol; the proxy translates queries into SQL against Postgres (JSONB storage, `tsvector` full-text search).

Think of it as FerretDB for Elasticsearch.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  Elasticsearch Client (curl, Python, Node.js, etc.)         │
└────────────────────┬────────────────────────────────────────┘
                     │ ES wire protocol (HTTP/JSON)
                     ▼
┌─────────────────────────────────────────────────────────────┐
│  pg-es-proxy (HTTP server)                                  │
│  • Regex router (server/handler)                            │
│  • Query DSL → PostgreSQL tsvector translation              │
│  • Bulk, search, index, document CRUD                       │
└────────────────────┬────────────────────────────────────────┘
                     │ SQL (go-pg/pg ORM)
                     ▼
┌─────────────────────────────────────────────────────────────┐
│  PostgreSQL (JSONB documents, tsvector indexes)             │
└─────────────────────────────────────────────────────────────┘
```

## Status

This is an early-stage fork of [asp437/pg_elastic](https://github.com/asp437/pg_elastic). The proxy handles basic Elasticsearch API operations (cluster health, bulk, search, index, document CRUD) but is not feature-complete or production-ready.

## License

BSD 2-Clause License. See [LICENSE](LICENSE) for details.

Originally forked from [asp437/pg_elastic](https://github.com/asp437/pg_elastic).
