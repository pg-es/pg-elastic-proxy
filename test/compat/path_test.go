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
				testValue: fieldFound,
			},
		},
	}

	got, ok := compat.GetPath(data, "response.total.value", nil)

	require.True(t, ok)
	assert.Equal(t, fieldFound, got)
}

func TestGetPathArrayIndex(t *testing.T) {
	t.Parallel()

	data := map[string]any{
		testHits: map[string]any{
			testHits: []any{
				map[string]any{fieldID: "doc1"},
				map[string]any{fieldID: "doc2"},
			},
		},
	}

	got, ok := compat.GetPath(data, "hits.hits.0._id", nil)

	require.True(t, ok)
	assert.Equal(t, "doc1", got)
}

func TestGetPathMissing(t *testing.T) {
	t.Parallel()

	data := map[string]any{"foo": "bar"}

	got, ok := compat.GetPath(data, "foo.missing.deep", nil)

	assert.False(t, ok)
	assert.Nil(t, got)
}

func TestGetPathEmptyPath(t *testing.T) {
	t.Parallel()

	data := map[string]any{"x": "val"}

	got, ok := compat.GetPath(data, "", nil)

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

	got, ok := compat.GetPath(data, "a.b.c.d", nil)

	require.True(t, ok)
	assert.Equal(t, "deep", got)
}

func TestGetPathInvalidIndex(t *testing.T) {
	t.Parallel()

	data := map[string]any{
		"items": []any{"a", "b"},
	}

	got, ok := compat.GetPath(data, "items.notanumber", nil)

	assert.False(t, ok)
	assert.Nil(t, got)
}

func TestGetPathStash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data map[string]any
		vars map[string]any
		path string
		want any
		ok   bool
	}{
		{
			name: "whole path is a stash returns the raw value",
			vars: map[string]any{"body": "raw response text"},
			path: testBodyStash,
			want: "raw response text",
			ok:   true,
		},
		{
			name: "stash used as a map key",
			data: map[string]any{"nodes": map[string]any{"n1": map[string]any{"transport": "yes"}}},
			vars: map[string]any{"master": "n1"},
			path: "nodes.$master.transport",
			want: "yes",
			ok:   true,
		},
		{
			name: "missing stash fails",
			vars: map[string]any{},
			path: "$nope",
			want: nil,
			ok:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := compat.GetPath(tt.data, tt.path, tt.vars)

			assert.Equal(t, tt.ok, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}
