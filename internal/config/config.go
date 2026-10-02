// Package config loads pg-es-proxy configuration from defaults, an
// optional config file, environment variables, and command-line flags,
// in increasing order of precedence.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	koanfjson "github.com/knadh/koanf/parsers/json"
	koanfyaml "github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/confmap"
	"github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
	kzflag "github.com/zulucmd/koanf-zflag/v2"
	"github.com/zulucmd/zflag/v2"
	"github.com/zulucmd/zulu/v2"
)

// envPrefix is the prefix for environment variable configuration, e.g.
// PGESPROXY_DB_HOST maps to db.host.
const envPrefix = "PGESPROXY_"

// ErrUnsupportedConfigFormat is returned when the --config file has an
// extension other than .json, .yaml, or .yml.
var ErrUnsupportedConfigFormat = errors.New("unsupported config file format")

// Hardcoded configuration defaults, also used as the flag default values
// in cmd/pg-es-proxy.
const (
	DefaultServerHost = "localhost"
	DefaultServerPort = 9200
	DefaultDBHost     = "localhost:5432"
	DefaultDBMaxConns = int32(10)
	DefaultDBMinConns = int32(2)
	DefaultLogLevel   = "info"
)

// Config is the pg-es-proxy runtime configuration.
type Config struct {
	Server struct {
		Host string `koanf:"host"`
		Port int    `koanf:"port"`
	} `koanf:"server"`
	DB struct {
		Host     string `koanf:"host"`
		User     string `koanf:"user"`
		Password string `koanf:"password"`
		Name     string `koanf:"name"`
		MaxConns int32  `koanf:"max_conns"`
		MinConns int32  `koanf:"min_conns"`
	} `koanf:"db"`
	Log struct {
		Level string `koanf:"level"`
	} `koanf:"log"`
}

// defaultConfig returns the hardcoded configuration loaded first (lowest
// precedence). Database user, password, and name have no defaults: they
// must come from a config file, the environment, or flags.
func defaultConfig() map[string]any {
	return map[string]any{
		"server.host":  DefaultServerHost,
		"server.port":  DefaultServerPort,
		"db.host":      DefaultDBHost,
		"db.max_conns": DefaultDBMaxConns,
		"db.min_conns": DefaultDBMinConns,
		"log.level":    DefaultLogLevel,
	}
}

// Load builds the effective configuration for the parsed command. Sources
// are merged in increasing order of precedence: hardcoded defaults, the
// --config file (JSON or YAML; the legacy flat pg_elastic_config.json
// format is mapped onto the new keys), PGESPROXY_ environment variables,
// and finally the command-line flags that were explicitly set.
func Load(cmd *zulu.Command) (*Config, error) {
	ko := koanf.New(".")

	err := ko.Load(confmap.Provider(defaultConfig(), "."), nil)
	if err != nil {
		return nil, fmt.Errorf("loading default config: %w", err)
	}

	configFile, err := cmd.Flags().GetString("config")
	if err != nil {
		return nil, fmt.Errorf("reading --config flag: %w", err)
	}

	if configFile != "" {
		err = loadConfigFile(ko, configFile)
		if err != nil {
			return nil, err
		}
	}

	err = ko.Load(env.Provider(".", env.Opt{
		Prefix:        envPrefix,
		TransformFunc: envKeyTransform,
	}), nil)
	if err != nil {
		return nil, fmt.Errorf("loading environment config: %w", err)
	}

	err = ko.Load(kzflag.Provider(cmd.Flags(), ".",
		kzflag.WithKoanf(ko),
		kzflag.WithFlagCallback(flagKeyTransform),
	), nil)
	if err != nil {
		return nil, fmt.Errorf("loading flag config: %w", err)
	}

	cfg := &Config{}

	err = ko.Unmarshal("", cfg)
	if err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	return cfg, nil
}

// envKeyTransform maps PGESPROXY_DB_HOST to db.host. Only the first
// underscore separates the section from the key so that keys containing
// underscores (db.max_conns, db.min_conns) survive: PGESPROXY_DB_MAX_CONNS
// maps to db.max_conns.
func envKeyTransform(key, value string) (name string, val any) {
	name = strings.TrimPrefix(key, envPrefix)
	name = strings.ToLower(name)
	name = strings.Replace(name, "_", ".", 1)

	return name, value
}

// flagKeyTransform maps the dashed flag name db-max-conns to the koanf key
// db.max_conns. Only the first dash separates the section from the key.
// The transformed key is what kzflag's WithKoanf Changed-skip check tests
// with ko.Exists, so unchanged flags never override earlier sources.
func flagKeyTransform(f *zflag.Flag) (name string, val any) {
	return strings.Replace(f.Name, "-", ".", 1), kzflag.FlagVal(f)
}

// loadConfigFile loads the file at path into k. A JSON document with a
// top-level "PostgresConfig" key is treated as the legacy
// pg_elastic_config.json format; anything else is parsed as the new
// nested format, JSON or YAML by file extension.
func loadConfigFile(ko *koanf.Koanf, path string) error {
	data, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("reading config file: %w", err)
	}

	var probe map[string]any

	err = json.Unmarshal(data, &probe)
	if err == nil {
		if _, legacy := probe["PostgresConfig"]; legacy {
			return loadLegacyConfig(ko, probe)
		}
	}

	var parser koanf.Parser

	switch ext := strings.ToLower(filepath.Ext(path)); ext {
	case ".json":
		parser = koanfjson.Parser()
	case ".yaml", ".yml":
		parser = koanfyaml.Parser()
	default:
		return fmt.Errorf("%w: %q", ErrUnsupportedConfigFormat, ext)
	}

	err = ko.Load(file.Provider(path), parser)
	if err != nil {
		return fmt.Errorf("loading config file: %w", err)
	}

	return nil
}

// loadLegacyConfig maps the flat legacy pg_elastic_config.json keys onto
// the new nested keys and loads them into k.
func loadLegacyConfig(ko *koanf.Koanf, raw map[string]any) error {
	mapped := map[string]any{}

	if port, ok := raw["ServerPort"]; ok {
		mapped["server.port"] = port
	}

	pgConfig, ok := raw["PostgresConfig"].(map[string]any)
	if ok {
		err := mapLegacyPostgresConfig(mapped, pgConfig)
		if err != nil {
			return err
		}
	}

	err := ko.Load(confmap.Provider(mapped, "."), nil)
	if err != nil {
		return fmt.Errorf("loading legacy config: %w", err)
	}

	return nil
}

func mapLegacyPostgresConfig(mapped, pgConfig map[string]any) error {
	if addr, ok := pgConfig["ServerAddress"].(string); ok && addr != "" {
		mapped["db.host"] = addr
	}

	if user, ok := pgConfig["User"].(string); ok {
		mapped["db.user"] = user
	}

	if password, ok := pgConfig["Password"].(string); ok {
		mapped["db.password"] = password
	}

	if name, ok := pgConfig["DBName"].(string); ok {
		mapped["db.name"] = name
	}

	return nil
}
