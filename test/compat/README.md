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

## Skiplist

`skiplist.yaml` lists test files to skip because the corresponding feature is not yet implemented. Each entry has a `file` (path relative to `testdata/`) and a `reason`.

Maintenance convention:

- Each PR that implements a feature removes the corresponding entries from `skiplist.yaml`.
- Never remove entries speculatively; only when the feature is tested and passing.
- Add new entries when upstream adds tests for features pg-es-proxy does not support.
