package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	AppEnv                     string
	AppSecret                  string
	HTTPAddr                   string
	DatabaseURL                string
	RedisAddr                  string
	RedisPassword              string
	RedisDB                    int
	JWTIssuer                  string
	JWTTTL                     time.Duration
	DefaultScanIntervalSeconds int
	MigrationsDir              string
	StaticDir                  string
	GitHubRepo                 string
	EnableSelfUpdate           bool
	UpdateCommand              string
}

func Load() Config {
	ttlHours := envInt("JWT_TTL_HOURS", 168)
	return Config{
		AppEnv:                     env("APP_ENV", "development"),
		AppSecret:                  env("APP_SECRET", "dev-secret-change-me-dev-secret"),
		HTTPAddr:                   env("HTTP_ADDR", ":8080"),
		DatabaseURL:                env("DATABASE_URL", "postgres://quota_guard:quota_guard@localhost:5432/quota_guard?sslmode=disable"),
		RedisAddr:                  env("REDIS_ADDR", "localhost:6379"),
		RedisPassword:              env("REDIS_PASSWORD", ""),
		RedisDB:                    envInt("REDIS_DB", 0),
		JWTIssuer:                  env("JWT_ISSUER", "api-monitor"),
		JWTTTL:                     time.Duration(ttlHours) * time.Hour,
		DefaultScanIntervalSeconds: envInt("DEFAULT_SCAN_INTERVAL_SECONDS", 60),
		MigrationsDir:              env("MIGRATIONS_DIR", "migrations"),
		StaticDir:                  env("STATIC_DIR", "web/dist"),
		GitHubRepo:                 env("GITHUB_REPO", "baogutang/api-monitor"),
		EnableSelfUpdate:           envBool("ENABLE_SELF_UPDATE", false),
		UpdateCommand:              env("UPDATE_COMMAND", ""),
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
