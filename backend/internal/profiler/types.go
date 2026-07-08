package profiler

import "time"

const ProfileVersion = "dataset-profile-v1"

type DatasetProfile struct {
	Version          string              `json:"version"`
	GeneratedAt      time.Time           `json:"generated_at"`
	TotalFiles       int                 `json:"total_files"`
	TotalSizeBytes   int64               `json:"total_size_bytes"`
	FileTypes        []TypeSummary       `json:"file_types"`
	Groups           []FileGroup         `json:"groups"`
	Files            []FileProfile       `json:"files"`
	Annotations      []AnnotationProfile `json:"annotations,omitempty"`
	Notes            []string            `json:"notes"`
	DetectedPatterns []string            `json:"detected_patterns"`
}

type TypeSummary struct {
	DetectedType   string `json:"detected_type"`
	FileCount      int    `json:"file_count"`
	TotalSizeBytes int64  `json:"total_size_bytes"`
}

type FileGroup struct {
	Key                    string          `json:"key"`
	DetectedType           string          `json:"detected_type"`
	Role                   string          `json:"role"`
	FileCount              int             `json:"file_count"`
	TotalSizeBytes         int64           `json:"total_size_bytes"`
	SharedColumns          []ColumnProfile `json:"shared_columns,omitempty"`
	AllColumnNames         []string        `json:"all_column_names,omitempty"`
	RepresentativeFileIDs  []int           `json:"representative_file_ids"`
	RepresentativeExamples []FileProfile   `json:"representative_examples"`
}

type FileProfile struct {
	FileID         int                 `json:"file_id"`
	OriginalName   string              `json:"original_name"`
	Extension      string              `json:"extension"`
	SizeBytes      int64               `json:"size_bytes"`
	MimeType       string              `json:"mime_type,omitempty"`
	DetectedType   string              `json:"detected_type"`
	Role           string              `json:"role"`
	SampledRows    int                 `json:"sampled_rows,omitempty"`
	Columns        []ColumnProfile     `json:"columns,omitempty"`
	AllColumnNames []string            `json:"all_column_names,omitempty"`
	SampleRows     []map[string]string `json:"sample_rows,omitempty"`
	SampleText     []string            `json:"sample_text,omitempty"`
	Annotation     *AnnotationProfile  `json:"annotation,omitempty"`
	Warnings       []string            `json:"warnings,omitempty"`
}

type ColumnProfile struct {
	Name          string       `json:"name"`
	InferredType  string       `json:"inferred_type"`
	NonEmptyCount int          `json:"non_empty_count"`
	EmptyCount    int          `json:"empty_count"`
	ExampleValues []string     `json:"example_values,omitempty"`
	TopValues     []ValueCount `json:"top_values,omitempty"` // populated for low-cardinality columns
}

// ValueCount is a single value and how many times it appeared in the scanned rows.
type ValueCount struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

type AnnotationProfile struct {
	Format           string         `json:"format"`
	SourceFiles      []string       `json:"source_files"`
	ClassCount       int            `json:"class_count"`
	TotalAnnotations int64          `json:"total_annotations"`
	Classes          []ClassProfile `json:"classes,omitempty"`
	Notes            []string       `json:"notes,omitempty"`
}

type ClassProfile struct {
	ID         string  `json:"id,omitempty"`
	Name       string  `json:"name"`
	Count      int64   `json:"count"`
	Proportion float64 `json:"proportion,omitempty"`
}
