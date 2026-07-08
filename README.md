# Dataset Platform

A centralized dataset management platform for a university research lab. Researchers can upload, search, browse, and download datasets of any kind — training data, annotated corpora, image sets, model checkpoints, and more.

## Why this exists

Datasets across the lab were scattered across different machines, inconsistently documented, and difficult to find or reuse across projects. This platform provides a single place where all datasets live, with AI-generated metadata (summaries, labels, class distributions) created automatically at upload time. Search is hybrid — keyword and semantic retrieval run in parallel and results are fused, with each result tagged to show how it was found.

---

## Quick start (on John)

The easiest way to start everything is the launch script at the project root:

```bash
cd /home/rudra/dataset-platform
bash start.sh
```

This starts PostgreSQL (if not already running), the Go backend, and the React frontend in the correct order. Once running, open `http://192.168.121.58:5173` in your browser from anywhere on the lab network.

To stop everything: CTRL + C

### Starting components manually

If you prefer to start each component yourself:

**1. Database** (skip if already running)
```bash
/opt/gitlab/embedded/bin/pg_ctl -D /home/rudra/pgdata -l /home/rudra/pgdata/postgres.log start
```

**2. Backend**
```bash
cd /home/rudra/dataset-platform/backend
./main
```

**3. Frontend**
```bash
cd /home/rudra/dataset-platform/frontend
npm run dev
```

> **Note:** `npm run dev` must be run from the `frontend/` directory, not the project root.

---

## Accessing the application

The application is accessible on the lab network at `http://192.168.121.58:5173`. No SSH or port forwarding needed.

If you are developing remotely over SSH, use VS Code port forwarding (Ports tab → Forward a Port → add `5173` and `8081`) and open `http://localhost:5173` in your browser.

---

## Architecture

```
Frontend (React)  ──HTTP──>  Backend (Go)  ──>  PostgreSQL (port 5433)
                                  │
                                  ├──>  Flash on James:8000  (AI analysis)
                                  └──>  Ollama on localhost:11434  (embeddings)
```

| Layer | Technology | Purpose |
|---|---|---|
| Frontend | React + TypeScript + Vite | Browse, search, upload, download |
| Backend | Go | API routing, AI pipeline, file handling, hybrid search |
| Database | PostgreSQL 16 | Dataset metadata, labels, search vectors, embeddings |
| AI analysis | Flash (James:8000) | Summaries, labels, class distributions, pseudo-queries |
| Embeddings | Ollama + nomic-embed-text | 768-dim semantic vectors for semantic search |
| File storage | Local disk on John | `/home/rudra/dataset-platform/storage` |

---

## Directory structure

```
dataset-platform/
  backend/                  Go backend
    cmd/                    Entry point (main.go)
    config/                 Environment-based configuration
    internal/
      api/                  HTTP router and handlers
      auth/                 JWT, bcrypt, and auth middleware
      ai/                   Flash chat + Ollama embedding client
      filehandler/          Chunked uploads and streaming downloads
      profiler/             Bounded file profiling
      search/               Hybrid keyword + semantic search with RRF fusion
      services/             AI pipeline and dataset service
    migrations/             PostgreSQL schema migrations
    scripts/                Utility scripts (admin seeding)

  frontend/                 React + TypeScript frontend
    src/
      api/                  Typed fetch wrappers for all backend endpoints
      components/           Shared UI components
      context/              AuthContext for login state
      pages/                Home, Upload, DatasetDetail, Login
      types/                TypeScript interfaces

  storage/                  Uploaded dataset files
  start.sh                  Launch script
  stop.sh                   Stop script
  INSTALL.md                Full installation and handoff guide
  CONTEXT.md                Full technical context and design decisions
  ARCHITECTURE.md           Architecture overview
```

Every subdirectory under `backend/` and `frontend/src/` has its own `README.md`.

---

## Key design decisions

