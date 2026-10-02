package pg_test

import (
	"testing"

	"github.com/pg-es/pg-es-proxy/internal/pg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateIdentifier(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "injection attempt", input: "foo; DROP TABLE bar", wantErr: true},
		{name: "hyphen accepted", input: "foo-bar", wantErr: false},
		{name: "empty string rejected", input: "", wantErr: true},
		{name: "leading digits accepted", input: "123abc", wantErr: false},
		{name: "underscore and digits accepted", input: "foo_bar123", wantErr: false},
		{name: "simple name accepted", input: "foo", wantErr: false},
		{name: "leading underscore accepted (type \"_doc\")", input: "_doc", wantErr: false},
		{name: "dot accepted", input: "foo.bar", wantErr: false},
		{name: "plus accepted", input: "foo+bar", wantErr: false},
		{name: "uppercase rejected (lowercase only)", input: "Foo", wantErr: true},
		{name: "bare dot rejected", input: ".", wantErr: true},
		{name: "double dot rejected", input: "..", wantErr: true},
		{name: "leading dash rejected", input: "-foo", wantErr: true},
		{name: "leading plus rejected", input: "+foo", wantErr: true},
		{name: "space rejected", input: "foo bar", wantErr: true},
		{name: "double quote rejected", input: `foo"bar`, wantErr: true},
		{name: "over 255 bytes rejected", input: "aabbcc", wantErr: len("aabbcc") > 255},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := pg.ValidateIdentifier(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			assert.NoError(t, err)
		})
	}
}

func TestValidateIndexName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{name: "hyphen accepted", input: "task-2", wantErr: false},
		{name: "dot accepted", input: "logs-2026.09", wantErr: false},
		{name: "plus accepted", input: "foo+bar", wantErr: false},
		{name: "leading digit accepted", input: "1task", wantErr: false},
		{name: "leading dot accepted (hidden index)", input: ".kibana", wantErr: false},
		{name: "leading underscore rejected (reserved)", input: "_all", wantErr: true},
		{name: "leading underscore type rejected as index", input: "_doc", wantErr: true},
		{name: "leading dash rejected", input: "-foo", wantErr: true},
		{name: "leading plus rejected", input: "+foo", wantErr: true},
		{name: "uppercase rejected (lowercase only)", input: "MyIndex", wantErr: true},
		{name: "injection attempt", input: "foo; DROP TABLE bar", wantErr: true},
		{name: "bare dot rejected", input: ".", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := pg.ValidateIndexName(tt.input)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			assert.NoError(t, err)
		})
	}
}
