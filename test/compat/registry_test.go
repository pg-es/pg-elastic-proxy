package compat_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pg-es/pg-es-proxy/test/compat"
)

func TestExecuteSelectsFlavourRegistry(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	executor := compat.NewExecutor(srv.URL, srv.Client())

	unfreeze := &compat.DoAction{Name: "indices.unfreeze", Params: map[string]any{compat.ParamIndex: "i"}}
	esql := &compat.DoAction{Name: "esql.get_data_source", Params: map[string]any{}}

	_, err := executor.Execute(t.Context(), unfreeze, compat.FlavourES8)
	require.NoError(t, err, "indices.unfreeze is routed on es8")

	_, err = executor.Execute(t.Context(), unfreeze, compat.FlavourES9)
	require.ErrorIs(t, err, compat.ErrSkip, "indices.unfreeze is absent from the es9 spec")

	_, err = executor.Execute(t.Context(), esql, compat.FlavourES9)
	require.NoError(t, err, "esql.get_data_source is routed on es9")

	_, err = executor.Execute(t.Context(), esql, compat.FlavourES8)
	require.ErrorIs(t, err, compat.ErrSkip, "esql.get_data_source is absent from the es8 spec")

	_, err = executor.Execute(t.Context(), esql, "")
	require.NoError(t, err, "an empty flavour falls back to es9")
}
