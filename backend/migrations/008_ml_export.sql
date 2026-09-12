-- migrations/008_ml_export.sql
-- ML package export: a structured, ML-ready snapshot of each dataset
-- (README datasheet + manifest + raw/ + processed/ + splits/ + build script)
-- built after the AI pipeline finishes and served as a single zip.

-- export_status tracks the ML package lifecycle independently of the main
-- ingestion status, so a dataset can be searchable while its package builds.
-- 'none'     = never built (pre-feature datasets, or ingestion failed)
-- 'building' = export builder running; export_progress is 0.0-1.0
-- 'ready'    = zip on disk at export_path
-- 'error'    = build failed; export_error has details
ALTER TABLE datasets
    ADD COLUMN IF NOT EXISTS export_status   TEXT  NOT NULL DEFAULT 'none';

ALTER TABLE datasets
    ADD COLUMN IF NOT EXISTS export_progress REAL  NOT NULL DEFAULT 0.0;

ALTER TABLE datasets
    ADD COLUMN IF NOT EXISTS export_path     TEXT;

ALTER TABLE datasets
    ADD COLUMN IF NOT EXISTS export_error    TEXT;

ALTER TABLE datasets
    ADD COLUMN IF NOT EXISTS export_built_at TIMESTAMPTZ;

-- SHA-256 of each assembled file, computed by the export builder and recorded
-- in the package manifest so consumers can verify raw data integrity.
ALTER TABLE files
    ADD COLUMN IF NOT EXISTS sha256 TEXT;
