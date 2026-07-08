# internal/profiler

This directory contains the **dataset profiler** — a bounded, deterministic component that inspects uploaded files to produce a structured profile without reading the entire dataset into memory.

## What the profiler does

- Samples inspectable formats: CSV, TSV, JSON, JSONL, Parquet, and plain text.
- Groups multi-file uploads by type, role, and shared schema.
- Extracts column names, inferred types, and sample rows.
- Produces a `DatasetProfile` that the AI pipeline uses as evidence for Flash.
- Stores `profile_json` so the frontend and AI prompt use the same evidence.

## Files

| File | Purpose |
|---|---|
| `profiler.go` | Main entry point `ProfileDataset()`. Orchestrates profiling across all files, groups them, and assembles the final `DatasetProfile`. |
| `types.go` | Defines all profiler structs: `DatasetProfile`, `FileProfile`, `FileGroup`, `ColumnProfile`, `AnnotationProfile`, `ClassProfile`, etc. |
| `file_profile.go` | Called per file to detect format and route to the correct profiler (CSV, JSON, text, Parquet). |
| `detect.go` | Format detection logic: sniffs file extensions and content headers to decide how to profile a file. |
| `columns.go` | Column-level analysis: infers types, counts non-empty vs empty values, and collects example values. |
| `grouping.go` | Groups files by detected type and shared schema so multi-file datasets are analyzed together rather than separately. |
| `annotations.go` | Reads annotation files (COCO JSON, YOLO TXT) and produces `AnnotationProfile` with class counts and proportions. |
| `delimited.go` | Parses delimited text formats (CSV/TSV) with a bounded number of rows and columns. |
| `json.go` | Parses JSON and JSONL files, infers schema, and extracts sample records. |
| `text.go` | Handles plain text files: line counting, sampling, and basic statistics. |
| `parquet.go` | Reads Parquet files to extract schema and metadata (reads only the footer, not full data). |
| `patterns.go` | Detects common dataset patterns (e.g., train/val/test splits) from file names and groupings. |
| `util.go` | Shared utilities for the profiler: string helpers, type inference, etc. |

## Key constraints

- Maximum **30 files** are individually profiled; remaining files are counted in aggregates.
- Maximum **3 representative files** per group.
- Maximum **20 columns** per file.
- Maximum **5 sample rows** and **12 text lines**.
- Maximum **1 MB** read per file.

These bounds keep the profiler fast and memory-safe regardless of dataset size.

## Output

The profiler returns a `DatasetProfile` JSON object that includes:
- File type summaries (count and size per type)
- Grouped file sets with shared columns
- Sample rows and columns per group
- Annotation summaries (if applicable)
- Detected patterns (e.g., train/validation/test splits)
- Notes about sampling limitations

This profile is then passed to Flash as structured evidence for AI analysis.
