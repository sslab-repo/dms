package search

import "time"

// CitationSource identifies which retrieval method(s) found a result.
type CitationSource string

const (
	CitationKeyword  CitationSource = "keyword"
	CitationSemantic CitationSource = "semantic"
	CitationHybrid   CitationSource = "hybrid"
)

// SearchResult is a single dataset result returned to the API layer.
type SearchResult struct {
	DatasetID         int            `json:"dataset_id"`
	Name              string         `json:"name"`
	ResearcherName    string         `json:"researcher_name"`
	AISummary         string         `json:"ai_summary"`
	Modality          string         `json:"modality"`
	DatasetType       string         `json:"dataset_type"`
	AnnotationFormat  string         `json:"annotation_format"`
	LabelCompleteness float64        `json:"label_completeness"`
	TotalSizeBytes    int64          `json:"total_size_bytes"`
	Tags              []string       `json:"tags"`
	FusionScore       float64        `json:"fusion_score"`
	KeywordScore      float64        `json:"keyword_score,omitempty"`
	SemanticScore     float64        `json:"semantic_score,omitempty"`
	Citation          CitationSource `json:"citation"`
	UploadedAt        time.Time      `json:"uploaded_at"`
}

// SearchFilters holds the optional filter parameters from the frontend.
type SearchFilters struct {
	Modality         string
	DatasetType      string
	AnnotationFormat string
	MinSizeBytes     int64
	MaxSizeBytes     int64
	MinLabelComplete float64
	MaxLabelComplete float64
	UploadedAfter    time.Time
	UploadedBefore   time.Time
}
