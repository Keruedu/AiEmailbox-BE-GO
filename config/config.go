package config

import (
	"log"
	"os"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                  string
	JWTSecret             string
	JWTAccessExpiration   time.Duration
	JWTRefreshExpiration  time.Duration
	GoogleClientID        string
	GoogleClientSecret    string
	FrontendURL           string
	MongoDBURI            string
	MongoDBDatabase       string
}

func Load() *Config {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	accessExp, _ := time.ParseDuration(getEnv("JWT_ACCESS_EXPIRATION", "15m"))
	refreshExp, _ := time.ParseDuration(getEnv("JWT_REFRESH_EXPIRATION", "168h"))

	return &Config{
		Port:                  getEnv("PORT", "8080"),
		JWTSecret:             getEnv("JWT_SECRET", "your-secret-key-change-in-production"),
		JWTAccessExpiration:   accessExp,
		JWTRefreshExpiration:  refreshExp,
		GoogleClientID:        getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret:    getEnv("GOOGLE_CLIENT_SECRET", ""),
		FrontendURL:           getEnv("FRONTEND_URL", "http://localhost:3000"),
		MongoDBURI:            getEnv("MONGODB_URI", ""),
		MongoDBDatabase:       getEnv("MONGODB_DATABASE", "aiemailbox"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
