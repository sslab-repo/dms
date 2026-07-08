# config

This directory holds the single source of configuration for the entire backend.

## What is here

- `config.go` — reads settings from environment variables and returns a typed `Config` struct.

## What `config.go` does

- Loads a `.env` file if one exists (falls back to defaults otherwise).
- Reads each setting from an environment variable.
- Provides sensible defaults so the application can run locally without manual configuration.

## Config values

| Field | Environment Variable | Default | Purpose |
|---|---|---|---|
| `ServerPort` | `SERVER_PORT` | `8081` | Port the HTTP server listens on |
| `DatabaseURL` | `DATABASE_URL` | `postgres://labuser:labpass@localhost:5433/labdatasets?sslmode=disable` | PostgreSQL connection string |
| `AIBaseURL` | `AI_BASE_URL` | `http://james:8000` | University AI API (Flash) base URL |
| `AIModel` | `AI_MODEL` | `Flash` | Model name used for chat completions |
| `FileStorageDir` | `FILE_STORAGE_DIR` | `/home/rudra/dataset-platform/storage` | Where uploaded files are stored |
| `EmbeddingURL` | `EMBEDDING_BASE_URL` | `http://localhost:11434` | Ollama embedding service URL |
| `EmbeddingModel` | `EMBEDDING_MODEL` | `nomic-embed-text` | Model used to generate 768-dim vectors |
| `SemanticSimilarityThreshold` | `SEMANTIC_SIMILARITY_THRESHOLD` | `0.6` | Minimum cosine similarity for semantic search results |
| `JWTSecret` | `JWT_SECRET` | `dev-secret-change-me` | HMAC secret used to sign auth tokens |
| `JWTExpiryHours` | `JWT_EXPIRY_HOURS` | `24` | Auth token lifetime in hours |

## How to change settings

Set the corresponding environment variable before starting the server. If a variable is missing, the default above is used. No code changes are needed.
