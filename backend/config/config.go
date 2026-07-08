// config/config.go

/*
This file is the single source for all configuration in the platform

Instead of hardcoding values like port numbers, database addresses, or API URLs, everything will live here

When something needs to change this is the only file that needs to be touched

Configuration is read from environment variables so we are never writing addresses, or passwords directly into the code
*/

package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {

	// ServerPort is the port the HTTP server listens on
	// The React frontend will send all its requests to this port
	ServerPort string

	// DatabaseURL is the connection string for the database
	// Format: postgres://user:password@host:port/dbname
	DatabaseURL string

	// AIBaseURL is the base URL of the university AI API running on James
	// All requests to Flash go through this address
	AIBaseURL string

	// AIModel is the name of the model we are calling on the AI API
	AIModel string

	// AIAPIKey is the Bearer token for the chat/analysis API (empty = no auth, e.g. internal james)
	AIAPIKey string

	// FileStorageDir is the directory on John's filesystem where uploaded dataset files will be stored. This directory must exist and be writable
	FileStorageDir string

	// EmbeddingURL is the base URL of the OpenAI-compatible embedding API (include /v1)
	EmbeddingURL string

	// EmbeddingModel is the model name passed to the embedding API
	EmbeddingModel string

	// EmbeddingAPIKey is the Bearer token for the embedding API (leave empty if the API requires no auth)
	EmbeddingAPIKey string

	// EmbeddingDimensions is the expected vector length returned by the embedding model.
	// GenerateEmbedding returns an error if the actual length doesn't match, catching model mismatches early.
	EmbeddingDimensions int

	// SemanticSimilarityThreshold is the minimum cosine similarity a semantic result must meet.
	SemanticSimilarityThreshold float64

	// JWTSecret signs local auth tokens.
	JWTSecret string

	// JWTExpiryHours controls how long auth tokens remain valid.
	JWTExpiryHours int
}

/*
Load reads configuration from environment variables and returns a Config struct.

If an environment variable is not set, a sensible default is used so the application can run locally without needing every variable defined.

In production on John, the real values will be set as environment variables.
*/
func Load() *Config {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, using default values")
	}
	return &Config{
		// Default port 8081 for the HTTP server (8080 is used by GitLab)
		ServerPort: getEnv("SERVER_PORT", "8081"),

		// Default points to our personal postgres instance on port 5433
		DatabaseURL: getEnv("DATABASE_URL", "postgres://labuser:labpass@localhost:5433/labdatasets?sslmode=disable"),

		// James is the AI API server
		AIBaseURL: getEnv("AI_BASE_URL", "http://james:8000"),

		AIModel:  getEnv("AI_MODEL", "Hopper2/Flash"),
		AIAPIKey: getEnv("AI_API_KEY", ""),

		// Default storage directory relative to the working directory
		FileStorageDir: getEnv("FILE_STORAGE_DIR", "/home/rudra/dataset-platform/storage"),

		EmbeddingURL: getEnv("EMBEDDING_BASE_URL", "https://hopper.cs.lewisu.edu:8443/v1"),

		EmbeddingModel: getEnv("EMBEDDING_MODEL", "Hopper2/Flash"),

		EmbeddingAPIKey:     getEnv("EMBEDDING_API_KEY", ""),
		EmbeddingDimensions: getEnvInt("EMBEDDING_DIMENSIONS", 1024),

		SemanticSimilarityThreshold: getEnvFloat("SEMANTIC_SIMILARITY_THRESHOLD", 0.6),

		JWTSecret: getEnv("JWT_SECRET", "dev-secret-change-me"),

		JWTExpiryHours: getEnvInt("JWT_EXPIRY_HOURS", 24),
	}
}

// WarnInsecureDefaults logs a warning if the configuration contains values
// that are only safe in development but dangerous in production.
func (c *Config) WarnInsecureDefaults() {
	if c.JWTSecret == "dev-secret-change-me" {
		log.Println("WARNING: JWT_SECRET is using the insecure default value. Set JWT_SECRET in .env before deploying.")
	}
}

// getEnv is a helper function that reads an environment variable by key
// If the variable is not set, it returns the provided default value
func getEnv(key string, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func getEnvFloat(key string, defaultValue float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		log.Printf("Invalid %s=%q, using default %.2f", key, value, defaultValue)
		return defaultValue
	}
	return parsed
}

func getEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		log.Printf("Invalid %s=%q, using default %d", key, value, defaultValue)
		return defaultValue
	}
	return parsed
}
