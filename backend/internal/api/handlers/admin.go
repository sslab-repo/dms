package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"dataset-platform/backend/internal/auth"

	"github.com/lib/pq"
)

type AdminHandler struct {
	db *sql.DB
}

func NewAdminHandler(db *sql.DB) *AdminHandler {
	return &AdminHandler{db: db}
}

type adminCreateUserRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Password    string `json:"password"`
	Role        string `json:"role"`
}

func (h *AdminHandler) HandleUsers(w http.ResponseWriter, req *http.Request) {
	switch req.Method {
	case http.MethodGet:
		h.listUsers(w, req)
	case http.MethodPost:
		h.createUser(w, req)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *AdminHandler) HandleUserByID(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(req.URL.Path, "/api/admin/users/")
	if strings.TrimSpace(id) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing user id"})
		return
	}

	res, err := h.db.ExecContext(req.Context(), `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *AdminHandler) createUser(w http.ResponseWriter, req *http.Request) {
	var body adminCreateUserRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	username := strings.ToLower(strings.TrimSpace(body.Username))
	displayName := strings.TrimSpace(body.DisplayName)
	role := strings.TrimSpace(body.Role)
	if role == "" {
		role = "researcher"
	}
	if username == "" || displayName == "" || body.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username, display_name, and password are required"})
		return
	}
	if role != "admin" && role != "researcher" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "role must be admin or researcher"})
		return
	}

	passwordHash, err := auth.HashPassword(body.Password)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not hash password"})
		return
	}

	var user AuthUserResponse
	err = h.db.QueryRowContext(req.Context(),
		`INSERT INTO users (username, display_name, password_hash, role)
		 VALUES ($1, $2, $3, $4)
		 RETURNING id::text, username, display_name, role`,
		username, displayName, passwordHash, role,
	).Scan(&user.ID, &user.Username, &user.DisplayName, &user.Role)
	if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "23505" {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "username already exists"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	writeJSON(w, http.StatusCreated, user)
}

func (h *AdminHandler) listUsers(w http.ResponseWriter, req *http.Request) {
	rows, err := h.db.QueryContext(req.Context(),
		`SELECT id::text, username, display_name, role
		   FROM users
		  ORDER BY created_at DESC`)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	defer rows.Close()

	users := []AuthUserResponse{}
	for rows.Next() {
		var user AuthUserResponse
		if err := rows.Scan(&user.ID, &user.Username, &user.DisplayName, &user.Role); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			return
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}

	writeJSON(w, http.StatusOK, users)
}
