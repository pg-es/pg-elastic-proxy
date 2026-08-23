package compat_test

import (
	"strings"
	"testing"

	"github.com/pg-es/pg-es-proxy/test/compat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const minimalYAML = `
setup:
  - do:
      indices.create:
        index: test-index

teardown:
  - do:
      indices.delete:
        index: test-index

"Test basic search":
  - do:
      search:
        index: test-index
  - match:
      hits.total.value: 0
`

func TestParseMinimalFile(t *testing.T) {
	t.Parallel()

	tf, err := compat.ParseTestFile(strings.NewReader(minimalYAML))

	require.NoError(t, err)
	require.Len(t, tf.Setup, 1)
	require.NotNil(t, tf.Setup[0].Do)
	assert.Equal(t, "indices.create", tf.Setup[0].Do.Name)

	require.Len(t, tf.Teardown, 1)
	require.NotNil(t, tf.Teardown[0].Do)
	assert.Equal(t, "indices.delete", tf.Teardown[0].Do.Name)

	require.Len(t, tf.Tests, 1)
	assert.Equal(t, "Test basic search", tf.Tests[0].Name)
	require.Len(t, tf.Tests[0].Steps, 2)
}

const multiTestYAML = `
"First test":
  - do:
      bulk:
        index: foo
        id: "1"

"Second test":
  - do:
      get:
        index: foo
        id: "1"

"Third test":
  - is_true: _source.title
`

func TestParseMultipleTests(t *testing.T) {
	t.Parallel()

	tf, err := compat.ParseTestFile(strings.NewReader(multiTestYAML))

	require.NoError(t, err)
	require.Len(t, tf.Tests, 3)
	assert.Equal(t, "First test", tf.Tests[0].Name)
	assert.Equal(t, "Second test", tf.Tests[1].Name)
	assert.Equal(t, "Third test", tf.Tests[2].Name)
}

const doActionYAML = `
"Test do action":
  - do:
      bulk:
        index: myindex
        id: "42"
        body:
          title: hello
`

func TestParseDoAction(t *testing.T) {
	t.Parallel()

	tf, err := compat.ParseTestFile(strings.NewReader(doActionYAML))

	require.NoError(t, err)
	require.Len(t, tf.Tests, 1)
	require.Len(t, tf.Tests[0].Steps, 1)

	step := tf.Tests[0].Steps[0]
	require.NotNil(t, step.Do)
	assert.Equal(t, "bulk", step.Do.Name)
	assert.Equal(t, "myindex", step.Do.Params["index"])
	assert.Equal(t, "42", step.Do.Params["id"])
}

const assertionsYAML = `
"Test assertions":
  - match:
      hits.total.value: 1
  - is_true: hits.hits.0._id
  - is_false: _source.missing
  - length:
      hits.hits: 1
  - gte:
      took: 0
  - lte:
      took: 1000
`

func TestParseAssertions(t *testing.T) {
	t.Parallel()

	tf, err := compat.ParseTestFile(strings.NewReader(assertionsYAML))

	require.NoError(t, err)
	require.Len(t, tf.Tests, 1)

	steps := tf.Tests[0].Steps

	expectedTypes := []string{"match", "is_true", "is_false", "length", "gte", "lte"}
	require.Len(t, steps, len(expectedTypes))

	for idx, wantType := range expectedTypes {
		require.NotNil(t, steps[idx].Assert, "step %d should be an assertion", idx)
		assert.Equal(t, wantType, steps[idx].Assert.Type, "step %d type", idx)
	}
}

const emptySetupTeardownYAML = `
"Test with no setup":
  - do:
      search:
        index: anything
`

func TestParseEmptySetupTeardown(t *testing.T) {
	t.Parallel()

	tf, err := compat.ParseTestFile(strings.NewReader(emptySetupTeardownYAML))

	require.NoError(t, err)
	assert.Empty(t, tf.Setup)
	assert.Empty(t, tf.Teardown)
	require.Len(t, tf.Tests, 1)
}
