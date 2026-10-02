package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zulucmd/zulu/v2"

	"github.com/pg-es/pg-es-proxy/internal/config"
)

// configFlag is the name of the flag pointing at the config file.
const configFlag = "--config"

// newTestCommand builds a zulu command with the same flags as the real
// binary and parses the given args. Flag registration is duplicated here
// (main registers its flags inline per the plan); the tests exercise
// config.Load's behaviour given a parsed flag set, not main's flag set.
func newTestCommand(t *testing.T, args []string) *zulu.Command {
	t.Helper()

	cmd := &zulu.Command{
		Use:  "pg-es-proxy",
		RunE: func(_ *zulu.Command, _ []string) error { return nil },
	}

	flags := cmd.Flags()
	flags.String("config", "", "path to config file")
	flags.String("server-host", config.DefaultServerHost, "server host")
	flags.Int("server-port", config.DefaultServerPort, "server port")
	flags.String("db-host", config.DefaultDBHost, "database host and port")
	flags.String("db-user", "", "database user")
	flags.String("db-password", "", "database password")
	flags.String("db-name", "", "database name")
	flags.Int32("db-max-conns", config.DefaultDBMaxConns, "max connections")
	flags.Int32("db-min-conns", config.DefaultDBMinConns, "min connections")
	flags.String("log-level", config.DefaultLogLevel, "log level")

	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute())

	return cmd
}

