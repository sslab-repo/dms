# internal/search

This directory contains the **search service** — the component that powers dataset discovery.

## What the search service does

- Runs **keyword search** using PostgreSQL full-text search (`tsvector` / `ts_rank_cd`).
- Runs **semantic search** using 768-dimension embeddings, cosine similarity, and a minimum similarity threshold.
- Fuses the two result sets with **Reciprocal Rank Fusion (RRF)**.
- Tags every result with a **citation** indicating whether it came from keyword search, semantic search, or both (hybrid).
- Supports **metadata-only filtering** when no query text is provided.

## Files

| File | Purpose |
|---|---|
| `types.go` | Defines `SearchResult`, `SearchFilters`, and `CitationSource` constants (`keyword`, `semantic`, `hybrid`). |
| `service.go` | Public API: `Service` struct, constructor, configured semantic threshold, and `Search()` entry point that delegates to the hybrid runner. |
| `hybrid.go` | Orchestrates parallel keyword and semantic search, then fuses their rankings with RRF (`k = 60.0`). Also handles the empty-query fallback to `filterOnlySearch`. |
| `keyword.go` | PostgreSQL full-text search over the materialized `datasets.search_vector` GIN index. |
| `semantic.go` | Generates a query embedding via Ollama, compares it against stored `embedding_json` dataset embeddings with `cosineSimilarity`, filters weak matches, and returns the top matches. |
| `filters.go` | `filterOnlySearch` for empty queries and `applyFilters` helper that dynamically appends `WHERE` clauses for modality, dataset type, annotation format, size, and label completeness. |
| `similarity.go` | Pure math utility: computes cosine similarity between two float64 vectors. |
| `query.go` | Shared SQL row scanner that hydrates `[]SearchResult` from database queries. |

## How hybrid search works

1. If the query is empty, skip to `filterOnlySearch`.
2. Launch keyword and semantic searches in parallel goroutines.
3. Collect ranked keyword results plus semantic results that meet the similarity threshold.
4. For each dataset that appears in either list, compute an RRF score:
   `score = sum(1 / (k + rank))` across all lists it appears in.
5. Sort by score descending and take the top 20.
6. Tag each result with its citation source.

## Search surfaces

Keyword search is intentionally exact and structured:

- Weight A: dataset name, label/class names.
- Weight B: pseudo-queries and AI summary.
- Weight C: upload description, tags, researcher name, modality, dataset type, and annotation format.
- The weighted document is stored in `datasets.search_vector` and maintained by the AI pipeline after labels and pseudo-queries are inserted.

Semantic search is intentionally meaning-heavy:

- AI summary.
- Upload description.
- Pseudo-queries.
- Label/class names.
- Compact profile evidence such as column names, sample values, inferred types, file groups, sample text, detected patterns, and annotation class names.
- The resulting embedding is stored separately in `datasets.embedding_json`; semantic search does not use the PostgreSQL `search_vector`.
- Results below `SEMANTIC_SIMILARITY_THRESHOLD` (default `0.6`) are dropped.

## Filter-only mode

When the user selects filters but types no query, `filterOnlySearch` returns all `ready` datasets matching the metadata filters, ordered by most recent upload. No text search is performed.
