package server

import (
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/pg-es/pg-es-proxy/db"
	"github.com/pg-es/pg-es-proxy/server"
	"github.com/pg-es/pg-es-proxy/utils"
)

const (
	readTimeout  = 10 * time.Second
	writeTimeout = 30 * time.Second
	idleTimeout  = 60 * time.Second
)

var errDBNotConnected = errors.New("database connection is not established")

// PGElasticServerProto describes a server runtime instance and its configuration.
type PGElasticServerProto struct {
	handler  http.Handler
	config   *utils.PGElasticConfig
	dbclient *db.Client
}

// InitializeServer creates an instance of server. Configuration should be loaded from file configFileName.
func InitializeServer(configFileName string) (server.PGElasticServer, error) {
	srv := new(PGElasticServerProto)
	srv.config = utils.ReadConfig(configFileName)
	srv.dbclient = db.CreateClient(srv.config.PostgresConfig)

	if srv.dbclient == nil {
		return nil, errDBNotConnected
	}

	err := srv.dbclient.InitializeSchema()
	if err != nil {
		return nil, err
	}

	srv.handler = NewRouter(srv)

	return srv, nil
}

// GetConfiguration returnes a configuration of the server.
func (s *PGElasticServerProto) GetConfiguration() utils.PGElasticConfig {
	return *s.config
}

// GetDBClient returnes a DB client of the server.
func (s *PGElasticServerProto) GetDBClient() *db.Client {
	return s.dbclient
}

// Start the server.
func (s *PGElasticServerProto) Start() {
	log.Printf("starting server, listening on port %d\n", s.config.ServerPort)

	srv := &http.Server{
		Addr:         ":" + strconv.Itoa(s.config.ServerPort),
		Handler:      s.handler,
		ReadTimeout:  readTimeout,
		WriteTimeout: writeTimeout,
		IdleTimeout:  idleTimeout,
	}

	log.Fatal(srv.ListenAndServe())
}
