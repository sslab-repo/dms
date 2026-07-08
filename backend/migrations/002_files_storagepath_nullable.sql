-- migrations/002_files_storagepath_nullable.sql
-- storage_path cannot be NOT NULL because the row is created before
-- the file is assembled. Make it nullable and default to empty string
-- so existing rows are unaffected.
ALTER TABLE files ALTER COLUMN storage_path DROP NOT NULL;
ALTER TABLE files ALTER COLUMN storage_path SET DEFAULT '';