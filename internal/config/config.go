package config

import (
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Env                    string
	Port                   string
	DatabaseURL            string
	JWTSecret              string
	TokenTTL               time.Duration
	TwilioAccountSID       string
	TwilioAuthToken        string
	TwilioVerifyServiceSID string
	R2AccessKeyID          string
	R2SecretAccessKey      string
	R2Endpoint             string
	R2Bucket               string
	R2PublicBaseURL        string
	R2Region               string
}

func Load() (Config, error) {
	godotenv.Load()

	cfg := Config{
		Env:                    os.Getenv("ENV"),
		Port:                   getEnv("PORT", "8080"),
		DatabaseURL:            os.Getenv("DATABASE_URL"),
		JWTSecret:              os.Getenv("JWT_SECRET"),
		TwilioAccountSID:       os.Getenv("TWILIO_ACCOUNT_SID"),
		TwilioAuthToken:        os.Getenv("TWILIO_AUTH_TOKEN"),
		TwilioVerifyServiceSID: os.Getenv("TWILIO_VERIFY_SERVICE_SID"),
		R2AccessKeyID:          os.Getenv("R2_ACCESS_KEY_ID"),
		R2SecretAccessKey:      os.Getenv("R2_SECRET_ACCESS_KEY"),
		R2Endpoint:             os.Getenv("R2_ENDPOINT"),
		R2Bucket:               os.Getenv("R2_BUCKET"),
		R2PublicBaseURL:        os.Getenv("R2_PUBLIC_BASE_URL"),
		R2Region:               getEnv("R2_REGION", "auto"),
	}

	ttlMinutes := getEnv("TOKEN_TTL_MINUTES", "43200")
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
