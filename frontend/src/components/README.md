# src/components

This directory contains **shared UI components** used across multiple pages in the frontend.

## Components

| Component | File | Purpose |
|---|---|---|
| `Layout` | `Layout.tsx` | App shell with header (logo, Browse/Upload nav), auth links, main content area (`<Outlet />`), and footer. Handles active link styling. |
| `ProtectedRoute` | `ProtectedRoute.tsx` | Redirects unauthenticated users to `/login` before protected pages such as upload. |
| `SearchBar` | `SearchBar.tsx` | Text input with a submit button. Emits the query string via `onSearch` callback. Clears internal state when `initialQuery` changes from parent. |
| `FilterPanel` | `FilterPanel.tsx` | Collapsible panel with chip selectors for **modality** (text, image, tabular, audio, multimodal), **dataset type** (supervised, unsupervised, etc.), and **annotation format**. Includes numeric range inputs for **size** (MB/GB toggle) and **label completeness** (0-100%). Emits filter changes to parent. |
| `DatasetCard` | `DatasetCard.tsx` | A clickable card (links to `/datasets/:id`) showing dataset name, researcher, AI summary (truncated), metadata tags (modality, type, size, completeness), optional `CitationBadge` and fusion score. Includes a Download button for ready datasets. |
| `CitationBadge` | `CitationBadge.tsx` | Colored tag indicating which retrieval method found a result: **Keyword** (earth tones), **Semantic** (blue tones), **Hybrid** (purple tones). |
| `StatusIndicator` | `StatusIndicator.tsx` | Colored dot + label for dataset status: Pending (gold), Processing (blue), Ready (green), Error (red). |
| `LabelBar` | `LabelBar.tsx` | Weka-style proportional stacked bar chart for class distribution with a color-coded legend below. Uses a 12-color green palette. |
| `UploadProgress` | `UploadProgress.tsx` | Progress bar showing chunk-level upload status with percentage, chunk count, and file index for multi-file datasets. |

## Design notes

- The design aesthetic is editorial/academic: serif headings (Georgia), warm neutral background (`#faf9f7`), green accent (`#2d6a4f`), and structured information density.
- All styling is in `src/styles/` (imported CSS files), not inline styles or CSS-in-JS.
- Components use TypeScript `interface Props` for type safety.
