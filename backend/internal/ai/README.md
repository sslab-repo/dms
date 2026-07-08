# internal/ai

This directory contains the **AI layer**: the only backend package that talks to external AI services.

## What The AI Layer Does

At upload time, it sends dataset metadata and a bounded profile to the university AI API and receives a structured dataset analysis. It also generates semantic embeddings through a local Ollama instance.

Search-time helper methods for query rewriting and reranking live here as well, but the active hybrid search pipeline primarily uses keyword search plus stored embeddings.

## Files

| File | Purpose |
|---|---|
| `client_core.go` | Defines `Client`, the wrapper around the chat API and Ollama embedding service. Provides `complete()` for single-turn chat completions. |
| `analysis.go` | Builds the upload-time analysis prompt, calls the chat API, tries JSON recovery paths, and records which recovery path succeeded. |
| `analysis_parse.go` | Defines `DatasetAnalysis` and `Label`, plus parsers for numbers as strings, nullable label proportions, missing fields, malformed arrays, and partial recovered payloads. |
| `analysis_normalize.go` | Post-processes AI output: trims fields, ensures the researcher appears in the summary, filters ungrounded label names, and applies conservative dataset-type fallback when labels are absent. |
| `search_helpers.go` | Optional search-time AI utilities: `RewriteQuery` and `RerankResults`. |
| `embeddings.go` | Generates semantic vectors via Ollama using the `/api/embeddings` endpoint. |
| `utils.go` | String/JSON recovery helpers: `stripMarkdownFences`, `repairJSON`, `sanitizeMalformedStringArray`, and related utilities for salvaging malformed AI responses. |

## Key Types

- `AnalyzeRequest`: input to `AnalyzeDateset()` with dataset name, researcher, file names, total size, user description, and compact profile JSON.
- `DatasetAnalysis`: output from AI analysis with summary, labels, label completeness, modality, dataset type, annotation format, pseudo-queries, confidence, caveats, and internal parse-recovery bookkeeping.
- `Label`: one class/target value with nullable `proportion` and `sample_count`.

## Important Design Notes

- The chat API is used for structured metadata generation. Ollama handles embeddings locally because embedding vectors need a dedicated embedding endpoint.
- The analysis prompt requires JSON-only output and defines metadata ownership clearly.
- `modality` means primary dataset content, not storage format. CSV, Parquet, JSON, and similar formats influence `annotation_format`; they do not automatically force `modality=tabular`.
- Malformed AI JSON is handled through direct parse, markdown stripping, truncated-JSON repair, and malformed-array sanitization.
- Recovered or partial parses record missing required and metadata fields so the services layer can complete only missing metadata from profiler evidence and add a caveat.
