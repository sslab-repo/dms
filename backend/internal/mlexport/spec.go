// Package mlexport builds the downloadable "ML package" for a dataset:
// a structured, ML-ready snapshot containing a README datasheet, a manifest
// with checksums, the untouched raw files, processed train/val/test splits,
// an explicit split file, and a deterministic rebuild script — all zipped.
//
// The Builder is database-free and works from a PackageSpec so it can be
// tested in isolation; the Service loads the spec from PostgreSQL, tracks
// progress in datasets.export_progress, and owns the on-disk export layout.
package mlexport

import "time"

// PackageSpec is everything the builder needs to produce an ML package.
type PackageSpec struct {
	DatasetID         int
	Name              string
	Slug              string // filesystem/zip-safe name derived from Name
	ResearcherName    string
	Description       string // uploader-written description
	AISummary         string
	Modality          string
	DatasetType       string
	AnnotationFormat  string
	LabelCompleteness float64
	AICaveats         []string
	Tags              []string
	UploadedAt        time.Time

	Labels []LabelDef
	Files  []SourceFile

	// InferredColumnTypes maps column name -> profiler-inferred type for
	// whatever columns the profiler sampled. Advisory only: the package
	// stores every processed value as a string and records that here.
	InferredColumnTypes map[string]string
}

// LabelDef mirrors one row of the labels table.
type LabelDef struct {
	Name        string   `json:"name"`
	Proportion  *float64 `json:"proportion,omitempty"`
	SampleCount int64    `json:"sample_count"`
}

// SourceFile is one assembled file of the dataset plus what the profiler
// concluded about it. Role and DetectedType use the profiler vocabulary
// (role: data | annotations | train-split | validation-split | test-split |
// documentation | configuration | model-artifact | instruction-data).
type SourceFile struct {
	FileID       int
	OriginalName string
	StoragePath  string
	SizeBytes    int64
	DetectedType string
	Role         string

	// zipName is the collision-free name this file gets under raw/.
	// Assigned by the builder before any stage runs.
	zipName string
}

// BuildResult reports what the builder produced.
type BuildResult struct {
	ZipPath      string
	Mode         string // "tabular" | "files"
	Counts       SplitCounts
	RawChecksums map[int]string // file ID -> hex sha256, for persisting to files.sha256
}

// SplitCounts is the number of samples that landed in each split.
type SplitCounts struct {
	Train int64 `json:"train"`
	Val   int64 `json:"val"`
	Test  int64 `json:"test"`
	Total int64 `json:"total"`
}
