package pg_test

import (
	"testing"

	"github.com/pg-es/pg-es-proxy/internal/pg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConnString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		hostPort string
		user     string
		pass     string
		db       string
		want     string
	}{
		{ //nolint:gosec // test fixture, not real credentials
			name:     "simple values",
			hostPort: "localhost:5432",
			user:     "user",
			pass:     "pass",
			db:       "db",
			want:     "postgres://user:pass@localhost:5432/db",
		},
		{ //nolint:gosec // test fixture, not real credentials
			name:     "password with special chars is percent-encoded",
			hostPort: "db.example.com:5433",
			user:     "admin",
			pass:     "p@ss/w rd",
			db:       "mydb",
			want:     "postgres://admin:p%40ss%2Fw%20rd@db.example.com:5433/mydb",
		},
		{
			name:     "empty password",
			hostPort: "localhost:5432",
			user:     "user",
			pass:     "",
			db:       "db",
			want:     "postgres://user:@localhost:5432/db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := pg.ConnString(tt.hostPort, tt.user, tt.pass, tt.db)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewPool(t *testing.T) {
	t.Parallel()

	t.Run("invalid conn string returns error", func(t *testing.T) {
		t.Parallel()
		_, err := pg.NewPool(t.Context(), "postgres://user:pass@host:notaport/db", 10, 2)
		require.Error(t, err)
	})

	t.Run("valid config creates pool", func(t *testing.T) {
		t.Parallel()

		if testing.Short() {
			t.Skip("skipping live PG connection in short mode")
		}

		pool, err := pg.NewPool(t.Context(), pg.ConnString("localhost:5432", "user", "pass", "db"), 10, 2)
		require.NoError(t, err)
		require.NotNil(t, pool)
		pool.Close()
	})
}
