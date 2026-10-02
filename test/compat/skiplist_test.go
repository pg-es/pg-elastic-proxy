package compat_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pg-es/pg-es-proxy/test/compat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSkiplist(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	skiplistPath := filepath.Join(dir, "skiplist.yaml")

	content := "skips:\n" +
		"  - file: \"search/10_basic.yml\"\n" +
		"    reason: \"not implemented: search aggregations\"\n" +
		"  - file: \"scroll/10_basic.yml\"\n" +
		"    reason: \"not implemented: scroll API\"\n"

	err := os.WriteFile(skiplistPath, []byte(content), 0o600)
	require.NoError(t, err)

	sl, err := compat.LoadSkiplist(skiplistPath)
	require.NoError(t, err)
	require.Len(t, sl.Skips, 2)

	assert.Equal(t, "search/10_basic.yml", sl.Skips[0].File)
	assert.Equal(t, "not implemented: search aggregations", sl.Skips[0].Reason)
	assert.Equal(t, "scroll/10_basic.yml", sl.Skips[1].File)
	assert.Equal(t, "not implemented: scroll API", sl.Skips[1].Reason)
}

func TestLoadSkiplistFileNotFound(t *testing.T) {
	t.Parallel()

	_, err := compat.LoadSkiplist(filepath.Join(t.TempDir(), "nonexistent.yaml"))
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed to read skiplist file")
}

func TestSkiplistShouldSkip(t *testing.T) {
	t.Parallel()

	sl := compat.Skiplist{
		Skips: []compat.SkipEntry{
			{File: "bulk/10_basic.yml", Reason: "not implemented: bulk upsert"},
			{File: "cat/health.yml", Reason: "not implemented: cat API"},
		},
	}

	skip, reason := sl.ShouldSkip("bulk/10_basic.yml")
	assert.True(t, skip)
	assert.Equal(t, "not implemented: bulk upsert", reason)

	skip, reason = sl.ShouldSkip("cat/health.yml")
	assert.True(t, skip)
	assert.Equal(t, "not implemented: cat API", reason)
}

func TestSkiplistShouldNotSkip(t *testing.T) {
	t.Parallel()

	sl := compat.Skiplist{
		Skips: []compat.SkipEntry{
			{File: "index/10_basic.yml", Reason: "not implemented: index settings"},
		},
	}

	skip, reason := sl.ShouldSkip("get/10_basic.yml")
	assert.False(t, skip)
	assert.Empty(t, reason)
}
