# internal/auth

Local authentication helpers for the dataset platform.

## Responsibilities

- Hash and compare passwords with bcrypt.
- Sign and verify HS256 JWTs using `JWT_SECRET`.
- Store authenticated user claims on request context.
- Provide middleware for `RequireAuth` and `RequireRole`.

## Auth Model

V1 uses admin-created accounts with `admin` and `researcher` roles. Admins can mutate any dataset, including legacy datasets with `owner_id = NULL`. Researchers can mutate only datasets they own. Public browse, detail, search, and download routes do not require a token.
