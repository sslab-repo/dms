package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHashAndComparePassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	if !ComparePassword(hash, "correct horse battery staple") {
		t.Fatal("expected password to match hash")
	}
	if ComparePassword(hash, "wrong password") {
		t.Fatal("expected wrong password not to match hash")
	}
}

func TestSignAndVerifyJWT(t *testing.T) {
	token, err := SignJWT("user-1", "ruser", "Research User", "researcher", "secret-a", time.Hour)
	if err != nil {
		t.Fatalf("SignJWT returned error: %v", err)
	}

	claims, err := VerifyJWT(token, "secret-a")
	if err != nil {
		t.Fatalf("VerifyJWT returned error: %v", err)
	}
	if claims.UserID != "user-1" || claims.Username != "ruser" || claims.DisplayName != "Research User" || claims.Role != "researcher" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestVerifyJWTRejectsWrongSecretAndExpiredTokens(t *testing.T) {
	token, err := SignJWT("user-1", "ruser", "Research User", "researcher", "secret-a", time.Hour)
	if err != nil {
		t.Fatalf("SignJWT returned error: %v", err)
	}
	if _, err := VerifyJWT(token, "secret-b"); err == nil {
		t.Fatal("expected wrong secret to fail verification")
	}

	expired, err := SignJWT("user-1", "ruser", "Research User", "researcher", "secret-a", -time.Hour)
	if err != nil {
		t.Fatalf("SignJWT returned error: %v", err)
	}
	if _, err := VerifyJWT(expired, "secret-a"); err == nil {
		t.Fatal("expected expired token to fail verification")
	}
}

func TestRequireRoleStatuses(t *testing.T) {
	secret := "middleware-secret"
	adminToken, err := SignJWT("admin-1", "admin", "Admin User", "admin", secret, time.Hour)
	if err != nil {
		t.Fatalf("SignJWT admin: %v", err)
	}
	researcherToken, err := SignJWT("user-1", "ruser", "Research User", "researcher", secret, time.Hour)
	if err != nil {
		t.Fatalf("SignJWT researcher: %v", err)
	}

	handler := RequireRole(secret, "admin", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status=%d, want %d", res.Code, http.StatusUnauthorized)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+researcherToken)
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusForbidden {
		t.Fatalf("wrong role status=%d, want %d", res.Code, http.StatusForbidden)
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	res = httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	if res.Code != http.StatusNoContent {
		t.Fatalf("admin status=%d, want %d", res.Code, http.StatusNoContent)
	}
}
