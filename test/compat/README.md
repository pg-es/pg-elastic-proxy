# Elasticsearch Compatibility Tests

Runs the [elasticsearch-clients-tests](https://github.com/elastic/elasticsearch-clients-tests) suite against pg-es-proxy.

## Setup

After cloning the repo, populate the testdata submodule:

```bash
git submodule update --init test/compat/testdata
```

## Running

Start pg-es-proxy, then run the suite:

```bash
mise run compat:run
```

The script checks that the proxy is listening on `localhost:9200` and that the testdata submodule is populated before running `go test`.

## Updating testdata

Advance the submodule to the latest upstream commit:

```bash
mise run compat:update
```

This prints the new HEAD hash. Commit the updated submodule pointer in the same PR as any code changes that depend on the new test files.

## Action registry

The runner translates every ES action name the suite uses (`indices.create`,
`cluster.health`, `ml.put_job`, …) into an HTTP request using a generated
registry. Each entry records the action's URL paths, HTTP methods, path
parameters, and query parameters, so no action list is maintained by hand.

There is one registry per ES major — `action_registry_es8.go` and
`action_registry_es9.go` — because the two spec revisions differ (ES 9 adds
`esql.*` and `cat.circuit_breaker`, ES 8 has `indices.unfreeze`). The executor
picks the table from `COMPAT_FLAVOUR` (`es8`/`es9`, default `es9`), which CI
sets per matrix leg.

Regenerate both after updating the testdata submodule or moving to new targeted
ES releases:

```bash
COMPAT_ES_SPEC_ES8=/path/to/elasticsearch-8.19/rest-api-spec/src/main/resources/rest-api-spec/api \
COMPAT_ES_SPEC_ES9=/path/to/elasticsearch-main/rest-api-spec/src/main/resources/rest-api-spec/api \
mise run compat:gen
```

## Format coverage

The runner implements the operators the ES YAML format defines: `do` (with
`catch`, `headers`, `warnings`, `allowed_warnings`), `set`, `transform_and_set`,
`skip`, `requires`, `match` (including `/regex/` values), `is_true`, `is_false`,
`lt`/`lte`/`gt`/`gte`, `close_to`, `is_after`, `length`, `exists`, `contains`,
and `matches_in_any_order`. It substitutes `$name` and `${name}` stashes in
paths, values and headers, and stashes the response as `body` after every
request (the parsed JSON, or the raw text when the response is not JSON).

`node_selector` is accepted but not evaluated, since the target is a single
endpoint. `warnings_regex`, `allowed_warnings_regex` and `yaml` are not
implemented; tests requiring those runner features are skipped.

`skip` and `requires` gates are evaluated against the target (flavour, an
optional `COMPAT_ES_VERSION`, and the runner's supported features) rather than
skipped unconditionally. A file the parser cannot handle is reported as
`errored` and fails the run — never silently counted as skipped.

## Skiplist

`skiplist.yaml` lists test files to skip because the corresponding feature is not yet implemented. Each entry has a `file` (path relative to `testdata/`) and a `reason`.

Maintenance convention:

- Each PR that implements a feature removes the corresponding entries from `skiplist.yaml`.
- Never remove entries speculatively; only when the feature is tested and passing.
- Add new entries when upstream adds tests for features pg-es-proxy does not support.
