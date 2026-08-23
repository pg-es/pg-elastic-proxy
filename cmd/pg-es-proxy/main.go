package main

import (
	"log"

	"github.com/pg-es/pg-es-proxy/internal/server"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Llongfile)

	srv, err := server.InitializeServer("pg_elastic_config.json")
	if err != nil {
		log.Fatal(err)
	}

	srv.Start()
}
