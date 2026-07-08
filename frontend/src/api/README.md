# src/api

This directory contains the **API client** — typed fetch wrappers that connect the frontend to the Go backend.

## What is here

- `client.ts` — all API calls, auth token helpers, and one function per backend endpoint.

## Endpoints covered

| Function | Backend Endpoint | Purpose |
|---|---|---|
| `healthCheck()` | `GET /api/health` | Liveness check |
| `login()` | `POST /api/auth/login` | Sign in and receive JWT |
| `getCurrentUser()` | `GET /api/auth/me` | Validate stored token and load current user |
| `listDatasets()` | `GET /api/datasets` | List all ready datasets |
| `createDataset(body)` | `POST /api/datasets` | Create a new dataset (auth required, pending) |
| `getDataset(id)` | `GET /api/datasets/:id` | Full dataset detail with labels, files, profile |
| `updateDataset(id, body)` | `PUT /api/datasets/:id` | Owner/admin metadata update |
| `deleteDataset(id)` | `DELETE /api/datasets/:id` | Owner/admin dataset delete |
| `registerFile(datasetId, name, mimeType)` | `POST /api/files/register` | Pre-create a file row before uploading chunks (auth required) |
| `uploadChunk(fileId, chunkIndex, totalChunks, data)` | `POST /api/files/chunk` | Upload one binary chunk (auth required) |
| `downloadUrl(datasetId)` | `GET /api/datasets/:id/download` | Generates the download URL |
| `search(query, filters)` | `GET /api/search` | Hybrid search with optional filters |

## How it works

- All calls use `fetch()` with the base URL from `import.meta.env.VITE_API_URL` (defaults to `http://localhost:8081`).
- `request<T>()` is a generic wrapper that sets JSON headers, checks for 2xx status, and returns typed JSON.
- Authenticated requests attach `Authorization: Bearer <token>` from `localStorage` when present.
- `uploadChunk()` uses `FormData` (multipart) for binary chunk uploads and attaches auth without setting a JSON content type.
- Upload chunk size and concurrency are read from `VITE_UPLOAD_*` variables in the upload page.
- Each response type mirrors the backend's JSON shape (defined in [`../types/`](../types/)).
- Normalizer functions (`normalizeDataset`, `normalizeSearchResult`, etc.) ensure null-safe defaults for optional fields.
