package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	AppPort               string
	BitrixWebhookBase    string
	BitrixSelfUserID     int64
	DBDSN                string
	PollIntervalSeconds  int
	DialogCooldownSeconds int
	AdminLogin           string
	AdminPassword        string
}

func Load() Config {
	_ = godotenv.Load()

	cfg := Config{
		AppPort:               getEnv("APP_PORT", "8080"),
		BitrixWebhookBase:     mustGetEnv("BITRIX_WEBHOOK_BASE"),
		BitrixSelfUserID:      getEnvInt64("BITRIX_SELF_USER_ID", 0),
		DBDSN:                 mustGetEnv("DB_DSN"),
		PollIntervalSeconds:   getEnvInt("POLL_INTERVAL_SECONDS", 5),
		DialogCooldownSeconds: getEnvInt("DIALOG_COOLDOWN_SECONDS", 30),
		AdminLogin:            getEnv("ADMIN_LOGIN", "admin"),
		AdminPassword:         getEnv("ADMIN_PASSWORD", "admin123"),
	}

	cfg.BitrixWebhookBase = strings.TrimRight(cfg.BitrixWebhookBase, "/") + "/"

	if cfg.BitrixSelfUserID == 0 {
		log.Fatal("BITRIX_SELF_USER_ID is required")
	}

	return cfg
}

func mustGetEnv(key string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		log.Fatalf("ENV %s is required", key)
	}
	return value
}

func getEnv(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getEnvInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}

	return n
}

func getEnvInt64(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	n, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}

	return n
}