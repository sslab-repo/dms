package handlers

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"dataset-platform/backend/internal/auth"
)

type AuthHandler struct {
	db        *sql.DB
	jwtSecret string
	jwtExpiry time.Duration
}

func NewAuthHandler(db *sql.DB, jwtSecret string, jwtExpiry time.Duration) *AuthHandler {
	return &AuthHandler{db: db, jwtSecret: jwtSecret, jwtExpiry: jwtExpiry}
}

type AuthUserResponse struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (h *AuthHandler) HandleLogin(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var body loginRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	username := strings.ToLower(strings.TrimSpace(body.Username))
	if username == "" || body.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password are required"})
		return
	}

	var user AuthUserResponse
	var passwordHash string
	err := h.db.QueryRowContext(req.Context(),
		`SELECT id::text, username, display_name, password_hash, role
		   FROM users
		  WHERE username = $1`,
		username,
	).Scan(&user.ID, &user.Username, &user.DisplayName, &passwordHash, &user.Role)
	if err == sql.ErrNoRows {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid username or password"})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return
	}
	if !auth.ComparePassword(passwordHash, body.Password) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid username or password"})
		return
	}

	token, err := auth.SignJWT(user.ID, user.Username, user.DisplayName, user.Role, h.jwtSecret, h.jwtExpiry)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not sign token"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user":  user,
	})
}

func (h *AuthHandler) HandleMe(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	claims, ok := auth.ClaimsFromContext(req.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "not authenticated"})
		return
	}

	writeJSON(w, http.StatusOK, AuthUserResponse{
		ID:          claims.UserID,
		Username:    claims.Username,
		DisplayName: claims.DisplayName,
		Role:        claims.Role,
	})
}
