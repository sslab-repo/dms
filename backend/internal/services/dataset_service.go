package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"dataset-platform/backend/internal/auth"
	"dataset-platform/backend/internal/profiler"

	"github.com/lib/pq"
)

var (
	ErrValidation = errors.New("validation error")
	ErrNotFound   = errors.New("not found")
	ErrForbidden  = errors.New("forbidden")
	ErrConflict   = errors.New("conflict")
)

type DatasetService struct {
	db *sql.DB
}

func NewDatasetService(db *sql.DB) *DatasetService {
	return &DatasetService{db: db}
}

type CreateDatasetInput struct {
	Name            string
	ResearcherName  string
	UploaderEmail   string
	UserDescription string
	Tags            []string
	TotalFiles      int
	OwnerID         string // empty string means anonymous
	LabelColumn     string
}

type CreateDatasetOutput struct {
	DatasetID int
	Status    string
	Message   string
}

func (s *DatasetService) CreateDataset(ctx context.Context, input CreateDatasetInput) (*CreateDatasetOutput, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.ResearcherName = strings.TrimSpace(input.ResearcherName)
	if input.Name == "" || input.ResearcherName == "" {
		return nil, fmt.Errorf("%w: name and researcher_name are required", ErrValidation)
	}
	if input.TotalFiles <= 0 {
		return nil, fmt.Errorf("%w: total_files must be greater than 0", ErrValidation)
	}
	if input.Tags == nil {
		input.Tags = []string{}
	}

	var datasetID int
	err := s.db.QueryRowContext(ctx,
		`INSERT INTO datasets (name, researcher_name, uploader_email, description, tags, expected_files, status, owner_id, label_column)
		VALUES ($1, $2, NULLIF($3,''), $4, $5, $6, 'pending', NULLIF($7,''), NULLIF($8,'')) RETURNING id`,
		input.Name, input.ResearcherName, input.UploaderEmail,
		input.UserDescription, pq.Array(input.Tags), input.TotalFiles,
		input.OwnerID, input.LabelColumn,
	).Scan(&datasetID)
	if err != nil {
		return nil, err
	}

	return &CreateDatasetOutput{
		DatasetID: datasetID,
		Status:    "pending",
		Message:   "Dataset created. Register files with POST /api/files/register, then upload chunks with POST /api/files/chunk.",
	}, nil
}

type DatasetSummary struct {
	ID                int       `json:"id"`
	Name              string    `json:"name"`
	ResearcherName    string    `json:"researcher_name"`
	OwnerID           *string   `json:"owner_id"`
	OwnerDisplayName  string    `json:"owner_display_name"`
	AISummary         string    `json:"ai_summary"`
	Modality          string    `json:"modality"`
	DatasetType       string    `json:"dataset_type"`
	LabelCompleteness float64   `json:"label_completeness"`
	TotalSizeBytes    int64     `json:"total_size_bytes"`
	Status            string    `json:"status"`
	UploadedAt        time.Time `json:"uploaded_at"`
	Tags              []string  `json:"tags"`
}

func (s *DatasetService) ListReadyDatasets(ctx context.Context) ([]DatasetSummary, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT d.id, d.name, d.researcher_name,
		        d.owner_id::text, coalesce(u.display_name, ''),
		        coalesce(ai_summary, ''), coalesce(modality, ''), coalesce(dataset_type, ''),
		        label_completeness, total_size_bytes, status, uploaded_at, coalesce(tags, '{}')
	   FROM datasets d
	   LEFT JOIN users u ON u.id = d.owner_id
	  WHERE d.status = 'ready'
	  ORDER BY d.uploaded_at DESC
	  LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var datasets []DatasetSummary
	for rows.Next() {
		var ds DatasetSummary
		var ownerID sql.NullString
		if err := rows.Scan(&ds.ID, &ds.Name, &ds.ResearcherName, &ownerID, &ds.OwnerDisplayName, &ds.AISummary, &ds.Modality,
			&ds.DatasetType, &ds.LabelCompleteness, &ds.TotalSizeBytes, &ds.Status, &ds.UploadedAt,
			pq.Array(&ds.Tags)); err != nil {
			return nil, err
		}
		if ownerID.Valid {
			ds.OwnerID = &ownerID.String
		}
		datasets = append(datasets, ds)
	}
	if datasets == nil {
		datasets = []DatasetSummary{}
	}
	return datasets, rows.Err()
}

type Label struct {
	Name        string   `json:"name"`
	Proportion  *float64 `json:"proportion"`
	SampleCount int64    `json:"sample_count"`
}

type FileInfo struct {
	ID           int    `json:"id"`
	OriginalName string `json:"original_name"`
	SizeBytes    int64  `json:"size_bytes"`
	MimeType     string `json:"mime_type"`
	UploadStatus string `json:"upload_status"`
}

