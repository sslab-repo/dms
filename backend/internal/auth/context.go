package auth

import (
	"context"
	"database/sql"
)

type contextKey string

const claimsContextKey contextKey = "authClaims"

func WithClaims(ctx context.Context, claims *Claims) context.Context {
	return context.WithValue(ctx, claimsContextKey, claims)
}

func ClaimsFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(claimsContextKey).(*Claims)
	return claims, ok && claims != nil
}

func CanModifyOwner(claims *Claims, ownerID sql.NullString) bool {
	if claims == nil {
		return false
	}
	if claims.Role == "admin" {
		return true
	}
	return ownerID.Valid && claims.UserID == ownerID.String
}

// CanUploadForDataset returns true when the caller may add files to a dataset.
// Datasets with no owner (owner_id IS NULL) accept uploads from anyone — the
// dataset_id returned at creation acts as an implicit upload token.
// Owned datasets require the owner or an admin.
func CanUploadForDataset(claims *Claims, ownerID sql.NullString) bool {
	if !ownerID.Valid {
		return true // anonymous dataset: open for upload
	}
	return CanModifyOwner(claims, ownerID)
}
