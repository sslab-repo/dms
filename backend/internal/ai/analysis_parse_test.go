package ai

import (
	"encoding/json"
	"testing"
)

func TestParseDatasetAnalysisPreservesNullLabelProportion(t *testing.T) {
	raw := `{
		"summary": "This dataset contains text examples for classification. It includes a visible label column. The uploader can use it for supervised experiments. The data may support text modeling. Uploaded by Rudra.",
		"labels": [{"name": "human", "proportion": null, "sample_count": -1}],
		"label_completeness": 1.0,
		"modality": "text",
		"dataset_type": "supervised",
		"annotation_format": "CSV",
		"pseudo_queries": [],
		"confidence": 0.8,
		"caveats": []
	}`

	analysis, err := parseDatasetAnalysis(raw)
	if err != nil {
		t.Fatalf("parseDatasetAnalysis returned error: %v", err)
	}
	if len(analysis.Labels) != 1 {
		t.Fatalf("expected 1 label, got %d", len(analysis.Labels))
	}
	if analysis.Labels[0].Proportion != nil {
		t.Fatalf("expected nil proportion, got %v", *analysis.Labels[0].Proportion)
	}

	body, err := json.Marshal(analysis.Labels[0])
	if err != nil {
		t.Fatalf("marshal label: %v", err)
	}
	if string(body) != `{"name":"human","proportion":null,"sample_count":-1}` {
		t.Fatalf("unexpected marshaled label: %s", string(body))
	}
}

func TestParseDatasetAnalysisTracksMissingMetadataFields(t *testing.T) {
	raw := `{
		"summary": "This dataset has enough summary text to parse.",
		"labels": [],
		"pseudo_queries": [],
		"caveats": []
	}`

	analysis, err := parseDatasetAnalysis(raw)
	if err != nil {
		t.Fatalf("parseDatasetAnalysis returned error: %v", err)
	}

	want := map[string]bool{
		"label_completeness": true,
		"modality":           true,
		"dataset_type":       true,
		"annotation_format":  true,
	}
	if len(analysis.MissingMetadataFields) != len(want) {
		t.Fatalf("expected missing metadata fields %v, got %v", want, analysis.MissingMetadataFields)
	}
	for _, field := range analysis.MissingMetadataFields {
		if !want[field] {
			t.Fatalf("unexpected missing metadata field %q", field)
		}
	}
}

func TestRepairJSONAllowsPartialAnalysisToParseWithMissingFields(t *testing.T) {
	raw := `{
		"summary": "The dataset contains movie metadata.",
		"labels": [{"name": "Genre", "proportion": null, "sample_count": 5}],
		"label_completeness": 0.0`

	analysis, err := parseDatasetAnalysis(repairJSON(raw))
	if err != nil {
		t.Fatalf("parseDatasetAnalysis returned error after repair: %v", err)
	}
	if analysis.ParseRecovery != "" {
		t.Fatalf("parseDatasetAnalysis should not set recovery mode directly, got %q", analysis.ParseRecovery)
	}
	for _, field := range []string{"modality", "dataset_type", "annotation_format"} {
		found := false
		for _, missing := range analysis.MissingMetadataFields {
			if missing == field {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected repaired partial response to mark %q missing, got %v", field, analysis.MissingMetadataFields)
		}
	}
}
