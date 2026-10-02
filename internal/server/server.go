// Package server wires the HTTP router to the Elasticsearch-compatible
// API handlers and runs the HTTP server.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/pg-es/pg-es-proxy/db"
	"github.com/pg-es/pg-es-proxy/internal/config"
	"github.com/pg-es/pg-es-proxy/server"
)

const (
	readTimeout     = 10 * time.Second
	writeTimeout    = 30 * time.Second
	idleTimeout     = 60 * time.Second
	shutdownTimeout = 10 * time.Second
)

var errDBNotConnected = errors.New("database connection is not established")

// PGElasticServerProto describes a server runtime instance and its configuration.
type PGElasticServerProto struct {
	handler  http.Handler
	config   *config.Config
	dbclient *db.Client
}

// InitializeServer creates an instance of the server from an already loaded
// configuration and an initialized database client.
func InitializeServer(cfg *config.Config, dbclient *db.Client) (server.PGElasticServer, error) {
	if dbclient == nil {
		return nil, errDBNotConnected
	}

	srv := &PGElasticServerProto{
		config:   cfg,
		dbclient: dbclient,
	}
	srv.handler = ElasticProductMiddleware(NewRouter(srv))

	return srv, nil
}

// GetDBClient returnes a DB client of the server.
func (s *PGElasticServerProto) GetDBClient() *db.Client {
	return s.dbclient
}

// Start runs the HTTP server until ctx is cancelled (graceful shutdown) or
// the listener fails.
func (s *PGElasticServerProto) Start(ctx context.Context) error {
	addr := net.JoinHostPort(s.config.Server.Host, strconv.Itoa(s.config.Server.Port))

	httpServer := &http.Server{
		Addr:         addr,
		Handler:      s.handler,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	errCh := make(chan error, 1)

	go func() {
		slog.Info("starting server", slog.String("addr", addr))

		err := httpServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}

		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		slog.Info("shutting down server")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()

		err := httpServer.Shutdown(shutdownCtx)
		if err != nil {
			return fmt.Errorf("shutting down server: %w", err)
		}

		// Wait for ListenAndServe to return; ErrServerClosed maps to nil.
		return <-errCh
	case err := <-errCh:
		return err
	}
}
