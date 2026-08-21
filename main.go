package main

import (
	"github.com/pg-es/pg-es-proxy/internal/server"
	"log"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Llongfile)
	s, err := server.InitializeServer("pg_elastic_config.json")

	if err != nil {
		log.Fatal(err)
	}

	s.Start()
}
