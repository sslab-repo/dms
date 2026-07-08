# scripts

Small operational helpers for the backend.

## seed_admin.go

Creates or updates the first admin account after migration `007_auth_users.sql` has been applied.

```bash
ADMIN_USERNAME=professor ADMIN_PASSWORD='change-me' go run scripts/seed_admin.go
```

Optional: set `ADMIN_DISPLAY_NAME`; otherwise it defaults to the username.
