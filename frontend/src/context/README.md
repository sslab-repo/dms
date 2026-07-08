# src/context

Shared React context providers.

## AuthContext

`AuthContext.tsx` owns the frontend auth session:

- Reads the JWT from `localStorage` on app load.
- Validates it with `GET /api/auth/me`.
- Provides `login()` and `logout()` to pages/components.
- Exposes the current user so upload protection and dataset edit/delete controls can check ownership.