type DatasetDetail struct {
	ID                int                      `json:"id"`
	Name              string                   `json:"name"`
	ResearcherName    string                   `json:"researcher_name"`
	OwnerID           *string                  `json:"owner_id"`
	OwnerDisplayName  string                   `json:"owner_display_name"`
	Description       string                   `json:"description"`
	UploaderEmail     string                   `json:"uploader_email,omitempty"`
	AISummary         string                   `json:"ai_summary"`
	Modality          string                   `json:"modality"`
	DatasetType       string                   `json:"dataset_type"`
	AnnotationFormat  string                   `json:"annotation_format"`
	LabelCompleteness float64                  `json:"label_completeness"`
	AIConfidence      float64                  `json:"ai_confidence"`
	AICaveats         []string                 `json:"ai_caveats"`
	TotalSizeBytes    int64                    `json:"total_size_bytes"`
	Status            string                   `json:"status"`
	ProcessingStage   string                   `json:"processing_stage"`
	ExportStatus      string                   `json:"export_status"`
	ExportProgress    float64                  `json:"export_progress"`
	UploadedAt        time.Time                `json:"uploaded_at"`
	ErrorMessage      string                   `json:"error_message"`
	Tags              []string                 `json:"tags"`
	LabelColumn       string                   `json:"label_column,omitempty"`
	Labels            []Label                  `json:"labels"`
	LabelFields       []LabelField             `json:"label_fields"`
	PseudoQueries     []string                 `json:"pseudo_queries"`
	Files             []FileInfo               `json:"files"`
	Profile           *profiler.DatasetProfile `json:"profile"`
}

