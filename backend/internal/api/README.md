# internal/api

This directory contains the **API layer** — the HTTP router and route handlers that sit between the React frontend and the rest of the backend.

## What the API layer does

- Receives all incoming HTTP requests from the frontend.
- Sets CORS headers so the frontend (running on a different port) can communicate freely.
- Routes each request to the correct handler.
- Validates request bodies and returns properly formatted JSON responses.
- Delegates actual work to the file handler, AI client, search service, or pipeline service.

## Files

### Router

| File | Purpose |
|---|---|
| `router.go` | Defines `Router`, wires all routes to their handlers, and sets up the `OnFileAssembled` callback so the AI pipeline runs once all files for a dataset are complete. Also handles CORS headers. |

### Handlers

| File | Purpose |
|---|---|
| `handlers/health.go` | `GET /api/health` — returns a simple liveness check (`{"status":"ok"}`). |
| `handlers/auth.go` | `POST /api/auth/login` and `GET /api/auth/me` for local JWT auth. |
| `handlers/admin.go` | Admin-only user creation, listing, and deletion. |
| `handlers/datasets.go` | Dataset list/detail, auth-protected create/update/delete, and download delegation. |
| `handlers/search.go` | `GET /api/search` — parses query and filter parameters, calls the search service, and returns results with citation tags. |
| `handlers/helpers.go` | Shared `writeJSON` helper used by all handlers for consistent JSON responses. |

## Registered endpoints

| Method | Path | Handler | Purpose |
|---|---|---|---|
| `GET` | `/api/health` | `healthHandler` | Liveness check |
| `POST` | `/api/auth/login` | `authHandler` | Login and return JWT |
| `GET` | `/api/auth/me` | `authHandler` | Return current user |
| `GET/POST` | `/api/admin/users` | `adminHandler` | List/create users (admin only) |
| `DELETE` | `/api/admin/users/:id` | `adminHandler` | Delete user (admin only) |
| `POST` | `/api/datasets` | `datasetHandler` | Create a new dataset (auth required, pending) |
| `GET` | `/api/datasets` | `datasetHandler` | List ready datasets (max 100) |
| `GET/PUT/DELETE` | `/api/datasets/:id` | `datasetHandler` | Detail, metadata edit, or delete |
| `GET` | `/api/datasets/:id/download` | `datasetHandler` | Stream dataset files |
| `POST` | `/api/files/register` | `fileHandler` | Pre-create a file row for chunked upload (auth required) |
| `POST` | `/api/files/chunk` | `fileHandler` | Upload one chunk of a file (auth required) |
| `GET` | `/api/search` | `searchHandler` | Hybrid search with filters and citations |

## Upload flow

1. `POST /api/datasets` → requires login and returns `dataset_id`
2. `POST /api/files/register` (once per file) → returns `file_id`
3. `POST /api/files/chunk` (repeated) → uploads chunks
4. When all files are assembled, `OnFileAssembled` fires → AI pipeline runs
5. `GET /api/datasets/:id` → returns full metadata including AI summary
6. `GET /api/datasets/:id/download` → streams the file(s)

Only mutating routes are auth-protected in v1. Public browse, detail, search, and download routes intentionally remain available without a token.

## Design note

The router is the **only** place in the backend that knows about HTTP. Every other component works with plain Go types and is agnostic to how it is called.
