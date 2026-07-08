# Dataset Platform Backend Manual

Welcome to the backend service for the Dataset Platform. This guide explains what the backend does, how the services fit together, where uploads are stored, and how the core flows work. It’s written for someone new to the project and focuses only on the `backend/` directory.

## What the backend does

The backend is a Go service that:
- Manages dataset metadata in PostgreSQL
- Handles multi‑file, chunked uploads and streaming downloads
- Authenticates admin-created users and enforces dataset ownership for mutations
- Runs an AI enrichment pipeline after all files are uploaded
- Provides a hybrid search API with filters and citations
- Exposes a stable REST API consumed by the React frontend

## Core services and responsibilities

### API Router (`internal/api/router.go`)
- Defines all HTTP routes and handlers
- Sets CORS headers for the frontend
- Wires together the file handler, AI client, search service, and database
- Applies auth middleware only to mutating routes; browse/search/download remain public

### Auth (`internal/auth/`)
- Hashes passwords with bcrypt
- Signs and verifies JWTs
- Stores authenticated user claims on request context

### File Handler (`internal/filehandler/`)
- Owns all file I/O for uploads and downloads
- Supports large files via chunked uploads
- Assembles chunks into final files on disk
- Triggers the AI pipeline once *all files* for a dataset are assembled

### AI Client (`internal/ai/client.go`)
- Talks to the AI API (Flash) for dataset analysis
- Generates summary, modality, dataset type, annotation format, and label completeness
- Produces suggested labels and pseudo‑queries
- Creates semantic embeddings (via Ollama) for search

### Profiler (`internal/profiler/`)
- Builds a bounded, deterministic profile from uploaded files before AI analysis
- Samples inspectable formats such as CSV, TSV, JSON, JSONL, and text
- Groups multi-file uploads by type, role, and shared schema
- Stores `profile_json` so the frontend and AI prompt use the same evidence

### Search Service (`internal/search/service.go`)
- Implements keyword + semantic search fusion
- Keyword search uses the materialized `datasets.search_vector` GIN index
- Semantic search uses stored `datasets.embedding_json` vectors and the configured cosine similarity threshold
- Supports filters (modality, dataset type, annotation format, size, label completeness)
- Returns search results with citation metadata

### Configuration (`config/config.go`)
- Reads environment variables for ports, DB URL, AI endpoints, storage paths
- Provides sensible defaults for local/dev

## API endpoints (backend)

These are implemented in `internal/api/router.go`:

- `GET /api/health` — liveness check
- `POST /api/auth/login` — username/password login
- `GET /api/auth/me` — current authenticated user
- `GET /api/admin/users` — list users (admin only)
- `POST /api/admin/users` — create user (admin only)
- `DELETE /api/admin/users/:id` — delete user and set their datasets ownerless (admin only)
- `GET /api/datasets` — list ready datasets (paginated to 100)
- `POST /api/datasets` — create dataset (auth required, pending status)
- `GET /api/datasets/:id` — full dataset detail
- `PUT /api/datasets/:id` — update dataset text metadata (owner/admin only; no AI rerun)
- `DELETE /api/datasets/:id` — delete dataset rows and physical files (owner/admin only)
- `GET /api/datasets/:id/download` — stream file download
- `POST /api/files/register` — register file and return file_id (auth required)
- `POST /api/files/chunk` — upload a file chunk (auth required)
- `GET /api/search` — hybrid search with filters

## Upload flow (chunked files)

Uploads are handled by `internal/filehandler/handler.go`:

1. **Create dataset** (`POST /api/datasets`) → requires login and returns `dataset_id` in `pending` state.
2. **Register each file** (`POST /api/files/register`) → creates a row in `files` and returns `file_id`.
3. **Upload chunks** (`POST /api/files/chunk`) → each chunk is written to disk immediately.
4. **Assemble file** → when the final chunk arrives, the file is assembled in the storage directory.
5. **Check all files complete** → once all files are marked complete, the AI pipeline runs once.

## AI pipeline (runs once per dataset)

Implemented in `internal/services/pipeline_service.go`:

1. Collect all assembled file names + total size
2. Mark dataset status as `processing`
3. Profile files and store `profile_json`
4. Read dataset metadata (name, researcher, description)
5. Call Flash with the dataset profile for analysis
6. Insert labels (non‑fatal on per‑label failure)
7. Insert pseudo‑queries (non‑fatal per query)
8. Update the materialized keyword `search_vector`
9. Generate and store a 768‑dim embedding of profile-enriched search text
10. Update dataset row with AI results and mark status `ready`

If any fatal step fails, the dataset is marked `error` with a human‑readable message.

Metadata edits update `name`, `researcher_name`, `description`, and `tags`, then rebuild keyword search. They do not rerun profiling, AI summary, labels, pseudo-queries, or embeddings. File-level add/delete/replace is intentionally out of scope for v1 and would need a full pipeline rerun.

## Where uploads are stored

Uploads are stored on disk in the directory defined by:

- `FILE_STORAGE_DIR` (default: `/home/rudra/dataset-platform/storage`)

Within this directory:

- Final assembled files live directly under the storage root
- In‑progress chunk uploads are stored in a `_tmp/` subfolder

The file handler ensures both folders exist at startup.

## Database

PostgreSQL schema is defined in:

- `migrations/001_init.sql`
- `migrations/002_files_storagepath_nullable.sql`

Key tables:
- `datasets` — dataset metadata + AI results
- `files` — uploaded file info, storage paths, and status
- `labels` — AI‑generated labels
- `pseudo_queries` — AI‑generated search queries

## Search behavior

Search is hybrid:
- Keyword search over a stored PostgreSQL `tsvector` that includes dataset metadata, labels, and pseudo-queries.
- Semantic search over stored Ollama embeddings in `embedding_json`.
- Results are fused and returned with citation tags

Filters are parsed from query parameters in the search handler and passed to the search service.

## Operational notes

- The server uses a **15‑minute read timeout** and **30‑minute write timeout** to handle large uploads.
- The backend reads only bounded samples for profiling; raw full datasets are never sent to AI, so file size does not scale token usage.
- Downloads are streamed with `io.Copy` for constant memory usage.

## Directory map (backend)

- `cmd/` — entry point for the service (see [`cmd/README.md`](cmd/README.md))
- `config/` — configuration loader (see [`config/README.md`](config/README.md))
- `internal/api/` — HTTP routing + handlers (see [`internal/api/README.md`](internal/api/README.md))
- `internal/ai/` — Flash + embedding client (see [`internal/ai/README.md`](internal/ai/README.md))
- `internal/filehandler/` — upload/download pipeline (see [`internal/filehandler/README.md`](internal/filehandler/README.md))
- `internal/profiler/` — bounded file profiler (see [`internal/profiler/README.md`](internal/profiler/README.md))
- `internal/search/` — search and scoring logic (see [`internal/search/README.md`](internal/search/README.md))
- `internal/services/` — domain orchestration services (see [`internal/services/README.md`](internal/services/README.md))
- `migrations/` — PostgreSQL schema migrations (see [`migrations/README.md`](migrations/README.md))

## Troubleshooting quick tips

- **CORS issues**: ensure the backend is running and the frontend points to the correct port.
- **Upload failures**: check storage permissions and available disk space.
- **AI errors**: check `AI_BASE_URL` and `EMBEDDING_BASE_URL` reachability.
- **Search oddities**: verify embeddings exist for datasets in the DB.
