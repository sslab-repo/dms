package mlexport

import "time"

const ManifestVersion = "dms-ml-package-v1"

// Manifest is the machine-readable description of the package
// (written to manifest.json at the package root).
type Manifest struct {
	ManifestVersion string          `json:"manifest_version"`
	GeneratedAt     time.Time       `json:"generated_at"`
	Dataset         ManifestDataset `json:"dataset"`
	Counts          ManifestCounts  `json:"counts"`
	Schema          ManifestSchema  `json:"schema"`
	Labels          []LabelDef      `json:"labels"`
	Split           ManifestSplit   `json:"split"`
	Files           []ManifestFile  `json:"files"`
	// Checksums maps package-relative paths (raw/* and processed/*) to
	// "sha256:<hex>" digests.
	Checksums map[string]string `json:"checksums"`
}

type ManifestDataset struct {
	ID                int       `json:"dms_dataset_id"`
	Name              string    `json:"name"`
	Researcher        string    `json:"researcher"`
	UploadedAt        time.Time `json:"uploaded_at"`
	Modality          string    `json:"modality,omitempty"`
	DatasetType       string    `json:"dataset_type,omitempty"`
	AnnotationFormat  string    `json:"annotation_format,omitempty"`
	LabelCompleteness float64   `json:"label_completeness"`
	Tags              []string  `json:"tags"`
}

type ManifestCounts struct {
	Files   int         `json:"files"`
	Samples SplitCounts `json:"samples"`
}

type ManifestSchema struct {
	// Mode is "tabular" (processed/*.parquet) or "files" (processed/*.jsonl
	// referencing raw/).
	Mode     string           `json:"mode"`
	IDColumn string           `json:"id_column,omitempty"`
	Columns  []ManifestColumn `json:"columns,omitempty"`
	// Note documents the string-typed storage decision for consumers.
	Note string `json:"note,omitempty"`
}

type ManifestColumn struct {
	Name string `json:"name"`
	// StoredType is always "string" in v1; InferredType is the profiler's
	// best guess and is advisory.
	StoredType   string `json:"stored_type"`
	InferredType string `json:"inferred_type,omitempty"`
}

type ManifestSplit struct {
	Method   string             `json:"method"` // "hash" | "provided"
	Seed     int                `json:"seed"`
	Ratios   map[string]float64 `json:"ratios,omitempty"`
	IDScheme string             `json:"id_scheme"`
	File     string             `json:"file"`
}

type ManifestFile struct {
	Name         string `json:"name"` // package name under raw/
	OriginalName string `json:"original_name"`
	Path         string `json:"path"` // package-relative, e.g. "raw/train.csv"
	SizeBytes    int64  `json:"size_bytes"`
	Sha256       string `json:"sha256"`
	DetectedType string `json:"detected_type,omitempty"`
	Role         string `json:"role,omitempty"`
}
