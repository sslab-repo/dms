# src/pages

This directory contains the **page-level components** — one per route in the application.

## Pages

| Page | Route | File | Purpose |
|---|---|---|---|
| Home | `/` | `Home.tsx` | Landing page with two modes: **browse** (lists all ready datasets) and **search** (keyword + filters with results grid). |
| Login | `/login` | `Login.tsx` | Username/password sign-in for upload and dataset management. |
| Upload | `/upload` | `Upload.tsx` | Protected multi-step upload wizard with metadata form, file selection, chunked upload, and AI pipeline polling. |
| DatasetDetail | `/datasets/:id` | `DatasetDetail.tsx` | Full metadata view with owner/admin edit and delete controls. |

## App entry point

| File | Purpose |
|---|---|
| `App.tsx` | Root component that sets up React Router with a `BrowserRouter`, three routes nested inside `Layout`. |
| `main.tsx` | React DOM entry point — renders `<App />` inside `StrictMode`. |

## Routes

- `/` → `Home` — browse all datasets or search with filters.
- `/login` → `Login` — sign in and return to the requested protected route.
- `/upload` → `Upload` — login required; upload a new dataset (4 steps: info, upload, processing, done/error).
- `/datasets/:id` → `DatasetDetail` — view dataset metadata and download files.

All routes are wrapped in the `Layout` component which provides the header/nav and footer.

## App.tsx

```tsx
<BrowserRouter>
  <Routes>
    <Route element={<Layout />}>
      <Route path="/" element={<Home />} />
      <Route path="/login" element={<Login />} />
      <Route path="/upload" element={<ProtectedRoute><Upload /></ProtectedRoute>} />
      <Route path="/datasets/:id" element={<DatasetDetail />} />
    </Route>
  </Routes>
</BrowserRouter>
```

## Page details

### Home page

Has two view modes:
- **Browse mode** (default) — loads `GET /api/datasets` on mount and displays summary cards in a paginated grid (20 at a time, "Load More" button).
- **Search mode** — triggered by typing a query or selecting filters. Uses `GET /api/search` and displays result cards with citation badges and fusion scores. Includes a "Clear search and filters" reset button.

### Upload page

Four-step flow with a visual step indicator:
1. **Info** — form fields (name, researcher, description, tags) + file selection via click-to-select or drag-and-drop.
2. **Uploading** — overall dataset upload progress using configurable chunks via `POST /api/files/chunk`.
3. **Processing** — polls `GET /api/datasets/:id` every 2 seconds until status changes to `ready` or `error`. Shows `processing_stage` messages.
4. **Done/Error** — success shows link to view dataset; error shows message with retry button.

### Dataset Detail page

Two-column layout:
- **Left column:** Overview (researcher, date, description, AI summary), Classification (modality, type, format, tags, size, label completeness bar, AI confidence, caveats), Data Profile (file types, patterns, annotation summaries, grouped file profiles with sample rows/tables), Files list with download links.
- **Right column (sticky):** Label distribution bar chart (Weka-style), suggested pseudo-queries, download button, and owner/admin management controls. Shows appropriate messages for pending/processing/error states.
