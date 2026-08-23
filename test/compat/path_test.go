package compat_test

import (
	"testing"

	"github.com/pg-es/pg-es-proxy/test/compat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetPathNested(t *testing.T) {
	t.Parallel()

	data := map[string]any{
		"response": map[string]any{
			"total": map[string]any{
				"value": "found",
			},
		},
	}

	got, ok := compat.GetPath(data, "response.total.value")

	require.True(t, ok)
	assert.Equal(t, "found", got)
}

func TestGetPathArrayIndex(t *testing.T) {
	t.Parallel()

	data := map[string]any{
		"hits": map[string]any{
			"hits": []any{
				map[string]any{"_id": "doc1"},
				map[string]any{"_id": "doc2"},
			},
		},
	}

	got, ok := compat.GetPath(data, "hits.hits.0._id")

	require.True(t, ok)
	assert.Equal(t, "doc1", got)
}

func TestGetPathMissing(t *testing.T) {
	t.Parallel()

	data := map[string]any{"foo": "bar"}

	got, ok := compat.GetPath(data, "foo.missing.deep")

	assert.False(t, ok)
	assert.Nil(t, got)
}

func TestGetPathEmptyPath(t *testing.T) {
	t.Parallel()

	data := map[string]any{"x": "val"}

	got, ok := compat.GetPath(data, "")

	require.True(t, ok)
	assert.Equal(t, data, got)
}

func TestGetPathDeepNest(t *testing.T) {
	t.Parallel()

	data := map[string]any{
		"a": map[string]any{
			"b": map[string]any{
				"c": map[string]any{
					"d": "deep",
				},
			},
		},
	}

	got, ok := compat.GetPath(data, "a.b.c.d")

	require.True(t, ok)
	assert.Equal(t, "deep", got)
}

func TestGetPathInvalidIndex(t *testing.T) {
	t.Parallel()

	data := map[string]any{
		"items": []any{"a", "b"},
	}

	got, ok := compat.GetPath(data, "items.notanumber")

	assert.False(t, ok)
	assert.Nil(t, got)
}
