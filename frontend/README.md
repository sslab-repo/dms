# Dataset Platform Frontend

This is the React frontend for the Dataset Platform — a web interface for researchers to upload, search, browse, and download datasets.

## What the frontend does

- **Browse** all ready datasets in a compact list layout on the home page.
- **Search** datasets with a hybrid keyword + semantic search engine, with metadata filters (modality, type, size, label completeness).
- **Authenticate** admin-created users before uploads and dataset mutations.
- **Upload** datasets via a multi-step wizard: fill in metadata, select files, chunked upload with progress, and poll for AI pipeline completion.
- **View and manage dataset details** including AI-generated summary, labels (Weka-style bar chart), profile samples, metadata caveats, download options, owner-only metadata editing, and owner/admin deletion.

## How it connects to the backend

All data comes from a Go backend at a configurable `VITE_API_URL` (default `http://localhost:8081`). The frontend calls REST endpoints for auth, datasets, file upload, and search.

## Directory map

- `src/api/` — typed fetch wrappers around every backend endpoint (see [`src/api/README.md`](src/api/README.md))
- `src/components/` — reusable UI components and route guards (see [`src/components/README.md`](src/components/README.md))
- `src/context/` — auth state and token lifecycle
- `src/pages/` — full-page views (Home, Upload, DatasetDetail) (see [`src/pages/README.md`](src/pages/README.md))
- `src/types/` — TypeScript interfaces mirroring backend JSON (see [`src/types/README.md`](src/types/README.md))
- `src/styles/` — CSS stylesheets (`index.css`, `Layout.css`)
- `src/utils.ts` — shared utility (`formatBytes`)

## Tech stack

- React 19 + TypeScript
- Vite for development and build
- React Router for client-side routing
- JWT stored in `localStorage` for authenticated mutations
- No UI library — custom CSS with an editorial/academic design (serif headings, warm neutral palette, green accent)

## Running locally

```bash
cd frontend
npm install
npm run dev
```

The dev server runs on port 5173 by default. Set `VITE_API_URL` to point to your backend.

## Upload tuning

Upload behavior is controlled by Vite build-time environment variables:

| Variable | Default | Purpose |
|---|---|---|
| `VITE_UPLOAD_CHUNK_SIZE_MB` | `10` | Chunk size in MB, clamped to the backend multipart limit |
| `VITE_UPLOAD_MAX_CONCURRENT_CHUNKS` | `4` | Non-final chunks uploaded in parallel per file |
| `VITE_UPLOAD_MAX_CONCURRENT_FILES` | `2` | Files uploaded in parallel per dataset |

The frontend uploads all non-final chunks concurrently, then sends the final chunk last so the backend can safely assemble the file.
