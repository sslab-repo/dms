# src/types

This directory contains all **TypeScript type definitions** that mirror the backend JSON responses.

## What is here

- `index.ts` — every interface used across the frontend.

## Key types

| Type | Purpose |
|---|---|
| `Dataset` | Full dataset detail (labels, pseudo_queries, files, profile, status, error_message) |
| `DatasetSummary` | Compact list view (used on the Home page browse mode) |
| `SearchResult` | Search response with `citation` and `fusion_score` |
| `SearchResponse` | Wrapper containing query, count, and results array |
| `SearchFilters` | Metadata filter parameters (modality, dataset_type, annotation_format, size, label_completeness) |
| `CreateDatasetRequest` | Payload for `POST /api/datasets` |
| `CreateDatasetResponse` | Response with `dataset_id` |
| `UpdateDatasetRequest` | Payload for owner/admin metadata edits |
| `AuthUser` | Current signed-in user returned by auth endpoints |
| `LoginResponse` | JWT plus current user returned by login |
| `RegisterFileResponse` | Response with `file_id` for chunked upload |
| `ChunkUploadResponse` | Chunk upload status with `done` and `all_done` flags |
| `Label` | One AI-generated class with proportion and sample count |
| `FileInfo` | One uploaded file's metadata |
| `DatasetProfile` | Full profiler output (file_types, groups, annotations, patterns) |
| `FileGroup` | Grouped files with shared columns and roles |
| `FileProfile` | Per-file profile (columns, sample rows, sample text) |
| `ColumnProfile` | Column metadata (name, inferred type, example values) |
| `AnnotationProfile` | Annotation file summary (format, class count, classes) |
| `ClassProfile` | Single annotation class |

All types are used by the API client ([`../api/client.ts`](../api/client.ts)) and the page/component props throughout the app.
