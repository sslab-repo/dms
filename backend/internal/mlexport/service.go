package mlexport

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"dataset-platform/backend/internal/profiler"

	"github.com/lib/pq"
)

// buildTimeout bounds one package build. Generous because large datasets are
// hashed, converted, and zipped in full.
const buildTimeout = 60 * time.Minute

// Service loads PackageSpecs from PostgreSQL, runs the Builder, and tracks
// lifecycle state in datasets.export_status / export_progress.
type Service struct {
	db      *sql.DB
	rootDir string // FileStorageDir; exports live under <rootDir>/exports/<datasetID>/
}

func NewService(db *sql.DB, fileStorageDir string) *Service {
	return &Service{db: db, rootDir: fileStorageDir}
}

func (s *Service) exportDir(datasetID int) string {
	return filepath.Join(s.rootDir, "exports", strconv.Itoa(datasetID))
}

// ExportInfo is the current package state for one dataset.
type ExportInfo struct {
	Status   string  // none | building | ready | error
	Progress float64 // 0.0 - 1.0
	Path     string  // zip location when Status == "ready"
	Error    string
	ZipName  string // suggested download filename
}

func (s *Service) Info(ctx context.Context, datasetID int) (*ExportInfo, error) {
	var info ExportInfo
	var name string
	var path, errMsg sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT name, coalesce(export_status, 'none'), coalesce(export_progress, 0),
		        export_path, export_error
		   FROM datasets WHERE id = $1`, datasetID,
	).Scan(&name, &info.Status, &info.Progress, &path, &errMsg)
	if err != nil {
		return nil, err
	}
	info.Path = path.String
	info.Error = errMsg.String
	info.ZipName = Slugify(name) + "-ml-package.zip"
	return &info, nil
}

// RecoverInterrupted marks datasets stuck in 'building' (e.g. after a server
// restart mid-build) as errored so they can be rebuilt on demand.
// Call once at startup.
func (s *Service) RecoverInterrupted(ctx context.Context) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE datasets
		    SET export_status = 'error',
		        export_error  = 'export interrupted by server restart'
		  WHERE export_status = 'building'`)
	if err != nil {
		fmt.Printf("[MLExport] startup recovery failed: %v\n", err)
		return
	}
	if n, _ := res.RowsAffected(); n > 0 {
		fmt.Printf("[MLExport] marked %d interrupted export(s) as error\n", n)
	}
}

// Reset clears export state and deletes any package on disk. The pipeline
// calls this when a dataset is reprocessed so a stale zip is never served.
func (s *Service) Reset(ctx context.Context, datasetID int) {
	if _, err := s.db.ExecContext(ctx,
		`UPDATE datasets
		    SET export_status = 'none', export_progress = 0,
		        export_path = NULL, export_error = NULL
		  WHERE id = $1`, datasetID,
	); err != nil {
		fmt.Printf("[MLExport] dataset %d: reset failed: %v\n", datasetID, err)
	}
	s.Cleanup(datasetID)
}

// Cleanup removes the export directory (used on dataset deletion).
func (s *Service) Cleanup(datasetID int) {
	if err := os.RemoveAll(s.exportDir(datasetID)); err != nil {
		fmt.Printf("[MLExport] dataset %d: cleanup failed: %v\n", datasetID, err)
	}
}

// StartIfNeeded kicks off an asynchronous build when the dataset is ready
// but has no package yet ('none', or a previous 'error'). Returns true when
// a build was started. Safe to call on every detail-page view: the atomic
// UPDATE guarantees only one builder runs per dataset.
func (s *Service) StartIfNeeded(datasetID int) bool {
	res, err := s.db.Exec(
		`UPDATE datasets
		    SET export_status = 'building', export_progress = 0, export_error = NULL
		  WHERE id = $1 AND status = 'ready' AND export_status IN ('none', 'error')`,
		datasetID)
	if err != nil {
		fmt.Printf("[MLExport] dataset %d: start check failed: %v\n", datasetID, err)
		return false
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false
	}
	go s.BuildNow(datasetID)
	return true
}

