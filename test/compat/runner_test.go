package compat_test

import (
	"fmt"
	"os"
	"path/filepath"
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
	assert.Equal(t, compat.ActionIndicesCreate, tf.Setup[0].Do.Name)

	require.Len(t, tf.Teardown, 1)
	require.NotNil(t, tf.Teardown[0].Do)
	assert.Equal(t, compat.ActionIndicesDelete, tf.Teardown[0].Do.Name)

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
	assert.Equal(t, compat.ActionBulk, step.Do.Name)
	assert.Equal(t, "myindex", step.Do.Params[compat.ParamIndex])
	assert.Equal(t, "42", step.Do.Params[compat.ParamID])
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

	expectedTypes := []string{
		compat.AssertMatch,
		compat.AssertIsTrue,
		compat.AssertIsFalse,
		compat.AssertLength,
		compat.AssertGTE,
		compat.AssertLTE,
	}
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

const multiDocRequiresYAML = `---
requires:
  serverless: true
  stack: true
---
teardown:
  - do:
      indices.delete:
        index: test-index
---
"Test search":
  - do:
      search:
        index: test-index
`

func TestParseRequiresMultiDoc(t *testing.T) {
	t.Parallel()

	tf, err := compat.ParseTestFile(strings.NewReader(multiDocRequiresYAML))

	require.NoError(t, err)
	require.NotNil(t, tf.Requires.Stack)
	require.NotNil(t, tf.Requires.Serverless)
	assert.True(t, *tf.Requires.Stack)
	assert.True(t, *tf.Requires.Serverless)
	require.Len(t, tf.Teardown, 1)
	require.Len(t, tf.Tests, 1)
	assert.Equal(t, "Test search", tf.Tests[0].Name)
}

const serverlessFlavour = "serverless"

func TestRequiresAppliesTo(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		requires compat.Requires
		flavour  string
		want     bool
	}{
		{
			name:     "stack true runs on stack flavour",
			requires: compat.Requires{Stack: new(true)},
			flavour:  testFlavour,
			want:     true,
		},
		{
			name:     "stack false skipped on stack flavour",
			requires: compat.Requires{Stack: new(false), Serverless: new(true)},
			flavour:  testFlavour,
			want:     false,
		},
		{
			name:     "serverless only skipped on os2 flavour",
			requires: compat.Requires{Stack: new(false)},
			flavour:  "os2",
			want:     false,
		},
		{
			name:     "both runs on serverless target",
			requires: compat.Requires{Stack: new(true), Serverless: new(true)},
			flavour:  serverlessFlavour,
			want:     true,
		},
		{
			name:     "stack only skipped on serverless target",
			requires: compat.Requires{Stack: new(true), Serverless: new(false)},
			flavour:  serverlessFlavour,
			want:     false,
		},
		{
			name:     "absent requirements run on stack flavour",
			requires: compat.Requires{},
			flavour:  "es8",
			want:     true,
		},
		{
			name:     "absent requirements run on serverless target",
			requires: compat.Requires{},
			flavour:  serverlessFlavour,
			want:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, tc.requires.AppliesTo(tc.flavour))
		})
	}
}

const setStepYAML = `
"Test set":
  - set: { _id: id }
`

func TestParseSetStep(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		yaml     string
		wantPath string
		wantName string
	}{
		{
			name:     "captures response field into variable",
			yaml:     setStepYAML,
			wantPath: fieldID,
			wantName: "id",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tf, err := compat.ParseTestFile(strings.NewReader(tt.yaml))
			require.NoError(t, err)
			require.Len(t, tf.Tests, 1)
			require.Len(t, tf.Tests[0].Steps, 1)

			step := tf.Tests[0].Steps[0]
			require.NotNil(t, step.Set)
			assert.Equal(t, tt.wantPath, step.Set.Path)
			assert.Equal(t, tt.wantName, step.Set.Name)
		})
	}
}

const skipStepYAML = `
"Test skip":
  - skip:
      features: allowed_warnings
`

func TestParseSkipStep(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		yaml         string
		wantFeatures any
	}{
		{
			name:         "records skip features",
			yaml:         skipStepYAML,
			wantFeatures: "allowed_warnings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tf, err := compat.ParseTestFile(strings.NewReader(tt.yaml))
			require.NoError(t, err)
			require.Len(t, tf.Tests, 1)
			require.Len(t, tf.Tests[0].Steps, 1)

			step := tf.Tests[0].Steps[0]
			require.NotNil(t, step.Skip)
			assert.Equal(t, tt.wantFeatures, step.Skip.Features)
		})
	}
}

