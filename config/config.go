package config

import (
	"time"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
)

type Config struct {
	Port                 string        `envconfig:"SERVER_PORT" default:":8000"`
	DBSource             string        `envconfig:"DB_SOURCE" required:"true"`
	JWTSecretKey         string        `envconfig:"JWT_SECRET_KEY" required:"true"`
	JWTRefreshSecretKey  string        `envconfig:"JWT_REFRESH_SECRET_KEY" required:"true"`
	AccessTokenDuration  time.Duration `envconfig:"ACCESS_TOKEN_DURATION" default:"15m"`
	RefreshTokenDuration time.Duration `envconfig:"REFRESH_TOKEN_DURATION" default:"24h"`
}

func LoadConfig() (*Config, error) {
	_ = godotenv.Load()

	var cfg Config
	err := envconfig.Process("", &cfg)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}
