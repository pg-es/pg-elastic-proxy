package utils

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
)

// PGElasticConfig represents structure of the pg-elastic configuration
type PGElasticConfig struct {
	ServerPort     int
	PostgresConfig PostgresConnectionConfig
}

// PostgresConnectionConfig represents structure of the pg-elastic Postgres connection configuration
type PostgresConnectionConfig struct {
	ServerAddress string
	User          string
	Password      string
	DBName        string
}

// ReadConfig reads a configuration from the file at path
func ReadConfig(path string) *PGElasticConfig {
	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		log.Fatal(err)
	}

	config := &PGElasticConfig{}
	decodeErr := json.NewDecoder(file).Decode(config)

	if closeErr := file.Close(); closeErr != nil {
		log.Fatal(closeErr)
	}
	if decodeErr != nil {
		log.Fatal(decodeErr)
	}
	return config
}
