package config

import (
	"os"
	"path/filepath"
	"testing"
)

func clearEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{"CONFIG_PATH", "SERVER_PORT", "MYSQL_HOST", "MYSQL_PORT", "MYSQL_USER", "MYSQL_PASSWORD", "MYSQL_ROOT_PASSWORD", "MYSQL_DATABASE", "REDIS_HOST", "REDIS_PORT", "REDIS_DB", "REDIS_PASSWORD", "REDIS_ENABLED", "RABBITMQ_HOST", "RABBITMQ_PORT", "RABBITMQ_USER", "RABBITMQ_PASS", "RABBITMQ_ENABLED", "PPROF_ENABLED"} {
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
}
func configFile(t *testing.T, text string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}
func TestLoadConfigAndEnvironmentPrecedence(t *testing.T) {
	clearEnv(t)
	p := configFile(t, "server:\n  port: 9090\ndatabase:\n  dbname: from_file\n")
	t.Setenv("MYSQL_DATABASE", "from_env")
	cfg, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 9090 || cfg.Database.DbName != "from_env" {
		t.Fatal("wrong config precedence")
	}
}
func TestBadConfigurationDoesNotSilentlyFallBack(t *testing.T) {
	clearEnv(t)
	for _, body := range []string{"server: [", "server:\n  prot: 8080\n", "server:\n  port: -1\n"} {
		if _, _, err := LoadLocalDev(configFile(t, body)); err == nil {
			t.Fatal("bad config accepted")
		}
	}
	t.Setenv("REDIS_PORT", "not-a-number")
	if _, err := LoadDefaultConfig(); err == nil {
		t.Fatal("invalid environment accepted")
	}
}
func TestMissingDevelopmentConfigAndExplicitPath(t *testing.T) {
	clearEnv(t)
	path := filepath.Join(t.TempDir(), "missing.yaml")
	if _, fallback, err := LoadLocalDev(path); err != nil || !fallback {
		t.Fatalf("expected local defaults: %v", err)
	}
	t.Setenv("CONFIG_PATH", path)
	if _, _, err := LoadLocalDev(path); err == nil {
		t.Fatal("missing explicit path accepted")
	}
}
