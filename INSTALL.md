# Dataset Platform — Installation & Handoff Guide

This document is for a new developer (or anyone rebuilding the platform from scratch) taking over this project. It covers every step from a clean server to a fully running application.

---

## Infrastructure overview

| Machine | Role |
|---|---|
| **Bom** | Department cluster server (Hopper). Acts as the SSH jump host. |
| **John** | Application server. Everything runs here: Go backend, React frontend, PostgreSQL, Ollama. |
| **James** | AI API server (port 8000). Runs the Flash model via vLLM, compatible with the OpenAI API format. |

All development and deployment targets **John**. You SSH into John through Bom.

---

## SSH access

```bash
# From your local machine (replace user and IPs as needed)
ssh -J username@bom username@john
```

If you are using VS Code, install the **Remote - SSH** extension and add a host entry in your SSH config:

```
Host john
    HostName <john-ip>
    User <your-username>
    ProxyJump <your-username>@<bom-ip>
```

Then use **Remote-SSH: Connect to Host** in VS Code to open the project folder directly on John.

---

## Prerequisites on John

The following must already be present on John. Check each one before proceeding.

| Tool | Check command | Required version |
|---|---|---|
| Go | `go version` | 1.23 or newer |
| Node.js | `node --version` | 20 or newer |
| npm | `npm --version` | 9 or newer |
| PostgreSQL | `/opt/gitlab/embedded/bin/psql --version` | 16 |
| Ollama | `ollama --version` | Any recent version |
| nomic-embed-text model | `ollama list` | Must appear in list |

If `nomic-embed-text` is not listed, pull it:

```bash
ollama pull nomic-embed-text
```

---

## Project location

The project lives at:

```
/home/rudra/dataset-platform/
```

Directory structure:

```
dataset-platform/
  backend/          Go backend
  frontend/         React frontend
  storage/          Uploaded dataset files (do not delete)
  start.sh          Launch script — starts all three components
  stop.sh           Stop script
  INSTALL.md        This file
  README.md         Project overview
  CONTEXT.md        Full technical context and design decisions
  ARCHITECTURE.md   Architecture overview
```

---

## Database setup

PostgreSQL runs in a **personal cluster** (not the system PostgreSQL) to avoid conflicting with GitLab, which uses port 5432.

| Setting | Value |
|---|---|
| Data directory | `/home/rudra/pgdata` |
| Port | `5433` |
| Database | `labdatasets` |
| User | `labuser` |
| GitLab pg_ctl | `/opt/gitlab/embedded/bin/pg_ctl` |

### Start the database

```bash
/opt/gitlab/embedded/bin/pg_ctl -D /home/rudra/pgdata -l /home/rudra/pgdata/postgres.log start
```

### Check status

```bash
/opt/gitlab/embedded/bin/pg_ctl -D /home/rudra/pgdata status
```

### Connect with psql

```bash
/opt/gitlab/embedded/bin/psql -h localhost -p 5433 -U labuser -d labdatasets
```

### Stop the database

```bash
/opt/gitlab/embedded/bin/pg_ctl -D /home/rudra/pgdata stop
```

### Applying migrations (fresh install only)

If setting up a brand new database, apply all migrations in order:

```bash
cd /home/rudra/dataset-platform/backend/migrations

for f in $(ls *.sql | sort); do
    echo "Applying $f..."
    /opt/gitlab/embedded/bin/psql -h localhost -p 5433 -U labuser -d labdatasets -f "$f"
done
```

---

## Backend

The backend is a Go application. A compiled binary (`main`) already exists in `backend/`. If you need to recompile:

```bash
cd /home/rudra/dataset-platform/backend
go build -o main ./cmd/main.go
```

### Environment configuration

All configuration is in `backend/.env`. A template is at `backend/.env.example`. The current production values on John are:

| Variable | Value on John |
|---|---|
| `SERVER_PORT` | `8081` (8080 is used by GitLab) |
| `DATABASE_URL` | `postgres://labuser:<password>@localhost:5433/labdatasets?sslmode=disable` |
| `AI_BASE_URL` | `http://james:8000` |
| `OLLAMA_BASE_URL` | `http://localhost:11434` |
| `FLASH_MODEL` | `flash` |
| `FILE_STORAGE_DIR` | `/home/rudra/dataset-platform/storage` |
| `SEMANTIC_SIMILARITY_THRESHOLD` | `0.6` |
| `JWT_SECRET` | Set to a long random string — keep this secret |
| `JWT_EXPIRY_HOURS` | `24` |

### Start the backend manually

```bash
cd /home/rudra/dataset-platform/backend
./main
```

Logs print to stdout. To run in the background and save logs:

```bash
nohup ./main > /home/rudra/dataset-platform/backend.log 2>&1 &
```

---

## Frontend

The frontend is a React + TypeScript + Vite application.

### Install dependencies (first time only)

```bash
cd /home/rudra/dataset-platform/frontend
npm install
```

### Environment configuration

Frontend config lives in `frontend/.env`:

| Variable | Value |
|---|---|
| `VITE_UPLOAD_CHUNK_SIZE_MB` | `10` |
| `VITE_UPLOAD_MAX_CONCURRENT_CHUNKS` | `4` |
| `VITE_UPLOAD_MAX_CONCURRENT_FILES` | `2` |

`VITE_API_URL` does not need to be set. The frontend automatically calls the backend on the same host it is served from, so it works correctly whether accessed via `localhost`, an IP address, or a hostname.

### Start the frontend

```bash
cd /home/rudra/dataset-platform/frontend
npm run dev
```

The dev server starts on port 5173 and is accessible on the network at `http://192.168.121.58:5173`.

---

## Starting everything at once

A launch script is provided at the project root:

```bash
cd /home/rudra/dataset-platform
bash start.sh
```

This starts PostgreSQL (if not running), the backend (in the background), and the frontend dev server. All in the correct order with health checks between steps.

To stop everything: CTRL + C

---

## Accessing the application

The application is accessible on the lab network at:

```
http://192.168.121.58:5173
```

No SSH or port forwarding needed. Anyone on the same network as John can open this URL directly.

If you are developing remotely over SSH and want to access it from your own machine, use VS Code port forwarding: open the **Ports** tab → **Forward a Port** → add `5173` and `8081`. Then open `http://localhost:5173` in your browser.

---

## First admin account

There is one admin account seeded in the database. Credentials are held by the professor (Dr. Cho). If you need to reset or create a new admin:

```bash
cd /home/rudra/dataset-platform/backend
go run scripts/seed_admin.go
```

This reads `ADMIN_USERNAME` and `ADMIN_PASSWORD` from the environment (or `.env`) and inserts the admin user. It will fail if the username already exists, so it is safe to re-run.

---

## Adding researcher accounts

There is no self-signup. Accounts are created by an admin via the API:

```bash
curl -X POST http://localhost:8081/api/admin/users \
  -H "Authorization: Bearer <admin-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "researcher1",
    "display_name": "Researcher Full Name",
    "password": "their-password",
    "role": "researcher"
  }'
```

Get the admin token by logging in:

```bash
curl -X POST http://localhost:8081/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username": "sslab123", "password": "<password>"}'
```

---

## What is not yet done

| Item | Status |
|---|---|
| **Hosting as a persistent service** | The backend and frontend are not set up as systemd services. They must be started manually with `start.sh`. Pending professor's confirmation on how to deploy. |
| **Storage location** | Files go to `/home/rudra/dataset-platform/storage` on John's local disk. Whether to move to a shared network drive across Hopper is pending professor's decision. |
| **External dataset API** | A third search channel is designed into the search service but not connected. Waiting on API credentials from the professor. |

---