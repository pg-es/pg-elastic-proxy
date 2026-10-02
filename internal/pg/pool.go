package pg

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	maxConnLifetime   = 1 * time.Hour
	maxConnIdleTime   = 30 * time.Minute
	healthCheckPeriod = 1 * time.Minute
)

// NewPool parses connString, applies pool tuning defaults, and creates
// a *pgxpool.Pool. maxConns is the maximum pool size, minConns the
// minimum number of idle connections kept warm.
func NewPool(ctx context.Context, connString string, maxConns, minConns int32) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(connString)
	if err != nil {
		return nil, fmt.Errorf("parsing pool config: %w", err)
	}

	cfg.MaxConns = maxConns
	cfg.MinConns = minConns
	cfg.MaxConnLifetime = maxConnLifetime
	cfg.MaxConnIdleTime = maxConnIdleTime
	cfg.HealthCheckPeriod = healthCheckPeriod

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("creating pool: %w", err)
	}

	return pool, nil
}
