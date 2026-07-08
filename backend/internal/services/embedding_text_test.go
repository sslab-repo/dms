package services

import (
	"strings"
	"testing"

	"dataset-platform/backend/internal/ai"
	"dataset-platform/backend/internal/profiler"
)

func TestBuildEmbeddingTextUsesSemanticEvidenceOnly(t *testing.T) {
	analysis := &ai.DatasetAnalysis{
		Summary:          "Natural language summary about cancer cell nuclei.",
		Modality:         "tabular",
		DatasetType:      "supervised",
		AnnotationFormat: "CSV",
		Labels:           []ai.Label{{Name: "benign"}, {Name: "malignant"}},
		PseudoQueries:    []string{"images of cell nuclei for cancer detection"},
	}
	profile := &profiler.DatasetProfile{
		FileTypes:        []profiler.TypeSummary{{DetectedType: "csv", FileCount: 1}},
		DetectedPatterns: []string{"train split detected"},
		Groups: []profiler.FileGroup{
			{
				Role:         "data",
				DetectedType: "csv",
				FileCount:    1,
				SharedColumns: []profiler.ColumnProfile{
					{Name: "diagnosis", InferredType: "string", ExampleValues: []string{"benign", "malignant"}},
					{Name: "radius_mean", InferredType: "number", ExampleValues: []string{"12.3"}},
				},
				RepresentativeExamples: []profiler.FileProfile{
					{
						SampleRows: []map[string]string{
							{"diagnosis": "benign", "radius_mean": "12.3"},
						},
					},
				},
			},
		},
	}

	text := buildEmbeddingText(analysis, profile, "Researcher-provided context about pathology records.")

	for _, want := range []string{
		"Natural language summary",
		"Researcher-provided context",
		"Search query: images of cell nuclei",
		"Label/class: benign",
		"Column: diagnosis string examples: benign, malignant",
		"Sample row: diagnosis=benign",
		"Detected pattern: train split detected",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected embedding text to contain %q:\n%s", want, text)
		}
	}

	for _, notWant := range []string{
		"Modality: tabular",
		"Dataset type: supervised",
		"Annotation format: CSV",
	} {
		if strings.Contains(text, notWant) {
			t.Fatalf("expected structured keyword-only metadata %q to be absent:\n%s", notWant, text)
		}
	}
}
