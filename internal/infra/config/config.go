package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config holds all application configuration loaded from environment variables.
type Config struct {
	AppEnv     string
	AppPort    string
	DBHost     string
	DBPort     string
	DBUser     string
	DBPassword string
	DBName     string
	JWTSecret  string

	// LLM
	LLMProvider      string // "gemini" or "openrouter"
	GeminiAPIKey     string
	GeminiModel      string
	OpenRouterAPIKey string
	OpenRouterModel  string

	// WhatsApp
	WAPhoneNumber string // owner's WhatsApp JID (e.g. 628xxxx@s.whatsapp.net)

	// Redis (for asynq)
	RedisAddr     string
	RedisPassword string

	// App settings
	Timezone string // e.g. "Asia/Jakarta"
}

// LoadConfig reads .env file and returns the populated Config.
func LoadConfig() (*Config, error) {
	err := godotenv.Load()
	if err != nil {
		return nil, fmt.Errorf("error loading .env file: %w", err)
	}

	return &Config{
		AppEnv:     os.Getenv("APP_ENV"),
		AppPort:    os.Getenv("APP_PORT"),
		DBHost:     os.Getenv("DB_HOST"),
		DBPort:     os.Getenv("DB_PORT"),
		DBUser:     os.Getenv("DB_USER"),
		DBPassword: os.Getenv("DB_PASSWORD"),
		DBName:     os.Getenv("DB_NAME"),
		JWTSecret:  os.Getenv("JWT_SECRET"),

		LLMProvider:      getEnvOrDefault("LLM_PROVIDER", "gemini"),
		GeminiAPIKey:     os.Getenv("GEMINI_API_KEY"),
		GeminiModel:      getEnvOrDefault("GEMINI_MODEL", "gemini-1.5-flash"),
		OpenRouterAPIKey: os.Getenv("OPENROUTER_API_KEY"),
		OpenRouterModel:  getEnvOrDefault("OPENROUTER_MODEL", "openai/gpt-4o-mini"),

		WAPhoneNumber: os.Getenv("WA_PHONE_NUMBER"),

		RedisAddr:     getEnvOrDefault("REDIS_ADDR", "localhost:6379"),
		RedisPassword: os.Getenv("REDIS_PASSWORD"),

		Timezone: getEnvOrDefault("TIMEZONE", "Asia/Jakarta"),
	}, nil
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
