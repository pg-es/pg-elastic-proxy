//go:build compat

package compat_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/pg-es/pg-es-proxy/test/compat"
	"github.com/stretchr/testify/require"
)

func TestCompat(t *testing.T) {
	t.Parallel()

	baseURL := os.Getenv("COMPAT_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:9200"
	}

	executor := compat.NewExecutor(baseURL, http.DefaultClient)

	flavour := os.Getenv("COMPAT_FLAVOUR")
	if flavour == "" {
		flavour = "es9"
	}

	target := compat.Target{Flavour: flavour, Version: os.Getenv("COMPAT_ES_VERSION")}

	skiplist, err := compat.LoadSkiplist("skiplist.yaml")
	if err != nil {
		t.Logf("skiplist not found, running all tests: %v", err)

		skiplist = compat.Skiplist{}
	}

	var filePaths []string

	for _, pattern := range []string{"testdata/tests/*.yml", "testdata/tests/*/*.yml"} {
		matches, err := filepath.Glob(pattern)
		require.NoError(t, err)

		filePaths = append(filePaths, matches...)
	}

	sort.Strings(filePaths)

	var total, passed, failed, skipped, errored int

	for _, filePath := range filePaths {
		total++

		fileHandle, err := os.Open(filePath)
		if err != nil {
			t.Errorf("open %s: %v", filePath, err)

			errored++

			continue
		}

		testFile, parseErr := compat.ParseTestFile(fileHandle)
		fileHandle.Close()

		if parseErr != nil {
			t.Errorf("parse %s: %v", filePath, parseErr)

			errored++

			continue
		}

		relPath := strings.TrimPrefix(filePath, "testdata/")

		if skip, reason := skiplist.ShouldSkip(relPath); skip {
			t.Logf("skip %s: %s", filePath, reason)
			skipped++

			continue
		}

		if !target.SatisfiesRequires(&testFile.Requires) {
			skipped++

			continue
		}

		result := compat.RunFile(context.Background(), &testFile, executor, target)
		passed += result.Passed
		failed += result.Failed
		skipped += result.Skipped

		for _, failure := range result.Failures {
			t.Logf("FAIL %s / %s[%d]: %s", filePath, failure.TestName, failure.StepIndex, failure.Err)
		}
	}

	t.Logf("compat: total=%d passed=%d failed=%d skipped=%d errored=%d", total, passed, failed, skipped, errored)
}
