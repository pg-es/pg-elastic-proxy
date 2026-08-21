# AGENTS.md

Go craft rules for this repo.

General Go style, error handling, testing, concurrency, and logging conventions live in the installed `golang-*` skills. Load the relevant one instead of guessing. This file covers only what those skills cannot know: this repo's toolchain, its lint config, and its boundaries.

## Project

pg_elastic is an HTTP server that exposes an Elasticsearch-compatible REST API backed by PostgreSQL. Clients speak the ES wire protocol; pg_elastic translates queries into SQL against Postgres (JSONB storage, `tsvector` full-text search).

A separate migration CLI (`pg_elastic_migrate/`) copies data from a real Elasticsearch cluster into the PostgreSQL backend.

### Structure

```
main.go                      entrypoint — reads config, starts server
server/                      PGElasticServer interface + ElasticHandler (regex router)
internal/server/             concrete server implementation, route wiring
api/                         ES API handlers: bulk, cluster health, documents, indices
api/search/                  ES query DSL → PostgreSQL tsvector query translation
db/                          PostgreSQL client (go-pg/pg ORM), index/type/document CRUD
utils/                       config loader, ElasticError hierarchy, field mapping
pg_elastic_migrate/          separate main package — ES→PG migration CLI
tests/                       Python/pytest black-box tests (NOT Go tests)
```

`server/` defines the interface and HTTP handler; `internal/server/` holds the implementation and route table. Do not merge them — `internal/` restricts import visibility.

### Dependencies

- **`github.com/go-pg/pg`** — PostgreSQL ORM used by `db/`. Not `database/sql`, not `pgx`.
- **`gopkg.in/olivere/elastic.v5`** — Elasticsearch client, used only in `pg_elastic_migrate/`.

### Config

Runtime config is read from `pg_elastic_config.json` in the working directory (JSON, not env vars). See the file in the repo root for the schema. The server does not accept CLI flags.

## Quick reference

| Task | Command |
|---|---|
| Build | `go build ./...` |
| Test (iterating) | `go test ./path/to/pkg -run TestName` |
| Test (before done) | `go test ./...` |
| Test with race | `go test -race ./...` |
| Lint + autofix | `golangci-lint run --fix` |
| Lint check only | `golangci-lint run` |
| Format | `golangci-lint fmt` |
| Vet | `go vet ./...` |
| Modernize | `go fix ./...` |
| Tidy | `go mod tidy` |
| Vulnerabilities | `go tool govulncheck ./...` |

Run `golangci-lint run --fix` before reporting work complete. It rewrites many modernization fixes automatically — `interface{}` → `any`, counting loops → `for i := range n`, `%v` → `%w` on errors, hand-rolled min/max → builtins.

## Toolchain

Tools are pinned in `mise.toml` and resolved by mise. Run them via `mise exec -- <tool>` or rely on mise shell activation.

- Add a tool with `mise use <tool>` or `mise use "go:<module path>@latest"`. This edits the repo's `mise.toml`.
- **Never** `go install` a tool. It writes into the mise Go install dir, is not tracked, and breaks on the next Go upgrade.
- **Never** `brew install` a tool.
- **Never** edit `~/.config/mise/config.toml` or anything under `~/dot_files`. Repo-local config only.

## Linting

`.golangci.yml` is golangci-lint **v2** format. Do not introduce v1 keys — `linters-settings`, `disable-all`, `enable-all`, `govet.check-shadowing`, `issues.exclude-use-default`, `run.skip-dirs` are all invalid. Formatters live under the top-level `formatters:` block, not `linters.enable`.

Verify any config edit with `golangci-lint config verify` before committing.

`//nolint` requires both a specific linter name and an explanation, because `nolintlint` enforces `require-explanation` and `require-specific`:

```go
//nolint:gosec // path is validated by callers, see validatePath
```

Two linters can report the same line:col; golangci-lint shows only one. If a fix seems to do nothing, re-run with `--enable-only=<linter>`.

## Shell and data processing

- Use `jq` for JSON and `yq` for YAML. Do not shell out to `python3` to read or edit JSON/YAML.
- Both are installed. `jq` here is `gojq`; `yq` is mikefarah's Go implementation.
- Write edits to a temp file and move it into place — `jq ... file > tmp && mv tmp file`. Redirecting onto the input file truncates it.
- Prefer `rg` over `grep` and `fd` over `find`.

## Workflow

Work on a feature branch, never commit directly to `master`. Branch names: `<type>/<short-description>` (e.g. `feat/bulk-upsert`, `fix/search-pagination`).

Open a pull request for every change. Keep PRs focused — one logical change per PR. Ensure `golangci-lint run --fix` and `go test ./...` pass before opening.

## Boundaries

**Always** run `golangci-lint run --fix` and `go test ./...` before declaring work done.

**Ask first** before adding a dependency, changing the Go version in `go.mod`, editing `.golangci.yml`, or creating a new top-level package.

**Never** commit directly to `master`. **Never** silence a type error with `any` casts or blank-identifier assignments to make the build pass. **Never** delete or skip a failing test to get green. **Never** touch anything outside this repository.
