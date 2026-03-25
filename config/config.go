package config

import (
	"os"
	"sync"

	"github.com/pelletier/go-toml/v2"
)

const defaultConfigPath = "config.toml"

// Config holds all global configurations.
// TODO: load in config - keep values in config.toml and avoid hardcoding.
type Config struct {
	DB    DBConfig    `toml:"db"`
	JWT   JWTConfig   `toml:"jwt"`
	S3    S3Config    `toml:"s3"`
	Redis RedisConfig `toml:"redis"`
}

type DBConfig struct {
	DSN string `toml:"dsn"` // TODO: load in config
}

type JWTConfig struct {
	Secret string `toml:"secret"` // TODO: load in config
}

type S3Config struct {
	Region       string `toml:"region"`         // TODO: load in config
	Endpoint     string `toml:"endpoint"`       // TODO: load in config
	Bucket       string `toml:"bucket"`         // TODO: load in config
	UsePathStyle bool   `toml:"use_path_style"` // TODO: load in config
}

type RedisConfig struct {
	Addr     string `toml:"addr"`     // TODO: load in config
	Password string `toml:"password"` // TODO: load in config
	DB       int    `toml:"db"`       // TODO: load in config
}

var (
	loadOnce sync.Once
	loaded   Config
	loadErr  error
)

func Load() (*Config, error) {
	loadOnce.Do(func() {
		loadErr = loadFromFile(defaultConfigPath, &loaded)
	})
	return &loaded, loadErr
}

func MustLoad() *Config {
	cfg, err := Load()
	if err != nil {
		panic(err)
	}
	return cfg
}

func Get() *Config {
	return MustLoad()
}

func loadFromFile(path string, out *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return toml.Unmarshal(data, out)
}
