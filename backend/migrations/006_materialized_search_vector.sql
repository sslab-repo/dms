-- migrations/006_materialized_search_vector.sql
-- Convert datasets.search_vector from a generated column to a plain tsvector
-- so it can include labels and pseudo_queries from child tables.

DROP INDEX IF EXISTS idx_datasets_fts;
DROP INDEX IF EXISTS idx_datasets_search_vector;

ALTER TABLE datasets DROP COLUMN IF EXISTS search_vector;
ALTER TABLE datasets ADD COLUMN search_vector TSVECTOR;

CREATE INDEX IF NOT EXISTS idx_datasets_search_vector
ON datasets USING GIN(search_vector);

UPDATE datasets d
SET search_vector =
    setweight(to_tsvector('english', coalesce(d.name, '')), 'A') ||
    setweight(to_tsvector('english', coalesce(label_text.labels, '')), 'A') ||
    setweight(to_tsvector('english', coalesce(d.ai_summary, '')), 'B') ||
    setweight(to_tsvector('english', coalesce(query_text.queries, '')), 'B') ||
    setweight(to_tsvector('english', coalesce(d.description, '')), 'C') ||
    setweight(to_tsvector('english', array_to_string(coalesce(d.tags, '{}'), ' ')), 'C') ||
    setweight(to_tsvector('english', coalesce(d.researcher_name, '')), 'C') ||
    setweight(to_tsvector('english', coalesce(d.modality, '')), 'C') ||
    setweight(to_tsvector('english', coalesce(d.dataset_type, '')), 'C') ||
    setweight(to_tsvector('english', coalesce(d.annotation_format, '')), 'C')
FROM (
    SELECT dataset_id, string_agg(label_name, ' ') AS labels
    FROM labels
    GROUP BY dataset_id
) label_text
FULL JOIN (
    SELECT dataset_id, string_agg(query_text, ' ') AS queries
    FROM pseudo_queries
    GROUP BY dataset_id
) query_text
ON query_text.dataset_id = label_text.dataset_id
WHERE d.id = coalesce(label_text.dataset_id, query_text.dataset_id);

UPDATE datasets d
SET search_vector =
    setweight(to_tsvector('english', coalesce(d.name, '')), 'A') ||
    setweight(to_tsvector('english', coalesce(d.ai_summary, '')), 'B') ||
    setweight(to_tsvector('english', coalesce(d.description, '')), 'C') ||
    setweight(to_tsvector('english', array_to_string(coalesce(d.tags, '{}'), ' ')), 'C') ||
    setweight(to_tsvector('english', coalesce(d.researcher_name, '')), 'C') ||
    setweight(to_tsvector('english', coalesce(d.modality, '')), 'C') ||
    setweight(to_tsvector('english', coalesce(d.dataset_type, '')), 'C') ||
    setweight(to_tsvector('english', coalesce(d.annotation_format, '')), 'C')
WHERE d.search_vector IS NULL;
