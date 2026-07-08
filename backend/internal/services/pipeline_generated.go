package services

import "context"

func (p *PipelineService) clearGeneratedMetadata(ctx context.Context, datasetID int) error {
	if _, err := p.db.ExecContext(ctx, `DELETE FROM labels WHERE dataset_id = $1`, datasetID); err != nil {
		return err
	}
	if _, err := p.db.ExecContext(ctx, `DELETE FROM pseudo_queries WHERE dataset_id = $1`, datasetID); err != nil {
		return err
	}
	_, err := p.db.ExecContext(ctx,
		`UPDATE datasets
		    SET ai_summary = NULL,
		        modality = NULL,
		        dataset_type = NULL,
		        annotation_format = NULL,
		        label_completeness = 0,
		        total_size_bytes = 0,
		        embedding_json = NULL,
		        ai_confidence = 0,
		        ai_caveats = '{}',
		        profile_json = NULL,
		        search_vector = NULL,
		        error_message = NULL,
		        updated_at = NOW()
		  WHERE id = $1`,
		datasetID,
	)
	return err
}