const doCatchYAML = `
"Test catch":
  - do:
      catch: /illegal_argument_exception/
      esql.get_query:
        id: "foobar"
`

func TestParseDoCatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		yaml       string
		wantName   string
		wantCatch  string
		wantParams map[string]any
	}{
		{
			name:       "parses catch pattern alongside action params",
			yaml:       doCatchYAML,
			wantName:   "esql.get_query",
			wantCatch:  catchIllegalArg,
			wantParams: map[string]any{compat.ParamID: "foobar"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tf, err := compat.ParseTestFile(strings.NewReader(tt.yaml))
			require.NoError(t, err)
			require.Len(t, tf.Tests, 1)
			require.Len(t, tf.Tests[0].Steps, 1)

			step := tf.Tests[0].Steps[0]
			require.NotNil(t, step.Do)
			assert.Equal(t, tt.wantName, step.Do.Name)
			assert.Equal(t, tt.wantCatch, step.Do.Catch)
			assert.Equal(t, tt.wantParams, step.Do.Params)
		})
	}
}

const doHeadersYAML = `
"Test headers":
  - do:
      headers:
        Authorization: "Basic abc"
      cluster.health: {}
`

func TestParseDoHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		yaml     string
		wantName string
	}{
		{
			name:     "skips headers and keeps action",
			yaml:     doHeadersYAML,
			wantName: "cluster.health",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tf, err := compat.ParseTestFile(strings.NewReader(tt.yaml))
			require.NoError(t, err)
			require.Len(t, tf.Tests, 1)
			require.Len(t, tf.Tests[0].Steps, 1)

			step := tf.Tests[0].Steps[0]
			require.NotNil(t, step.Do)
			assert.Equal(t, tt.wantName, step.Do.Name)
			assert.Empty(t, step.Do.Params)
		})
	}
}

const (
	gtAssertionYAML = `
"Test gt":
  - gt:
      count: 0
`

	existsAssertionYAML = `
"Test exists":
  - exists: (local).version.number
`

	containsAssertionYAML = `
"Test contains":
  - contains:
      nodes.$master.modules:
        name: x-pack-ent-search
`
)

func TestParseNewAssertions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		yaml      string
		wantType  string
		wantValue any
	}{
		{
			name:      "gt",
			yaml:      gtAssertionYAML,
			wantType:  compat.AssertGT,
			wantValue: map[string]any{fieldCount: 0},
		},
		{
			name:      "exists with local prefix",
			yaml:      existsAssertionYAML,
			wantType:  compat.AssertExists,
			wantValue: "(local).version.number",
		},
		{
			name:      "contains nested module",
			yaml:      containsAssertionYAML,
			wantType:  compat.AssertContains,
			wantValue: map[string]any{"nodes.$master.modules": map[string]any{fieldName: entSearchModule}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tf, err := compat.ParseTestFile(strings.NewReader(tt.yaml))
			require.NoError(t, err)
			require.Len(t, tf.Tests, 1)
			require.Len(t, tf.Tests[0].Steps, 1)

			step := tf.Tests[0].Steps[0]
			require.NotNil(t, step.Assert)
			assert.Equal(t, tt.wantType, step.Assert.Type)
			assert.Equal(t, tt.wantValue, step.Assert.Value)
		})
	}
}

func TestAllTestdataParse(t *testing.T) {
	t.Parallel()

	var filePaths []string

	for _, pattern := range []string{"testdata/tests/*.yml", "testdata/tests/*/*.yml"} {
		matches, err := filepath.Glob(pattern)
		require.NoError(t, err)

		filePaths = append(filePaths, matches...)
	}

	require.NotEmpty(t, filePaths, "testdata corpus must be populated")

	var parseErrors []string

	for _, filePath := range filePaths {
		fileHandle, err := os.Open(filepath.Clean(filePath))
		require.NoError(t, err)

		_, parseErr := compat.ParseTestFile(fileHandle)
		require.NoError(t, fileHandle.Close())

		if parseErr != nil {
			parseErrors = append(parseErrors, fmt.Sprintf("%s: %v", filePath, parseErr))
		}
	}

	t.Logf("parsed %d/%d test files", len(filePaths)-len(parseErrors), len(filePaths))
	assert.Empty(t, parseErrors, "every test file must parse")
}
