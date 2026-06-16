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
	RedisUrl               string
	SessionStreamTTL       time.Duration
	MaxGPSPointAge         time.Duration
	MaxGPSPointFutureSkew  time.Duration
	MaxGPSSpeedMPH         float64
	MaxStepsPerMinute      float64
	MaxStrideLengthMeters  float64
	TwilioAccountSID       string
	TwilioAuthToken        string
	TwilioVerifyServiceSID string
	R2AccessKeyID          string
	R2SecretAccessKey      string
	R2Endpoint             string
	R2Bucket               string
	R2PublicBaseURL        string
	R2Region               string
	StripeSecretKey        string
	StripeWebhookSecret    string
}

func Load() (Config, error) {
	godotenv.Load()

	cfg := Config{
		Env:                    os.Getenv("ENV"),
		Port:                   getEnv("PORT", "8080"),
		DatabaseURL:            os.Getenv("DATABASE_URL"),
		JWTSecret:              os.Getenv("JWT_SECRET"),
		RedisUrl:               os.Getenv("REDIS_URL"),
		TwilioAccountSID:       os.Getenv("TWILIO_ACCOUNT_SID"),
		TwilioAuthToken:        os.Getenv("TWILIO_AUTH_TOKEN"),
		TwilioVerifyServiceSID: os.Getenv("TWILIO_VERIFY_SERVICE_SID"),
		R2AccessKeyID:          os.Getenv("R2_ACCESS_KEY_ID"),
		R2SecretAccessKey:      os.Getenv("R2_SECRET_ACCESS_KEY"),
		R2Endpoint:             os.Getenv("R2_ENDPOINT"),
		R2Bucket:               os.Getenv("R2_BUCKET"),
		R2PublicBaseURL:        os.Getenv("R2_PUBLIC_BASE_URL"),
		R2Region:               getEnv("R2_REGION", "auto"),
		StripeSecretKey:        os.Getenv("STRIPE_SECRET_KEY"),
		StripeWebhookSecret:    os.Getenv("STRIPE_WEBHOOK_SECRET"),
	}

	ttlMinutes := getEnv("TOKEN_TTL_MINUTES", "43200")
	minutes, err := strconv.Atoi(ttlMinutes)
	if err != nil {
		return Config{}, err
	}
	cfg.TokenTTL = time.Duration(minutes) * time.Minute

	sessionStreamTTLMinutes, err := strconv.Atoi(getEnv("SESSION_STREAM_TTL_MINUTES", "360"))
	if err != nil {
		return Config{}, err
	}
	cfg.SessionStreamTTL = time.Duration(sessionStreamTTLMinutes) * time.Minute

	maxGPSPointAgeSeconds, err := strconv.Atoi(getEnv("MAX_GPS_POINT_AGE_SECONDS", "300"))
	if err != nil {
		return Config{}, err
	}
	cfg.MaxGPSPointAge = time.Duration(maxGPSPointAgeSeconds) * time.Second

	maxGPSPointFutureSkewSeconds, err := strconv.Atoi(getEnv("MAX_GPS_POINT_FUTURE_SKEW_SECONDS", "30"))
	if err != nil {
		return Config{}, err
	}
	cfg.MaxGPSPointFutureSkew = time.Duration(maxGPSPointFutureSkewSeconds) * time.Second

	maxGPSSpeedMPH, err := strconv.ParseFloat(getEnv("MAX_GPS_SPEED_MPH", "15"), 64)
	if err != nil {
		return Config{}, err
	}
	cfg.MaxGPSSpeedMPH = maxGPSSpeedMPH

	maxStepsPerMinute, err := strconv.ParseFloat(getEnv("MAX_STEPS_PER_MINUTE", "240"), 64)
	if err != nil {
		return Config{}, err
	}
	cfg.MaxStepsPerMinute = maxStepsPerMinute

	maxStrideLengthMeters, err := strconv.ParseFloat(getEnv("MAX_STRIDE_LENGTH_METERS", "2.5"), 64)
	if err != nil {
		return Config{}, err
	}
	cfg.MaxStrideLengthMeters = maxStrideLengthMeters

	if cfg.DatabaseURL == "" || cfg.JWTSecret == "" || cfg.RedisUrl == "" {
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
