package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"dataset-platform/backend/internal/auth"
)

type DatasetFileService struct {
	db *sql.DB
}

func NewDatasetFileService(db *sql.DB) *DatasetFileService {
	return &DatasetFileService{db: db}
}

type RegisterAdditionalFileInput struct {
	DatasetID    int
	OriginalName string
	MimeType     string
}

type RegisterAdditionalFileOutput struct {
	FileID  int    `json:"file_id"`
	Message string `json:"message"`
}

type DeleteDatasetFileOutput struct {
	DatasetID   int
	FileID      int
	StoragePath string
}

func (s *DatasetFileService) RegisterAdditionalFile(
	ctx context.Context,
	input RegisterAdditionalFileInput,
	claims *auth.Claims,
) (*RegisterAdditionalFileOutput, error) {
	input.OriginalName = strings.TrimSpace(input.OriginalName)
	if input.DatasetID <= 0 || input.OriginalName == "" {
		return nil, fmt.Errorf("%w: dataset_id and original_name are required", ErrValidation)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if err := s.lockEditableDataset(ctx, tx, input.DatasetID, claims, true); err != nil {
		return nil, err
	}

	var completed, active int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FILTER (WHERE upload_status = 'complete'),
		        COUNT(*) FILTER (WHERE upload_status IN ('uploading', 'assembling'))
		   FROM files
		  WHERE dataset_id = $1`,
		input.DatasetID,
	).Scan(&completed, &active); err != nil {
		return nil, err
	}

	var fileID int
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO files (dataset_id, original_name, mime_type, upload_status)
		 VALUES ($1, $2, $3, 'uploading')
		 RETURNING id`,
		input.DatasetID, input.OriginalName, input.MimeType,
	).Scan(&fileID); err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE datasets
		    SET expected_files = $1,
		        status = 'pending',
		        processing_stage = 'file_edit_upload',
		        error_message = NULL,
		        updated_at = NOW()
		  WHERE id = $2`,
		completed+active+1, input.DatasetID,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &RegisterAdditionalFileOutput{
		FileID:  fileID,
		Message: "File registered. Begin chunk upload using this file_id.",
	}, nil
}

func (s *DatasetFileService) DeleteDatasetFile(
	ctx context.Context,
	fileID int,
	claims *auth.Claims,
) (*DeleteDatasetFileOutput, error) {
	if fileID <= 0 {
		return nil, fmt.Errorf("%w: file_id is required", ErrValidation)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var datasetID int
	var storagePath, uploadStatus string
	err = tx.QueryRowContext(ctx,
		`SELECT f.dataset_id, coalesce(f.storage_path, ''), f.upload_status
		   FROM files f
		  WHERE f.id = $1
		  FOR UPDATE`,
		fileID,
	).Scan(&datasetID, &storagePath, &uploadStatus)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if uploadStatus != "complete" {
		return nil, fmt.Errorf("%w: only complete files can be deleted", ErrConflict)
	}
	if err := s.lockEditableDataset(ctx, tx, datasetID, claims, false); err != nil {
		return nil, err
	}

	var completed int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM files WHERE dataset_id = $1 AND upload_status = 'complete'`,
		datasetID,
	).Scan(&completed); err != nil {
		return nil, err
	}
	if completed <= 1 {
		return nil, fmt.Errorf("%w: cannot delete the last file; delete the dataset instead", ErrConflict)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM files WHERE id = $1`, fileID); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE datasets
		    SET expected_files = $1,
		        status = 'pending',
		        processing_stage = 'file_edit_upload',
		        error_message = NULL,
		        updated_at = NOW()
		  WHERE id = $2`,
		completed-1, datasetID,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return &DeleteDatasetFileOutput{
		DatasetID:   datasetID,
		FileID:      fileID,
		StoragePath: storagePath,
	}, nil
}

func (s *DatasetFileService) lockEditableDataset(
	ctx context.Context,
	tx *sql.Tx,
	datasetID int,
	claims *auth.Claims,
	allowFileEditStaging bool,
) error {
	var ownerID sql.NullString
	var status, stage string
	err := tx.QueryRowContext(ctx,
		`SELECT owner_id::text, status, coalesce(processing_stage, '')
		   FROM datasets
		  WHERE id = $1
		  FOR UPDATE`,
		datasetID,
	).Scan(&ownerID, &status, &stage)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !auth.CanModifyOwner(claims, ownerID) {
		return ErrForbidden
	}
	if status == "ready" || status == "error" {
		return nil
	}
	if allowFileEditStaging && status == "pending" && stage == "file_edit_upload" {
		return nil
	}
	return fmt.Errorf("%w: dataset files can only be changed after processing finishes", ErrConflict)
}
