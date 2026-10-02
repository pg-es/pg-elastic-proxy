package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"

	"github.com/pg-es/pg-es-proxy/db"
	"github.com/pg-es/pg-es-proxy/internal/pg"
	"gopkg.in/olivere/elastic.v5"
)

const (
	poolMaxConns = 10
	poolMinConns = 2
)

func migrateMapping(ctx context.Context, postgresqlClient *db.Client, indexName, typeName string, mapping map[string]any) {
	options, err := json.MarshalIndent(mapping, "", "  ")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Processing mapping for Index: %s, Type: %s\n", indexName, typeName)

	_, err = postgresqlClient.CreateType(ctx, indexName, typeName, string(options))
	if err != nil {
		log.Fatal(err)
	}
}

func migrateIndexMappings(ctx context.Context, elasticClient *elastic.Client, postgresqlClient *db.Client, indexName string) {
	mappings, err := elasticClient.GetMapping().Index(indexName).Do(ctx)
	if err != nil {
		log.Fatal(err)
	}

	indexData, ok := mappings[indexName].(map[string]any)
	if !ok {
		log.Fatalf("unexpected mapping format for index %s", indexName)
	}

	mappingsData, ok := indexData["mappings"].(map[string]any)
	if !ok {
		log.Fatalf("unexpected mappings format for index %s", indexName)
	}

	for key, mapping := range mappingsData {
		processedMapping := map[string]any{
			"mappings": map[string]any{
				key: mapping,
			},
		}

		migrateMapping(ctx, postgresqlClient, indexName, key, processedMapping)
	}
}

func migrateIndex(ctx context.Context, elasticClient *elastic.Client, postgresqlClient *db.Client, indexName string) {
	getResult, err := elasticClient.Search().Index(indexName).Do(ctx)
	if err != nil {
		log.Fatal(err)
	}

	for _, item := range getResult.Hits.Hits {
		body, err := json.MarshalIndent(item.Source, "", "\t")
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("Processing record Index: %s, Type: %s, ID: %s\n", item.Index, item.Type, item.Id)

		_, err = postgresqlClient.CreateDocument(ctx, item.Index, item.Type, string(body), item.Id)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func main() {
	postgresqlHost := flag.String("postgresql-host", "localhost:5432", "Host and port of destination PostgreSQL instance (host:port)")
	postgresqlUser := flag.String("postgresql-user", "postgres", "User to perform actions on destanation PostgreSQL instance")
	postgresqlPassword := flag.String("postgresql-password", "", "Password for specified user on destanation PostgreSQL instance")
	postgresqlDB := flag.String("postgresql-database", "postgres", "Name of destanation database inside target PostgreSQL instance")
	elasticsearchHost := flag.String("elasticsearch-host", "localhost:9200", "Host and port of source ElasticSearch instance")

	flag.Parse()

	ctx := context.Background()

	elasticURL := "http://" + *elasticsearchHost

	elasticClient, err := elastic.NewClient(elastic.SetURL(elasticURL))
	if err != nil {
		log.Fatal(err)
	}

	connString := pg.ConnString(*postgresqlHost, *postgresqlUser, *postgresqlPassword, *postgresqlDB)

	pool, err := pg.NewPool(ctx, connString, poolMaxConns, poolMinConns)
	if err != nil {
		log.Fatal(err)
	}

	postgresqlClient := db.NewClient(pool)

	err = postgresqlClient.InitializeSchema(ctx)
	if err != nil {
		log.Fatal(err)
	}

	indices, err := elasticClient.IndexNames()
	if err != nil {
		log.Fatal(err)
	}

	for _, indexName := range indices {
		migrateIndexMappings(ctx, elasticClient, postgresqlClient, indexName)
		migrateIndex(ctx, elasticClient, postgresqlClient, indexName)
	}
}
