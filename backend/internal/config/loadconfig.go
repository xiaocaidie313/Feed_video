package config

import (
	"os"
	"strconv"

	"gopkg.in/yaml.v2"
)

// 总的config
type Config struct {
	Server   ServerConfig   `yaml :"serve"`
	Database DataBaseConfig `yaml :"database"`
	Redis    RedisConfig    `yaml :"redis"`
}

type ServerConfig struct {
	Port int `yaml :"port"`
}
type DataBaseConfig struct {
	Port     int    `yaml :"port"`
	User     string `yaml :"user"`
	Host     string `yaml :"host"`
	Password string `yaml :"password"`
	DBName   string `yaml :"db_name"`
}
type RedisConfig struct {
	Host     string `yaml :"host"`
	Port     int    `yaml :"port"`
	Password string `yaml :"password"`
	DB       int    `yaml :"db"`
}

// 传入config.yaml
func Load(filename string) (*Config, error) {
	// 加载环境变量
	data, err := os.ReadFile(filename)
	if err != nil {
		return &Config{}, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return &Config{}, err
	}
	ApplyEnvOverrides(&cfg)
	return &cfg, nil
}

// 应用环境变量的cfg
func ApplyEnvOverrides(cfg *Config) {
	if cfg == nil {
		return
	}
	if v := os.Getenv("SERVER_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = port
		}
	}
	if v := os.Getenv("MYSQL_HOST"); v != "" {
		cfg.Database.Host = v
	}
	if v := os.Getenv("MYSQL_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Database.Port = port
		}
	}
	if v := os.Getenv("MYSQL_USER"); v != "" {
		cfg.Database.User = v
	}
	if v := os.Getenv("MYSQL_ROOT_PASSWORD"); v != "" {
		cfg.Database.Password = v
	}
	if v := os.Getenv("MYSQL_PASSWORD"); v != "" {
		cfg.Database.Password = v
	}
	if v := os.Getenv("MYSQL_DATABASE"); v != "" {
		cfg.Database.DBName = v
	}
	if v := os.Getenv("REDIS_HOST"); v != "" {
		cfg.Redis.Host = v
	}
	if v := os.Getenv("REDIS_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Redis.Port = port
		}
	}
	if v := os.Getenv("REDIS_PASSWORD"); v != "" {
		cfg.Redis.Password = v
	}
	if v := os.Getenv("REDIS_DB"); v != "" {
		if db, err := strconv.Atoi(v); err == nil {
			cfg.Redis.DB = db
		}
	}
	if v := os.Getenv("RABBITMQ_HOST"); v != "" {
		cfg.RabbitMQ.Host = v
	}
	if v := os.Getenv("RABBITMQ_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.RabbitMQ.Port = port
		}
	}
	if v := os.Getenv("RABBITMQ_USER"); v != "" {
		cfg.RabbitMQ.Username = v
	}
	if v := os.Getenv("RABBITMQ_PASS"); v != "" {
		cfg.RabbitMQ.Password = v
	}
}

func LoadDefaultConfig() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port: 8080,
		},
		Database: DataBaseConfig{
			Port:     3306,
			User:     "root",
			Host:     "localhost",
			Password: "root",
			DBName:   "test",
		},
		Redis: RedisConfig{
			Host:     "localhost",
			Port:     6379,
			Password: "",
			DB:       0,
		},
	}
	ApplyEnvOverrides(cfg)
	return cfg, nil
}

// 判断是否用了default config
func LoadLocalDev(filename string) (*Config, bool, error) {
	cfg, err := Load(filename)
	if err == nil {
		return cfg, false, nil
	}
	cfg, err = LoadDefaultConfig()
	if err == nil {
		return cfg, true, nil
	}
	return &Config{}, false, err
}