func (s *DatasetService) GetDataset(ctx context.Context, id int) (*DatasetDetail, error) {
	var ds DatasetDetail
	var profileJSON string
	var ownerID, labelColumn, uploaderEmail sql.NullString
	if err := s.db.QueryRowContext(ctx,
		`SELECT d.id, d.name, d.researcher_name,
		        d.owner_id::text, coalesce(u.display_name, ''),
		        coalesce(description, ''), coalesce(uploader_email, ''),
		        coalesce(ai_summary, ''),
		        coalesce(modality, ''), coalesce(dataset_type, ''), coalesce(annotation_format, ''),
		        label_completeness, ai_confidence, coalesce(ai_caveats, '{}'),
		        total_size_bytes, status,
		        coalesce(processing_stage, ''),
		        coalesce(export_status, 'none'), coalesce(export_progress, 0),
		        uploaded_at,
		        coalesce(error_message, ''), coalesce(tags, '{}'),
		        coalesce(profile_json::text, ''),
		        label_column
	   FROM datasets d
	   LEFT JOIN users u ON u.id = d.owner_id
	  WHERE d.id = $1`, id,
	).Scan(
		&ds.ID, &ds.Name, &ds.ResearcherName,
		&ownerID, &ds.OwnerDisplayName,
		&ds.Description, &uploaderEmail,
		&ds.AISummary,
		&ds.Modality, &ds.DatasetType, &ds.AnnotationFormat,
		&ds.LabelCompleteness, &ds.AIConfidence, pq.Array(&ds.AICaveats),
		&ds.TotalSizeBytes,
		&ds.Status, &ds.ProcessingStage,
		&ds.ExportStatus, &ds.ExportProgress,
		&ds.UploadedAt,
		&ds.ErrorMessage, pq.Array(&ds.Tags), &profileJSON,
		&labelColumn,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if ownerID.Valid {
		ds.OwnerID = &ownerID.String
	}
	if uploaderEmail.Valid {
		ds.UploaderEmail = uploaderEmail.String
	}
	if labelColumn.Valid {
		ds.LabelColumn = labelColumn.String
	}
	if profileJSON != "" {
		var profile profiler.DatasetProfile
		if err := json.Unmarshal([]byte(profileJSON), &profile); err == nil {
			ds.Profile = &profile
		}
	}
	ds.LabelFields = deriveLabelFields(ds.Profile)

	labelRows, err := s.db.QueryContext(ctx,
		`SELECT label_name, proportion, sample_count FROM labels WHERE dataset_id = $1 ORDER BY proportion DESC NULLS LAST`, id,
	)
	if err == nil {
		defer labelRows.Close()
		for labelRows.Next() {
			var l Label
			var prop sql.NullFloat64
			if err := labelRows.Scan(&l.Name, &prop, &l.SampleCount); err != nil {
				return nil, err
			}
			if prop.Valid {
				value := prop.Float64
				l.Proportion = &value
			}
			if field, ok := legacyLabelField(l, ds.LabelCompleteness); ok {
				ds.LabelFields = appendMissingLabelField(ds.LabelFields, field)
				continue
			}
			if !detailLabelShouldBeClass(l, ds.Profile, ds.LabelFields) {
				continue
			}
			ds.Labels = append(ds.Labels, l)
		}
	}
	if ds.Labels == nil {
		ds.Labels = []Label{}
	}
	if ds.LabelFields == nil {
		ds.LabelFields = []LabelField{}
	}
	if ds.AICaveats == nil {
		ds.AICaveats = []string{}
	}

	queryRows, err := s.db.QueryContext(ctx,
		`SELECT query_text FROM pseudo_queries WHERE dataset_id = $1`, id,
	)
	if err == nil {
		defer queryRows.Close()
		for queryRows.Next() {
			var qt string
			if err := queryRows.Scan(&qt); err != nil {
				return nil, err
			}
			ds.PseudoQueries = append(ds.PseudoQueries, qt)
		}
	}
	if ds.PseudoQueries == nil {
		ds.PseudoQueries = []string{}
	}

	fileRows, err := s.db.QueryContext(ctx,
		`SELECT id, original_name, size_bytes, coalesce(mime_type, ''), upload_status FROM files WHERE dataset_id = $1 ORDER BY id`, id,
	)
	if err == nil {
		defer fileRows.Close()
		for fileRows.Next() {
			var f FileInfo
			if err := fileRows.Scan(&f.ID, &f.OriginalName, &f.SizeBytes, &f.MimeType, &f.UploadStatus); err != nil {
				return nil, err
			}
			ds.Files = append(ds.Files, f)
		}
	}
	if ds.Files == nil {
		ds.Files = []FileInfo{}
	}

	return &ds, nil
}

type UpdateDatasetInput struct {
	ID              int
	Name            string
	ResearcherName  string
	UserDescription string
	Tags            []string
	LabelColumn     string
}

type DatasetFileCleanup struct {
	FileID      int
	StoragePath string
}

func (s *DatasetService) UpdateDataset(ctx context.Context, input UpdateDatasetInput, claims *auth.Claims) (*DatasetDetail, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.ResearcherName = strings.TrimSpace(input.ResearcherName)
	if input.Name == "" || input.ResearcherName == "" {
		return nil, fmt.Errorf("%w: name and researcher_name are required", ErrValidation)
	}
	if input.Tags == nil {
		input.Tags = []string{}
	}

	var ownerID sql.NullString
	var status string
	err := s.db.QueryRowContext(ctx,
		`SELECT owner_id::text, status FROM datasets WHERE id = $1`,
		input.ID,
	).Scan(&ownerID, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !auth.CanModifyOwner(claims, ownerID) {
		return nil, ErrForbidden
	}
	if status != "ready" && status != "error" {
		return nil, fmt.Errorf("%w: dataset metadata can only be edited after upload processing has finished", ErrConflict)
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE datasets
		    SET name = $1,
		        researcher_name = $2,
		        description = $3,
		        tags = $4,
		        label_column = NULLIF($5, ''),
		        updated_at = NOW()
		  WHERE id = $6
		    AND status IN ('ready', 'error')`,
		input.Name, input.ResearcherName, input.UserDescription, pq.Array(input.Tags),
		input.LabelColumn, input.ID,
	)
	if err != nil {
		return nil, err
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		return nil, fmt.Errorf("%w: dataset metadata can only be edited after upload processing has finished", ErrConflict)
	}
	if err := s.rebuildSearchVector(ctx, input.ID); err != nil {
		return nil, err
	}
	return s.GetDataset(ctx, input.ID)
}

func (s *DatasetService) DeleteDataset(ctx context.Context, datasetID int, claims *auth.Claims) ([]DatasetFileCleanup, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var ownerID sql.NullString
	err = tx.QueryRowContext(ctx,
		`SELECT owner_id::text FROM datasets WHERE id = $1 FOR UPDATE`,
		datasetID,
	).Scan(&ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if !auth.CanModifyOwner(claims, ownerID) {
		return nil, ErrForbidden
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT id, coalesce(storage_path, '')
		   FROM files
		  WHERE dataset_id = $1
		  ORDER BY id`,
		datasetID,
	)
	if err != nil {
		return nil, err
	}
	var cleanup []DatasetFileCleanup
	for rows.Next() {
		var item DatasetFileCleanup
		if err := rows.Scan(&item.FileID, &item.StoragePath); err != nil {
			rows.Close()
			return nil, err
		}
		cleanup = append(cleanup, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	res, err := tx.ExecContext(ctx, `DELETE FROM datasets WHERE id = $1`, datasetID)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return cleanup, nil
}

func (s *DatasetService) rebuildSearchVector(ctx context.Context, datasetID int) error {
	_, err := s.db.ExecContext(ctx, `
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
			SELECT string_agg(label_name, ' ') AS labels
			FROM labels
			WHERE dataset_id = $1
		) label_text,
		(
			SELECT string_agg(query_text, ' ') AS queries
			FROM pseudo_queries
			WHERE dataset_id = $1
		) query_text
		WHERE d.id = $1`,
		datasetID,
	)
	return err
}
