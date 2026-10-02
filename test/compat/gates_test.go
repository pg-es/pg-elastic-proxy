package compat_test

import (
	"testing"

	"github.com/pg-es/pg-es-proxy/test/compat"
	"github.com/stretchr/testify/assert"
)

func TestSatisfiesRequires(t *testing.T) {
	t.Parallel()

	target := compat.Target{Flavour: testFlavour, Version: "9.0.0"}

	tests := []struct {
		name string
		req  compat.Requires
		want bool
	}{
		{name: "empty requires satisfied", req: compat.Requires{}, want: true},
		{name: "stack true", req: compat.Requires{Stack: new(true)}, want: true},
		{name: "stack false excludes stack", req: compat.Requires{Stack: new(false)}, want: false},
		{name: "cluster features unsupported", req: compat.Requires{ClusterFeatures: "feature_x"}, want: false},
		{name: "supported runner feature", req: compat.Requires{TestRunnerFeatures: "regex"}, want: true},
		{name: "unsupported runner feature", req: compat.Requires{TestRunnerFeatures: testNodeSelector}, want: false},
		{name: "capabilities unsupported", req: compat.Requires{Capabilities: []compat.Capability{{Path: "/_api"}}}, want: false},
		{name: "version in range", req: compat.Requires{Version: "8.0.0 - 9.5.0"}, want: true},
		{name: "version out of range", req: compat.Requires{Version: "8.0.0 - 8.5.0"}, want: false},
		{name: "os mismatch", req: compat.Requires{OS: "windows"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, target.SatisfiesRequires(&tt.req))
		})
	}
}

func TestShouldSkip(t *testing.T) {
	t.Parallel()

	target := compat.Target{Flavour: testFlavour, Version: "9.0.0"}

	tests := []struct {
		name string
		skip compat.SkipStep
		want bool
	}{
		{name: "empty skip", skip: compat.SkipStep{}, want: false},
		{name: "unsupported feature skips", skip: compat.SkipStep{Features: testNodeSelector}, want: true},
		{name: "supported feature does not skip", skip: compat.SkipStep{Features: "regex"}, want: false},
		{name: "awaits_fix skips", skip: compat.SkipStep{AwaitsFix: "https://example/1"}, want: true},
		{name: "version in range skips", skip: compat.SkipStep{Version: "8.0.0 - 9.5.0"}, want: true},
		{name: "version out of range does not skip", skip: compat.SkipStep{Version: "8.0.0 - 8.5.0"}, want: false},
		{name: "cluster features absent does not skip", skip: compat.SkipStep{ClusterFeatures: "feature_x"}, want: false},
		{name: "known issues do not skip", skip: compat.SkipStep{KnownIssues: []compat.KnownIssue{{ClusterFeature: "f"}}}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, target.ShouldSkip(&tt.skip))
		})
	}
}

func TestSectionRequiresSkips(t *testing.T) {
	t.Parallel()

	srv := newJSONServer(t, map[string]any{"ok": true})
	executor := compat.NewExecutor(srv.URL, srv.Client())

	file := compat.TestFile{
		Tests: []compat.TestSection{
			{
				Name: "needs an unsupported capability",
				Steps: []compat.Step{
					{Requires: &compat.Requires{TestRunnerFeatures: testNodeSelector}},
					{Do: &compat.DoAction{Name: compat.ActionGet, Params: map[string]any{compat.ParamIndex: "x", compat.ParamID: "1"}}},
				},
			},
		},
	}

	result := compat.RunFile(t.Context(), &file, executor, compat.Target{Flavour: testFlavour})

	assert.Equal(t, 1, result.Skipped)
	assert.Equal(t, 0, result.Passed)
}
