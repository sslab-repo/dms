package api

/*
	This file wires all HTTP routes to their handlers.

	The router is the only place in the backend that knows about HTTP.
	Every other component works with plain Go types.

	ENDPOINTS:
		GET  /api/health                — liveness check
		POST /api/datasets              — create dataset record
		GET  /api/datasets              — list ready datasets only
		GET  /api/datasets/:id          — single dataset with full metadata
		GET  /api/datasets/:id/download — stream original file(s) to client
		GET  /api/datasets/:id/export   — stream the prebuilt ML package zip
		POST /api/files/register        — pre-create a files row, get file_id
		POST /api/files/chunk           — upload one chunk of a large file
		GET  /api/search                — hybrid search with citation tags

	UPLOAD FLOW (three steps the frontend follows):
		1. POST /api/datasets       → returns dataset_id
		2. POST /api/files/register → returns file_id  (called once per file)
		3. POST /api/files/chunk    → repeated until done=true for every file

	When all files for a dataset are assembled on disk, the file handler
	fires the OnFileAssembled callback. runAIPipeline then runs once,
	calls Flash, stores labels + pseudo-queries + embedding, and sets
	the dataset status to 'ready' so it becomes searchable.
*/

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"dataset-platform/backend/internal/ai"
	"dataset-platform/backend/internal/api/handlers"
	"dataset-platform/backend/internal/auth"
	"dataset-platform/backend/internal/filehandler"
	"dataset-platform/backend/internal/mlexport"
	"dataset-platform/backend/internal/search"
	"dataset-platform/backend/internal/services"
)

// Router holds references to all components and implements http.Handler.
type Router struct {
	db                 *sql.DB
	aiClient           *ai.Client
	fileHandler        *filehandler.Handler
	pipelineSvc        *services.PipelineService
	healthHandler      *handlers.HealthHandler
	authHandler        *handlers.AuthHandler
	adminHandler       *handlers.AdminHandler
	datasetHandler     *handlers.DatasetHandler
	datasetFileHandler *handlers.DatasetFileHandler
	searchHandler      *handlers.SearchHandler
	jwtSecret          string
	mux                *http.ServeMux
}

// NewRouter wires all routes and returns the router.
// fileStorageDir is the root of on-disk dataset storage; ML package exports
// are built under <fileStorageDir>/exports/.
func NewRouter(
	db *sql.DB,
	aiClient *ai.Client,
	fh *filehandler.Handler,
	ss *search.Service,
	fileStorageDir string,
	jwtSecret string,
	jwtExpiry time.Duration,
) *Router {
	exportSvc := mlexport.NewService(db, fileStorageDir)
	exportSvc.RecoverInterrupted(context.Background())

	r := &Router{
		db:                 db,
		aiClient:           aiClient,
		fileHandler:        fh,
		pipelineSvc:        services.NewPipelineService(db, aiClient, exportSvc),
		healthHandler:      handlers.NewHealthHandler(),
		authHandler:        handlers.NewAuthHandler(db, jwtSecret, jwtExpiry),
		adminHandler:       handlers.NewAdminHandler(db),
		datasetHandler:     handlers.NewDatasetHandler(services.NewDatasetService(db), fh, exportSvc),
		datasetFileHandler: handlers.NewDatasetFileHandler(services.NewDatasetFileService(db), fh),
		searchHandler:      handlers.NewSearchHandler(ss),
		jwtSecret:          jwtSecret,
		mux:                http.NewServeMux(),
	}

	// Wire the AI pipeline callback.
	// The file handler calls this once ALL files in a dataset are assembled.
	// It is never called once per individual file.
	fh.OnFileAssembled = r.pipelineSvc.Run

	r.registerRoutes()
	return r
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// CORS headers so the React frontend on a different port can call us
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	// Defensive security headers on every response
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
	w.Header().Set("Content-Security-Policy", "default-src 'none'")
	w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
	if req.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	r.mux.ServeHTTP(w, req)
}

