package config

// 总的config
type Config struct {
	Server   ServerConfig   `yaml :"serve"`
	DataBase DataBaseConfig `yaml :"database"`
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
