# cmd

This directory contains the entry point for the Dataset Platform backend.

## What is here

- `main.go` — the single startup file that brings the entire backend to life.

## What `main.go` does

When you run the application, `main.go` runs first and performs these steps in order:

1. **Loads configuration** — reads environment variables for port, database URL, AI API address, and file storage path (via [`config/`](../config/)).
2. **Connects to PostgreSQL** — opens a connection with a tuned pool (max 25 open, 5 idle) and verifies it with a ping.
3. **Ensures storage directory exists** — creates the file storage root and a `_tmp` subfolder if they are not already there.
4. **Initializes internal components** — wires together four core pieces:
   - `ai.Client` (talks to Flash + Ollama)
   - `filehandler.Handler` (manages uploads/downloads)
   - `search.Service` (runs hybrid search)
   - `api.Router` (exposes HTTP endpoints)
5. **Starts the HTTP server** — listens on the configured port with generous timeouts (15 min read, 30 min write) to support large file transfers.
6. **Graceful shutdown** — on `SIGINT` or `SIGTERM`, waits up to 30 seconds for in-flight requests to finish before exiting.

## Why this directory is simple

All real logic lives in `internal/` packages. `cmd/main.go` is only the startup sequence and wiring. If you ever need to change how components are initialized or how the server starts, this is the file to edit.