- **Go over Python** — no ML runs locally; Go handles large concurrent file transfers without blocking threads. Files up to tens of gigabytes upload reliably.
- **Chunked uploads** — files are split into configurable chunks (default 10 MB), uploaded concurrently, and assembled on disk. No memory pressure regardless of file size.
- **Streaming downloads** — `io.Copy` with a 32 KB buffer keeps memory flat for any file size. Multi-file datasets stream as a ZIP archive built on-the-fly.
- **Hybrid search** — keyword search uses a materialized PostgreSQL `tsvector` + GIN index. Semantic search uses stored 768-dim Ollama embeddings. Both run in parallel and are fused with Reciprocal Rank Fusion. Every result is tagged with which method found it (`keyword`, `semantic`, or `hybrid`).
- **Flash for chat, Ollama for vectors** — Flash cannot generate reliable embeddings. Ollama with `nomic-embed-text` handles all vectorization locally on John via CPU matrix math.
- **Bounded profiling** — the profiler reads at most 1 MB per file. Raw file contents are never sent to the AI, so file size does not affect token usage or cost.
- **Auth** — username and password login with signed JWTs (24-hour expiry). Roles: admin and researcher. Public browsing, searching, and downloading require no login. Upload and dataset management require an account.

---

## Access and accounts

- **Browse and search** — no login required. Open the app and search freely.
- **Upload** — requires a researcher or admin account.
- **Edit/delete your datasets** — requires login as the dataset owner or an admin.
- **Manage user accounts** — requires admin login.

The admin account credentials are held by Dr. Cho. To add a researcher account, log in as admin and use the admin panel or the API (see `INSTALL.md`).

---

## Environment variables

Full configuration reference in `backend/config/config.go` and `INSTALL.md`. Key settings:

| Variable | Default on John | Purpose |
|---|---|---|
| `SERVER_PORT` | `8081` | Backend port (8080 is used by GitLab) |
| `DATABASE_URL` | `postgres://labuser:...@localhost:5433/labdatasets` | PostgreSQL connection |
| `AI_BASE_URL` | `http://james:8000` | Flash AI API |
| `OLLAMA_BASE_URL` | `http://localhost:11434` | Ollama embeddings |
| `FILE_STORAGE_DIR` | `/home/rudra/dataset-platform/storage` | Upload destination |
| `JWT_SECRET` | *(set in .env)* | Token signing secret |
| `SEMANTIC_SIMILARITY_THRESHOLD` | `0.6` | Minimum cosine similarity for semantic results |

---

## API endpoints

| Method | Path | Auth required | Purpose |
|---|---|---|---|
| GET | `/api/health` | No | Liveness check |
| GET | `/api/datasets` | No | List all ready datasets |
| POST | `/api/datasets` | Yes | Create a new dataset |
| GET | `/api/datasets/:id` | No | Full dataset detail |
| PUT | `/api/datasets/:id` | Yes (owner or admin) | Update dataset metadata |
| DELETE | `/api/datasets/:id` | Yes (owner or admin) | Delete dataset and files |
| GET | `/api/datasets/:id/download` | No | Download file or ZIP |
| POST | `/api/files/register` | Yes | Pre-create file row for upload |
| POST | `/api/files/chunk` | Yes | Upload one chunk |
| GET | `/api/search` | No | Hybrid search with filters |
| POST | `/api/auth/login` | No | Log in, get JWT |
| GET | `/api/auth/me` | Yes | Get current user info |
| POST | `/api/admin/users` | Admin only | Create a user account |
| GET | `/api/admin/users` | Admin only | List all users |
| DELETE | `/api/admin/users/:id` | Admin only | Delete a user account |

---

## What is pending

| Item | Notes |
|---|---|
| Hosting as a persistent service | Not yet deployed as a systemd service. Must be started manually. Pending professor's decision on deployment approach. |
| Storage location | Files go to John's local disk. Whether to move to a shared network drive across the Hopper cluster is pending professor's confirmation. |
| External dataset API | A third search channel is built into the search service but not connected. Waiting on API credentials. |

---

## Research foundation

The search architecture was informed by six papers:

- Singhal and Srivastava (2017) — multi-field keyword indexing with field weighting
- Tan and Duan (2026) — AI-generated summaries improve recall when descriptions are sparse
- Yang et al. (2026) — dense semantic retrieval with offline vector encoding
- Terrenzi et al. (2026) — hybrid keyword + semantic fusion with RRF, pseudo-query generation
- Gan et al. (2025) — AI-generated descriptions outperform human-authored keywords as a search surface
- Shao and Wijaya (2025) — field-level explainability in search results, informing the citation tag requirement

---

## For a new developer

See `INSTALL.md` for a complete step-by-step setup guide covering prerequisites, database initialization, migrations, configuration, building, running, and troubleshooting.