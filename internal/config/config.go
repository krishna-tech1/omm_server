package config

import (
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Env         string
	Port        string
	DatabaseURL string
	JWTSecret   string
	TokenTTL    time.Duration
}

func Load() (Config, error) {
	godotenv.Load()

	cfg := Config{
		Env:         os.Getenv("ENV"),
		Port:        getEnv("PORT", "8080"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		JWTSecret:   os.Getenv("JWT_SECRET"),
	}

	ttlMinutes := getEnv("TOKEN_TTL_MINUTES", "1440")
	minutes, err := strconv.Atoi(ttlMinutes)
	if err != nil {
		return Config{}, err
	}
	cfg.TokenTTL = time.Duration(minutes) * time.Minute

	if cfg.DatabaseURL == "" || cfg.JWTSecret == "" {
		return Config{}, errors.New("missing required env vars")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