func writeConfigFile(t *testing.T, name, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

func TestLoadDefaults(t *testing.T) {
	t.Parallel()

	cmd := newTestCommand(t, nil)

	cfg, err := config.Load(cmd)
	require.NoError(t, err)

	assert.Equal(t, "localhost", cfg.Server.Host)
	assert.Equal(t, 9200, cfg.Server.Port)
	assert.Equal(t, "localhost:5432", cfg.DB.Host)
	assert.Empty(t, cfg.DB.User)
	assert.Empty(t, cfg.DB.Password)
	assert.Empty(t, cfg.DB.Name)
	assert.Equal(t, int32(10), cfg.DB.MaxConns)
	assert.Equal(t, int32(2), cfg.DB.MinConns)
	assert.Equal(t, "info", cfg.Log.Level)
}

func TestLoadEnvOverride(t *testing.T) {
	t.Setenv("PGESPROXY_DB_HOST", "testhost")
	t.Setenv("PGESPROXY_SERVER_PORT", "1234")
	t.Setenv("PGESPROXY_DB_MAX_CONNS", "42")
	t.Setenv("PGESPROXY_LOG_LEVEL", "debug")

	cmd := newTestCommand(t, nil)

	cfg, err := config.Load(cmd)
	require.NoError(t, err)

	assert.Equal(t, "testhost", cfg.DB.Host)
	assert.Equal(t, 1234, cfg.Server.Port)
	assert.Equal(t, int32(42), cfg.DB.MaxConns)
	assert.Equal(t, "debug", cfg.Log.Level)

	// Untouched keys keep their defaults.
	assert.Equal(t, "localhost", cfg.Server.Host)
}

func TestLoadFlagOverridesEnv(t *testing.T) {
	t.Setenv("PGESPROXY_DB_HOST", "envhost")
	t.Setenv("PGESPROXY_SERVER_PORT", "1111")

	cmd := newTestCommand(t, []string{"--db-host=flaghost", "--server-port=2222"})

	cfg, err := config.Load(cmd)
	require.NoError(t, err)

	assert.Equal(t, "flaghost", cfg.DB.Host)
	assert.Equal(t, 2222, cfg.Server.Port)
}

func TestLoadConfigFileJSON(t *testing.T) {
	t.Parallel()

	path := writeConfigFile(t, "config.json", `{
		"server": {"host": "jsonhost", "port": 9999},
		"db": {"host": "jsondbhost", "name": "jsondb", "user": "jsonuser"},
		"log": {"level": "warn"}
	}`)

	cmd := newTestCommand(t, []string{configFlag, path})

	cfg, err := config.Load(cmd)
	require.NoError(t, err)

	assert.Equal(t, "jsonhost", cfg.Server.Host)
	assert.Equal(t, 9999, cfg.Server.Port)
	assert.Equal(t, "jsondbhost", cfg.DB.Host)
	assert.Equal(t, "jsondb", cfg.DB.Name)
	assert.Equal(t, "jsonuser", cfg.DB.User)
	assert.Equal(t, "warn", cfg.Log.Level)

	// Keys absent from the file keep their defaults.
	assert.Equal(t, int32(10), cfg.DB.MaxConns)
}

func TestLoadConfigFileYAML(t *testing.T) {
	t.Parallel()

	path := writeConfigFile(t, "config.yaml", `
server:
  host: yamlhost
  port: 8888
db:
  name: yamldb
  max_conns: 25
`)

	cmd := newTestCommand(t, []string{configFlag, path})

	cfg, err := config.Load(cmd)
	require.NoError(t, err)

	assert.Equal(t, "yamlhost", cfg.Server.Host)
	assert.Equal(t, 8888, cfg.Server.Port)
	assert.Equal(t, "yamldb", cfg.DB.Name)
	assert.Equal(t, int32(25), cfg.DB.MaxConns)
}

func TestLoadLegacyJSON(t *testing.T) {
	t.Parallel()

	path := writeConfigFile(t, "pg_elastic_config.json", `{
		"ServerPort": 9999,
		"PostgresConfig": {
			"ServerAddress": "legacyhost:5433",
			"User": "legacyuser",
			"Password": "legacypass",
			"DBName": "legacydb"
		}
	}`)

	cmd := newTestCommand(t, []string{configFlag, path})

	cfg, err := config.Load(cmd)
	require.NoError(t, err)

	assert.Equal(t, 9999, cfg.Server.Port)
	assert.Equal(t, "legacyhost:5433", cfg.DB.Host)
	assert.Equal(t, "legacyuser", cfg.DB.User)
	assert.Equal(t, "legacypass", cfg.DB.Password)
	assert.Equal(t, "legacydb", cfg.DB.Name)

	// Keys the legacy format cannot express keep their defaults.
	assert.Equal(t, "localhost", cfg.Server.Host)
	assert.Equal(t, int32(10), cfg.DB.MaxConns)
	assert.Equal(t, "info", cfg.Log.Level)
}

// TestFlagDefaultDoesNotOverrideConfigFile pins the kzflag !f.Changed
// behaviour: a flag the user did not set must not clobber a value coming
// from the config file, even though the flag itself has a default.
func TestFlagDefaultDoesNotOverrideConfigFile(t *testing.T) {
	t.Parallel()

	path := writeConfigFile(t, "config.json", `{"db": {"host": "filehost:6543"}}`)

	// No --db-host passed: its flag default must not win.
	cmd := newTestCommand(t, []string{"--config", path, "--db-name=flagdb"})

	cfg, err := config.Load(cmd)
	require.NoError(t, err)

	assert.Equal(t, "filehost:6543", cfg.DB.Host)
	assert.Equal(t, "flagdb", cfg.DB.Name)
}

func TestLoadEnvOverridesConfigFile(t *testing.T) {
	t.Setenv("PGESPROXY_DB_HOST", "envhost")

	path := writeConfigFile(t, "config.json", `{"db": {"host": "filehost"}}`)

	cmd := newTestCommand(t, []string{configFlag, path})

	cfg, err := config.Load(cmd)
	require.NoError(t, err)

	assert.Equal(t, "envhost", cfg.DB.Host)
}

func TestLoadMissingConfigFile(t *testing.T) {
	t.Parallel()

	cmd := newTestCommand(t, []string{configFlag, filepath.Join(t.TempDir(), "does-not-exist.json")})

	_, err := config.Load(cmd)
	require.Error(t, err)
}

func TestLoadInvalidConfigFile(t *testing.T) {
	t.Parallel()

	path := writeConfigFile(t, "config.json", `{not json`)

	cmd := newTestCommand(t, []string{configFlag, path})

	_, err := config.Load(cmd)
	require.Error(t, err)
}

func TestLoadUnsupportedConfigExtension(t *testing.T) {
	t.Parallel()

	path := writeConfigFile(t, "config.toml", `server = {}`)

	cmd := newTestCommand(t, []string{configFlag, path})

	_, err := config.Load(cmd)
	require.Error(t, err)
}
