package config

import (
	"log"
	"os"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Port                 string
	JWTSecret            string
	JWTAccessExpiration  time.Duration
	JWTRefreshExpiration time.Duration
	GoogleClientID       string
	GoogleClientSecret   string
	FrontendURL          string
	MongoDBURI           string
	MongoDBDatabase      string

	// New fields for GA05
	LLMApiKey           string
	LLMProvider         string
	LLMModel            string // Configurable model for summarization
	SnoozeCheckInterval time.Duration
	KanbanColumns       []string

	// Week 4: Embedding/Semantic Search config
	EmbeddingProvider string // "openai" | "gemini" | "local"
	EmbeddingAPIKey   string
	EmbeddingModel    string
}

func Load() *Config {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	accessExp, _ := time.ParseDuration(getEnv("JWT_ACCESS_EXPIRATION", "15m"))
	refreshExp, _ := time.ParseDuration(getEnv("JWT_REFRESH_EXPIRATION", "168h"))

	// new values
	llmKey := getEnv("LLM_API_KEY", "")
	llmProvider := getEnv("LLM_PROVIDER", "")
	llmModel := getEnv("LLM_MODEL", "") // Empty defaults to internal default

	snoozeIntervalStr := getEnv("SNOOZE_CHECK_INTERVAL", "1m")
	snoozeInterval, err := time.ParseDuration(snoozeIntervalStr)
	if err != nil {
		snoozeInterval = time.Minute
	}
	kanbanColsRaw := getEnv("KANBAN_COLUMNS", "Inbox,To Do,In Progress,Done,Snoozed")
	cols := []string{}
	for _, p := range strings.Split(kanbanColsRaw, ",") {
		if t := strings.TrimSpace(p); t != "" {
			cols = append(cols, t)
		}
	}

	return &Config{
		Port:                 getEnv("PORT", "8080"),
		JWTSecret:            getEnv("JWT_SECRET", "your-secret-key-change-in-production"),
		JWTAccessExpiration:  accessExp,
		JWTRefreshExpiration: refreshExp,
		GoogleClientID:       getEnv("GOOGLE_CLIENT_ID", ""),
		GoogleClientSecret:   getEnv("GOOGLE_CLIENT_SECRET", ""),
		FrontendURL:          getEnv("FRONTEND_URL", "http://localhost:3000"),
		MongoDBURI:           getEnv("MONGODB_URI", ""),
		MongoDBDatabase:      getEnv("MONGODB_DATABASE", "aiemailbox"),

		LLMApiKey:           llmKey,
		LLMProvider:         llmProvider,
		LLMModel:            llmModel,
		SnoozeCheckInterval: snoozeInterval,
		KanbanColumns:       cols,

		// Week 4: Embedding config
		EmbeddingProvider: getEnv("EMBEDDING_PROVIDER", "openai"),
		EmbeddingAPIKey:   getEnv("EMBEDDING_API_KEY", ""),
		EmbeddingModel:    getEnv("EMBEDDING_MODEL", "text-embedding-ada-002"),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
