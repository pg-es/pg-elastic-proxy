// pg-es-proxy serves an Elasticsearch-compatible REST API backed by
// PostgreSQL.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/zulucmd/zulu/v2"

	"github.com/pg-es/pg-es-proxy/db"
	"github.com/pg-es/pg-es-proxy/internal/config"
	"github.com/pg-es/pg-es-proxy/internal/pg"
	"github.com/pg-es/pg-es-proxy/internal/server"
)

func main() {
	os.Exit(realMain())
}

func realMain() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	rootCmd := newRootCommand()

	err := rootCmd.ExecuteContext(ctx)
	if err != nil {
		// zulu has already printed the error (and usage, for usage errors).
		return 1
	}

	return 0
}

// newRootCommand builds the root command with all configuration flags
// registered inline.
func newRootCommand() *zulu.Command {
	cmd := &zulu.Command{
		Use:   "pg-es-proxy",
		Short: "Elasticsearch-compatible REST API proxy backed by PostgreSQL",
		// Runtime failures must not dump usage; flag parse errors still do.
		SilenceUsage: true,
		RunE: func(cmd *zulu.Command, _ []string) error {
			return runServer(cmd)
		},
	}

	flags := cmd.Flags()
	flags.String("config", "", "path to a JSON or YAML config file")
	flags.String("server-host", config.DefaultServerHost, "host the HTTP server binds to")
	flags.Int("server-port", config.DefaultServerPort, "port the HTTP server listens on")
	flags.String("db-host", config.DefaultDBHost, "PostgreSQL host and port (host:port)")
	flags.String("db-user", "", "PostgreSQL user")
	flags.String("db-password", "", "PostgreSQL password")
	flags.String("db-name", "", "PostgreSQL database name")
	flags.Int32("db-max-conns", config.DefaultDBMaxConns, "maximum number of PostgreSQL pool connections")
	flags.Int32("db-min-conns", config.DefaultDBMinConns, "minimum number of idle PostgreSQL pool connections")
	flags.String("log-level", config.DefaultLogLevel, "log level (debug, info, warn, error)")

	return cmd
}

// runServer loads configuration, connects to PostgreSQL, and serves until
// the command context is cancelled (SIGINT/SIGTERM) or the server fails.
func runServer(cmd *zulu.Command) error {
	ctx := cmd.Context()

	cfg, err := config.Load(cmd)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	err = setupLogging(cfg.Log.Level)
	if err != nil {
		return err
	}

	pool, err := pg.NewPool(
		ctx,
		pg.ConnString(cfg.DB.Host, cfg.DB.User, cfg.DB.Password, cfg.DB.Name),
		cfg.DB.MaxConns,
		cfg.DB.MinConns,
	)
	if err != nil {
		return fmt.Errorf("creating connection pool: %w", err)
	}
	defer pool.Close()

	dbClient := db.NewClient(pool)

	err = dbClient.InitializeSchema(ctx)
	if err != nil {
		return fmt.Errorf("initializing schema: %w", err)
	}

	srv, err := server.InitializeServer(cfg, dbClient)
	if err != nil {
		return fmt.Errorf("initializing server: %w", err)
	}

	err = srv.Start(ctx)
	if err != nil {
		return fmt.Errorf("serving: %w", err)
	}

	return nil
}

// setupLogging installs the default slog logger at the given level.
func setupLogging(level string) error {
	var lvl slog.Level

	err := lvl.UnmarshalText([]byte(level))
	if err != nil {
		return fmt.Errorf("invalid log level %q: %w", level, err)
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl})))

	return nil
}
