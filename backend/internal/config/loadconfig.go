package config

import (
	"errors"
	"fmt"
	"gopkg.in/yaml.v2"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Server        ServerConfig        `yaml:"server"`
	Database      DataBaseConfig      `yaml:"database"`
	Redis         RedisConfig         `yaml:"redis"`
	RabbitMQ      RabbitMQConfig      `yaml:"rabbitmq"`
	Observability ObservabilityConfig `yaml:"observability"`
}
type ServerConfig struct {
	Port int `yaml:"port"`
}
type DataBaseConfig struct {
	Port     string `yaml:"port"`
	User     string `yaml:"user"`
	Host     string `yaml:"host"`
	Password string `yaml:"password"`
	DbName   string `yaml:"dbname"`
}
type RedisConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}
type RabbitMQConfig struct {
	Enabled  bool   `yaml:"enabled"`
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}
type ObservabilityConfig struct {
	Pprof PprofConfig `yaml:"pprof"`
}
type PprofConfig struct {
	Enabled    bool   `yaml:"enabled"`
	ApiAddr    string `yaml:"api_addr"`
	WorkerAddr string `yaml:"worker_addr"`
}

func defaults() *Config {
	return &Config{
		Server:        ServerConfig{Port: 8080},
		Database:      DataBaseConfig{Host: "localhost", Port: "3306", User: "root", Password: "root", DbName: "feedsystem"},
		Redis:         RedisConfig{Enabled: true, Host: "localhost", Port: 6379},
		RabbitMQ:      RabbitMQConfig{Enabled: true, Host: "localhost", Port: 5672, Username: "guest", Password: "guest"},
		Observability: ObservabilityConfig{Pprof: PprofConfig{ApiAddr: "localhost:6060", WorkerAddr: "localhost:6061"}},
	}
}
func Load(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg := defaults()
	if err := yaml.UnmarshalStrict(data, cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	if err := ApplyEnvOverrides(cfg); err != nil {
		return nil, err
	}
	return cfg, cfg.Validate()
}
func ApplyEnvOverrides(cfg *Config) error {
	if cfg == nil {
		return errors.New("config is nil")
	}
	for name, target := range map[string]*string{
		"MYSQL_HOST": &cfg.Database.Host, "MYSQL_PORT": &cfg.Database.Port, "MYSQL_USER": &cfg.Database.User,
		"MYSQL_ROOT_PASSWORD": &cfg.Database.Password, "MYSQL_DATABASE": &cfg.Database.DbName,
		"REDIS_HOST": &cfg.Redis.Host, "REDIS_PASSWORD": &cfg.Redis.Password,
		"RABBITMQ_HOST": &cfg.RabbitMQ.Host, "RABBITMQ_USER": &cfg.RabbitMQ.Username, "RABBITMQ_PASS": &cfg.RabbitMQ.Password,
	} {
		if value, ok := os.LookupEnv(name); ok {
			*target = value
		}
	}
	if value, ok := os.LookupEnv("MYSQL_PASSWORD"); ok {
		cfg.Database.Password = value
	}
	for name, target := range map[string]*int{"SERVER_PORT": &cfg.Server.Port, "REDIS_PORT": &cfg.Redis.Port, "REDIS_DB": &cfg.Redis.DB, "RABBITMQ_PORT": &cfg.RabbitMQ.Port} {
		if value, ok := os.LookupEnv(name); ok {
			n, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("%s must be an integer", name)
			}
			*target = n
		}
	}
	for name, target := range map[string]*bool{"REDIS_ENABLED": &cfg.Redis.Enabled, "RABBITMQ_ENABLED": &cfg.RabbitMQ.Enabled, "PPROF_ENABLED": &cfg.Observability.Pprof.Enabled} {
		if value, ok := os.LookupEnv(name); ok {
			b, err := strconv.ParseBool(value)
			if err != nil {
				return fmt.Errorf("%s must be a boolean", name)
			}
			*target = b
		}
	}
	return nil
}
func (cfg *Config) Validate() error {
	validPort := func(p int) bool { return p > 0 && p <= 65535 }
	if !validPort(cfg.Server.Port) {
		return errors.New("server.port must be between 1 and 65535")
	}
	p, err := strconv.Atoi(cfg.Database.Port)
	if err != nil || !validPort(p) {
		return errors.New("database.port must be between 1 and 65535")
	}
	if strings.TrimSpace(cfg.Database.Host) == "" || strings.TrimSpace(cfg.Database.User) == "" || strings.TrimSpace(cfg.Database.DbName) == "" {
		return errors.New("database host, user and dbname are required")
	}
	if cfg.Redis.Enabled && (cfg.Redis.Host == "" || !validPort(cfg.Redis.Port) || cfg.Redis.DB < 0) {
		return errors.New("invalid redis host, port or db")
	}
	if cfg.RabbitMQ.Enabled && (cfg.RabbitMQ.Host == "" || !validPort(cfg.RabbitMQ.Port) || cfg.RabbitMQ.Username == "") {
		return errors.New("invalid rabbitmq host, port or username")
	}
	return nil
}
func LoadDefaultConfig() (*Config, error) {
	cfg := defaults()
	if err := ApplyEnvOverrides(cfg); err != nil {
		return nil, err
	}
	return cfg, cfg.Validate()
}
func LoadLocalDev(filename string) (*Config, bool, error) {
	cfg, err := Load(filename)
	if err == nil {
		return cfg, false, nil
	}
	// Invalid files and explicitly configured paths must stop startup.
	if !errors.Is(err, os.ErrNotExist) || os.Getenv("CONFIG_PATH") != "" {
		return nil, false, err
	}
	cfg, err = LoadDefaultConfig()
	return cfg, true, err
}