func (r *Router) registerRoutes() {
	r.mux.HandleFunc("/api/health", r.healthHandler.HandleHealth)
	r.mux.HandleFunc("/api/auth/login", auth.RateLimitLogin(withBodyLimit(r.authHandler.HandleLogin)))
	r.mux.HandleFunc("/api/auth/me", r.withAuth(r.authHandler.HandleMe))
	r.mux.HandleFunc("/api/admin/users", r.withAdmin(r.adminHandler.HandleUsers))
	r.mux.HandleFunc("/api/admin/users/", r.withAdmin(withBodyLimit(r.adminHandler.HandleUserByID)))
	r.mux.HandleFunc("/api/datasets", r.handleDatasets)
	r.mux.HandleFunc("/api/datasets/", r.handleDatasetByID) // covers /:id and /:id/download
	r.mux.HandleFunc("/api/files/register", r.withOptionalAuth(withBodyLimit(r.fileHandler.RegisterFile)))
	r.mux.HandleFunc("/api/files/chunk", r.withOptionalAuth(r.fileHandler.HandleChunkUpload))
	r.mux.HandleFunc("/api/files/", r.withAuth(r.datasetFileHandler.HandleFileByID))
	r.mux.HandleFunc("/api/search", r.searchHandler.HandleSearch)
}

func (r *Router) handleDatasets(w http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodPost {
		r.withOptionalAuth(withBodyLimit(r.datasetHandler.HandleDatasets))(w, req)
		return
	}
	r.datasetHandler.HandleDatasets(w, req)
}

func (r *Router) handleDatasetByID(w http.ResponseWriter, req *http.Request) {
	if req.Method == http.MethodPost {
		r.withAuth(r.datasetFileHandler.HandleDatasetFileRegister)(w, req)
		return
	}
	if req.Method == http.MethodPut || req.Method == http.MethodDelete {
		r.withAuth(r.datasetHandler.HandleDatasetByID)(w, req)
		return
	}
	r.datasetHandler.HandleDatasetByID(w, req)
}

func (r *Router) withAuth(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		auth.RequireAuth(r.jwtSecret, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			claims, ok := auth.ClaimsFromContext(req.Context())
			if !ok {
				writeRouteJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
				return
			}

			err := r.db.QueryRowContext(req.Context(),
				`SELECT username, display_name, role FROM users WHERE id = $1`,
				claims.UserID,
			).Scan(&claims.Username, &claims.DisplayName, &claims.Role)
			if errors.Is(err, sql.ErrNoRows) {
				writeRouteJSON(w, http.StatusUnauthorized, map[string]string{"error": "user no longer exists"})
				return
			}
			if err != nil {
				writeRouteJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
				return
			}

			fn(w, req.WithContext(auth.WithClaims(req.Context(), claims)))
		})).ServeHTTP(w, req)
	}
}

// withOptionalAuth injects verified claims when a valid Bearer token is
// present. Requests without a token (or with an invalid one) proceed without
// claims — useful for endpoints that are open to anonymous users but can still
// recognise logged-in ones.
func (r *Router) withOptionalAuth(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		auth.OptionalAuth(r.jwtSecret, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			// If claims are present, hydrate them from the DB so Role/DisplayName are set.
			if claims, ok := auth.ClaimsFromContext(req.Context()); ok {
				err := r.db.QueryRowContext(req.Context(),
					`SELECT username, display_name, role FROM users WHERE id = $1`,
					claims.UserID,
				).Scan(&claims.Username, &claims.DisplayName, &claims.Role)
				if err == nil {
					req = req.WithContext(auth.WithClaims(req.Context(), claims))
				} else {
					// Token is valid but user no longer exists — treat as anonymous.
					req = req.WithContext(auth.WithClaims(req.Context(), nil))
				}
			}
			fn(w, req)
		})).ServeHTTP(w, req)
	}
}


func (r *Router) withAdmin(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		r.withAuth(func(w http.ResponseWriter, req *http.Request) {
			claims, ok := auth.ClaimsFromContext(req.Context())
			if !ok || claims.Role != "admin" {
				writeRouteJSON(w, http.StatusForbidden, map[string]string{"error": "forbidden"})
				return
			}
			fn(w, req)
		})(w, req)
	}
}

func writeRouteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
