-- migrations/004_add_dataset_profiles.sql
-- Store the deterministic file/data profile used to generate AI metadata.
-- processing_stage gives the frontend a more specific status while a dataset
-- is being profiled, summarized, and indexed.

ALTER TABLE datasets
    ADD COLUMN IF NOT EXISTS profile_json JSONB;

ALTER TABLE datasets
    ADD COLUMN IF NOT EXISTS processing_stage TEXT NOT NULL DEFAULT '';
