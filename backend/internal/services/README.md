# internal/services

This directory contains high-level domain services that orchestrate complex operations across multiple internal packages.

## What is here

| File | Purpose |
|---|---|
| `pipeline_service.go` | The AI pipeline that runs once all files for a dataset are assembled. |
| `dataset_service.go` | CRUD operations for dataset records, ownership checks, metadata edits, delete cleanup planning, and keyword search-vector rebuilds. |
| `analysis_metadata.go` | Helpers for correcting AI metadata with profiler evidence. |
| `ai_profile.go` | Builds compact JSON profiles from profiler output for AI analysis, with bounded limits on groups, columns, rows, and character counts. |
| `label_fields.go` | Derives label field evidence from profiler columns, reconciles AI-generated labels with profile evidence, and determines which labels represent true class distributions versus inferred feature columns. |

## What `pipeline_service.go` does

The `PipelineService` is the heart of the upload-time AI enrichment. It is wired to `filehandler.OnFileAssembled` so it runs exactly once per dataset, only after all files are complete.

### Pipeline steps

1. **Idempotency guard** - checks that the dataset is still `pending`. If not, skips.
2. **Marks `processing`** - atomically updates the dataset status.
3. **Profiles files** - calls `profiler.ProfileDataset()` to build a bounded, deterministic profile from the uploaded files.
4. **Stores profile JSON** - saves the profile into `datasets.profile_json`.
5. **Reads metadata** - fetches the dataset name, researcher, and user description.
6. **Calls Flash** - sends the profile and metadata to `aiClient.AnalyzeDateset()`.
7. **Applies profile corrections and reconciles labels** - filters AI labels using profile evidence and determines true class distributions.
8. **Inserts labels** - stores each AI-generated label in the `labels` table (non-fatal per label).
9. **Inserts pseudo-queries** - stores up to 10 deduplicated pseudo-queries in the `pseudo_queries` table (non-fatal per query).
10. **Updates keyword search vector** - materializes `datasets.search_vector` after labels and pseudo-queries are available.
11. **Generates embedding** - calls Ollama to produce a 768-dimension vector of profile-enriched search text, capped before embedding, then stores it as JSON in `embedding_json`.
12. **Updates dataset** - writes all AI results back to the `datasets` row and sets status to `ready`.

If any fatal step fails, the dataset is marked `error` with a descriptive message.

### Profile corrections

After Flash returns, `applyProfileCorrections` reconciles the AI analysis with the profiler's findings to improve accuracy:

- Annotation format is replaced with profiler-backed detection when available.
- Modality falls back to profiler detection when AI returns `unknown`.
- Display values are normalized, such as `csv` to `CSV` and `coco json` to `COCO JSON`.

## What `dataset_service.go` does

Provides data access for the dataset handlers:

- `ListReadyDatasets()` returns all `ready` datasets ordered by most recent upload.
- `CreateDataset()` inserts a new owner-linked dataset in `pending` status.
- `GetDataset()` returns full dataset metadata including labels, pseudo-queries, files, and profile JSON.
- `UpdateDataset()` lets owners/admins edit text metadata on `ready` or `error` datasets and rebuilds keyword search without rerunning AI.
- `DeleteDataset()` lets owners/admins delete a dataset row and returns file paths for best-effort disk cleanup.

## What `analysis_metadata.go` does

Helpers for correcting generated metadata:

- `applyProfileCorrections()` reconciles AI output with profiler evidence.
- `profileBackedFormat()` derives annotation format from profiler annotations and file types.
- `profileBackedModality()` derives modality from detected file types when AI output is unknown.
- `normalizeDisplayValue()` normalizes format names for consistent display.
- `normalizeModality()` ensures modality matches allowed values.
- `normalizeDatasetType()` ensures dataset type matches allowed values.

## What `label_fields.go` does

Derives and reconciles label metadata from profiler evidence:

- `deriveLabelFields()` extracts candidate label fields from columns with target-like names.
- `reconcileLabelMetadata()` filters AI labels using profile evidence, updates label completeness, and adjusts dataset type.
- `filterClassLabelsForProfile()` removes AI labels that appear to be feature columns based on profile evidence.
- `detailLabelShouldBeClass()` determines whether a label should be shown as a class distribution or hidden as an inferred feature.
- `legacyLabelField()` converts legacy AI labels with null proportion into label field evidence.

Key design principle: AI-generated labels are filtered and refined using concrete profiler evidence to avoid showing feature columns, such as `vendor_id` or `payment_type`, as class labels.

## What `ai_profile.go` does

Builds bounded JSON profiles for AI consumption:

- `buildAIProfileJSON()` creates a compact profile with limits on groups, columns, rows, examples, and character counts.
- `compactGroups()` reduces file groups to essential metadata.
- `compactColumns()` limits columns and truncates example values.
- `compactAnnotations()` bounds annotation metadata and class lists.
- `truncateForAI()` removes null bytes and truncates long values.

The goal is to provide Flash with enough structured evidence for accurate analysis while staying within token limits.
