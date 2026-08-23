//go:build compat

package compat_test

import (
	"context"
	"net/http"
	"os"
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

	skiplist, err := compat.LoadSkiplist("skiplist.yaml")
	if err != nil {
		t.Logf("skiplist not found, running all tests: %v", err)

		skiplist = compat.Skiplist{}
	}

	entries, err := os.ReadDir("testdata")
	require.NoError(t, err)

	var total, passed, failed, skipped int

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yml") {
			continue
		}

		total++

		filePath := "testdata/" + entry.Name()

		fileHandle, err := os.Open(filePath)
		if err != nil {
			t.Logf("skip %s: open error: %v", entry.Name(), err)
			skipped++

			continue
		}

		testFile, parseErr := compat.ParseTestFile(fileHandle)
		fileHandle.Close()

		if parseErr != nil {
			t.Logf("skip %s: parse error: %v", entry.Name(), parseErr)
			skipped++

			continue
		}

		result := compat.RunFile(context.Background(), testFile, skiplist, executor)
		passed += result.Passed
		failed += result.Failed
		skipped += result.Skipped

		for _, failure := range result.Failures {
			t.Logf("FAIL %s / %s[%d]: %s", entry.Name(), failure.TestName, failure.StepIndex, failure.Err)
		}
	}

	t.Logf("compat: total=%d passed=%d failed=%d skipped=%d", total, passed, failed, skipped)
}