// BuildNow builds the package synchronously. The caller must already have
// set export_status='building' (the pipeline does this atomically together
// with status='ready'; StartIfNeeded does it with a guarded UPDATE).
func (s *Service) BuildNow(datasetID int) {
	ctx, cancel := context.WithTimeout(context.Background(), buildTimeout)
	defer cancel()
	defer func() {
		if r := recover(); r != nil {
			s.fail(datasetID, fmt.Sprintf("export recovered from internal error: %v", r))
		}
	}()

	start := time.Now()
	spec, err := s.loadSpec(ctx, datasetID)
	if err != nil {
		s.fail(datasetID, fmt.Sprintf("load dataset for export: %v", err))
		return
	}

	dir := s.exportDir(datasetID)
	stagingDir := filepath.Join(dir, "staging")
	if err := os.RemoveAll(dir); err != nil {
		s.fail(datasetID, fmt.Sprintf("clean export dir: %v", err))
		return
	}
	if err := os.MkdirAll(stagingDir, 0755); err != nil {
		s.fail(datasetID, fmt.Sprintf("create export dir: %v", err))
		return
	}

	zipPath := filepath.Join(dir, spec.Slug+"-ml-package.zip")
	builder := &Builder{Progress: s.progressWriter(datasetID)}
	result, err := builder.Build(ctx, spec, stagingDir, zipPath+".tmp")
	if err != nil {
		os.RemoveAll(dir)
		s.fail(datasetID, fmt.Sprintf("build package: %v", err))
		return
	}
	if err := os.Rename(zipPath+".tmp", zipPath); err != nil {
		os.RemoveAll(dir)
		s.fail(datasetID, fmt.Sprintf("finalize package: %v", err))
		return
	}
	os.RemoveAll(stagingDir)

	for fileID, sum := range result.RawChecksums {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE files SET sha256 = $1 WHERE id = $2`, sum, fileID); err != nil {
			fmt.Printf("[MLExport] dataset %d: warning — sha256 update for file %d failed: %v\n",
				datasetID, fileID, err)
		}
	}

	if _, err := s.db.ExecContext(ctx,
		`UPDATE datasets
		    SET export_status = 'ready', export_progress = 1,
		        export_path = $1, export_error = NULL, export_built_at = NOW()
		  WHERE id = $2`, zipPath, datasetID,
	); err != nil {
		s.fail(datasetID, fmt.Sprintf("record finished package: %v", err))
		return
	}
	fmt.Printf("[MLExport] dataset %d: package READY (%s mode, %d samples, %s) in %s\n",
		datasetID, result.Mode, result.Counts.Total, zipPath, time.Since(start).Round(time.Second))
}

func (s *Service) fail(datasetID int, msg string) {
	fmt.Printf("[MLExport] dataset %d ERROR: %s\n", datasetID, msg)
	if _, err := s.db.Exec(
		`UPDATE datasets SET export_status = 'error', export_error = $1 WHERE id = $2`,
		msg, datasetID); err != nil {
		fmt.Printf("[MLExport] dataset %d: could not record error: %v\n", datasetID, err)
	}
}

// progressWriter persists progress, throttled to meaningful changes so the
// builder can report per-read without hammering the database.
func (s *Service) progressWriter(datasetID int) func(float64) {
	var lastValue float64
	var lastWrite time.Time
	return func(f float64) {
		now := time.Now()
		if f < 1 && f-lastValue < 0.02 && now.Sub(lastWrite) < time.Second {
			return
		}
		lastValue, lastWrite = f, now
		if _, err := s.db.Exec(
			`UPDATE datasets SET export_progress = $1 WHERE id = $2`, f, datasetID); err != nil {
			fmt.Printf("[MLExport] dataset %d: progress update failed: %v\n", datasetID, err)
		}
	}
}

// loadSpec assembles the PackageSpec from the datasets, labels, and files
// tables plus the stored profiler output (for per-file roles/types and
// column type hints).
func (s *Service) loadSpec(ctx context.Context, datasetID int) (*PackageSpec, error) {
	spec := &PackageSpec{DatasetID: datasetID, InferredColumnTypes: map[string]string{}}
	var profileJSON string
	if err := s.db.QueryRowContext(ctx,
		`SELECT name, researcher_name, coalesce(description, ''), coalesce(ai_summary, ''),
		        coalesce(modality, ''), coalesce(dataset_type, ''), coalesce(annotation_format, ''),
		        label_completeness, coalesce(ai_caveats, '{}'), coalesce(tags, '{}'),
		        uploaded_at, coalesce(profile_json::text, '')
		   FROM datasets WHERE id = $1`, datasetID,
	).Scan(&spec.Name, &spec.ResearcherName, &spec.Description, &spec.AISummary,
		&spec.Modality, &spec.DatasetType, &spec.AnnotationFormat,
		&spec.LabelCompleteness, pq.Array(&spec.AICaveats), pq.Array(&spec.Tags),
		&spec.UploadedAt, &profileJSON,
	); err != nil {
		return nil, err
	}
	spec.Slug = Slugify(spec.Name)

	labelRows, err := s.db.QueryContext(ctx,
		`SELECT label_name, proportion, sample_count
		   FROM labels WHERE dataset_id = $1
		  ORDER BY proportion DESC NULLS LAST, label_name`, datasetID)
	if err != nil {
		return nil, err
	}
	defer labelRows.Close()
	for labelRows.Next() {
		var l LabelDef
		var prop sql.NullFloat64
		if err := labelRows.Scan(&l.Name, &prop, &l.SampleCount); err != nil {
			return nil, err
		}
		if prop.Valid {
			v := prop.Float64
			l.Proportion = &v
		}
		spec.Labels = append(spec.Labels, l)
	}
	if err := labelRows.Err(); err != nil {
		return nil, err
	}

	fileRows, err := s.db.QueryContext(ctx,
		`SELECT id, original_name, storage_path, size_bytes
		   FROM files
		  WHERE dataset_id = $1 AND upload_status = 'complete'
		  ORDER BY id`, datasetID)
	if err != nil {
		return nil, err
	}
	defer fileRows.Close()
	for fileRows.Next() {
		var f SourceFile
		if err := fileRows.Scan(&f.FileID, &f.OriginalName, &f.StoragePath, &f.SizeBytes); err != nil {
			return nil, err
		}
		f.Role = "data" // overwritten below when the profile knows better
		spec.Files = append(spec.Files, f)
	}
	if err := fileRows.Err(); err != nil {
		return nil, err
	}
	if len(spec.Files) == 0 {
		return nil, fmt.Errorf("dataset has no complete files")
	}

	if profileJSON != "" {
		var profile profiler.DatasetProfile
		if err := json.Unmarshal([]byte(profileJSON), &profile); err == nil {
			applyProfile(spec, &profile)
		}
	}
	return spec, nil
}

// applyProfile copies per-file roles/detected types and column type hints
// from the stored profiler output onto the spec.
func applyProfile(spec *PackageSpec, profile *profiler.DatasetProfile) {
	byID := map[int]*profiler.FileProfile{}
	for i := range profile.Files {
		byID[profile.Files[i].FileID] = &profile.Files[i]
	}
	for i := range spec.Files {
		fp, ok := byID[spec.Files[i].FileID]
		if !ok {
			continue
		}
		spec.Files[i].DetectedType = fp.DetectedType
		if fp.Role != "" {
			spec.Files[i].Role = fp.Role
		}
		for _, col := range fp.Columns {
			if col.InferredType != "" {
				spec.InferredColumnTypes[col.Name] = col.InferredType
			}
		}
	}
	for _, g := range profile.Groups {
		for _, col := range g.SharedColumns {
			if col.InferredType != "" {
				spec.InferredColumnTypes[col.Name] = col.InferredType
			}
		}
	}
}
