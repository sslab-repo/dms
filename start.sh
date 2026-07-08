#!/usr/bin/env bash
# =============================================================================
# start.sh — Dataset Platform
# Starts database, backend, and frontend. Ctrl+C stops everything cleanly
# and asks whether to stop PostgreSQL as well.
# Usage: bash start.sh (from project root)
# =============================================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PG_CTL="/opt/gitlab/embedded/bin/pg_ctl"
PG_DATA="/home/rudra/pgdata"
PG_LOG="$PG_DATA/postgres.log"
BACKEND_DIR="$SCRIPT_DIR/backend"
FRONTEND_DIR="$SCRIPT_DIR/frontend"
BACKEND_LOG="$SCRIPT_DIR/backend.log"

BACKEND_PID=""
FRONTEND_PID=""

# -----------------------------------------------------------------------------
# Cleanup — runs on Ctrl+C or any exit
# -----------------------------------------------------------------------------
cleanup() {
    echo ""
    echo ""
    echo "================================================="
    echo "  Stopping Dataset Platform..."
    echo "================================================="

    # Kill by PID first, then by port as a safety net
    for PID in "$FRONTEND_PID" "$BACKEND_PID"; do
        if [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null; then
            kill -9 "$PID" 2>/dev/null
        fi
    done

    # Clean up anything still on the ports
    for PORT in 5173 8081; do
        LEFTOVER=$(lsof -ti:"$PORT" 2>/dev/null)
        if [ -n "$LEFTOVER" ]; then
            kill -9 $LEFTOVER 2>/dev/null
        fi
    done

    echo "  Frontend and backend stopped."

    if "$PG_CTL" -D "$PG_DATA" status > /dev/null 2>&1; then
        echo ""
        read -r -p "  Stop PostgreSQL as well? [y/N] " response
        if [[ "$response" =~ ^[Yy]$ ]]; then
            "$PG_CTL" -D "$PG_DATA" stop
            echo "  PostgreSQL stopped."
        else
            echo "  PostgreSQL left running."
        fi
    fi

    echo ""
    echo "  Done. Goodbye."
    echo ""
    exit 0
}

trap cleanup EXIT

# -----------------------------------------------------------------------------
# Banner
# -----------------------------------------------------------------------------
echo ""
echo "================================================="
echo "  Dataset Platform — Starting all components"
echo "================================================="
echo ""

# -----------------------------------------------------------------------------
# 1. DATABASE
# -----------------------------------------------------------------------------
echo "[1/3] Checking PostgreSQL..."

if "$PG_CTL" -D "$PG_DATA" status > /dev/null 2>&1; then
    echo "      Already running. Skipping."
else
    echo "      Starting PostgreSQL..."
    "$PG_CTL" -D "$PG_DATA" -l "$PG_LOG" start
    sleep 2
    if "$PG_CTL" -D "$PG_DATA" status > /dev/null 2>&1; then
        echo "      PostgreSQL started."
    else
        echo ""
        echo "ERROR: PostgreSQL failed to start. Check: $PG_LOG"
        exit 1
    fi
fi

# -----------------------------------------------------------------------------
# 2. BACKEND
# -----------------------------------------------------------------------------
echo ""
echo "[2/3] Starting Go backend..."

if [ ! -f "$BACKEND_DIR/main" ]; then
    echo "      No binary found. Building..."
    cd "$BACKEND_DIR"
    go build -o main ./cmd/main.go
    echo "      Build complete."
fi

# Stop anything already on 8081
if lsof -ti:8081 > /dev/null 2>&1; then
    echo "      Port 8081 in use. Stopping existing process..."
    kill -9 "$(lsof -ti:8081)" 2>/dev/null || true
    sleep 1
fi

cd "$BACKEND_DIR"
./main >> "$BACKEND_LOG" 2>&1 &
BACKEND_PID=$!
echo "      Backend started (PID $BACKEND_PID). Log: $BACKEND_LOG"

echo "      Waiting for backend to be ready..."
for i in {1..15}; do
    if curl -s http://localhost:8081/api/health > /dev/null 2>&1; then
        echo "      Backend is ready."
        break
    fi
    if [ "$i" -eq 15 ]; then
        echo ""
        echo "ERROR: Backend did not become ready. Check: $BACKEND_LOG"
        exit 1
    fi
    sleep 1
done

# -----------------------------------------------------------------------------
# 3. FRONTEND
# -----------------------------------------------------------------------------
echo ""
echo "[3/3] Starting frontend..."

if [ ! -d "$FRONTEND_DIR/node_modules" ]; then
    echo "      node_modules not found. Running npm install..."
    cd "$FRONTEND_DIR"
    npm install
fi

cd "$FRONTEND_DIR"
npm run dev -- --host &
FRONTEND_PID=$!

# -----------------------------------------------------------------------------
# Ready
# -----------------------------------------------------------------------------
sleep 2
echo ""
echo "================================================="
echo "  All components running."
echo ""
echo "  Open in browser:  http://192.168.121.58:5173"
echo "  Backend API:      http://localhost:8081"
echo "  Backend log:      $BACKEND_LOG"
echo "  Database log:     $PG_LOG"
echo ""
echo "  Press Ctrl+C to stop everything."
echo "================================================="
echo ""

wait $FRONTEND_PID