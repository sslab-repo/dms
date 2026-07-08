-- migrations/003_add_unique_constraints.sql
-- Run this migration to add deduplication constraints for pseudo_queries

-- Add unique index to prevent future duplicates
CREATE UNIQUE INDEX IF NOT EXISTS idx_pseudo_queries_unique 
ON pseudo_queries(dataset_id, query_text);

-- Add index on label_name for faster label lookups
CREATE INDEX IF NOT EXISTS idx_labels_name ON labels(label_name);

-- Add expected_files column to track how many files a dataset should have
-- This prevents the AI pipeline from triggering prematurely when files are uploaded sequentially
ALTER TABLE datasets ADD COLUMN IF NOT EXISTS expected_files INTEGER NOT NULL DEFAULT 0;
