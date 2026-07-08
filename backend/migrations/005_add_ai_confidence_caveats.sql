-- migrations/005_add_ai_confidence_caveats.sql
-- Store AI confidence and caveats so researchers can see when metadata was
-- inferred from bounded samples or incomplete evidence.

ALTER TABLE datasets
    ADD COLUMN IF NOT EXISTS ai_confidence REAL NOT NULL DEFAULT 0.0;

ALTER TABLE datasets
    ADD COLUMN IF NOT EXISTS ai_caveats TEXT[] NOT NULL DEFAULT '{}';
