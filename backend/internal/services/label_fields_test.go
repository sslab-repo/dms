package services

import (
	"testing"

	"dataset-platform/backend/internal/ai"
	"dataset-platform/backend/internal/profiler"
)

func TestDeriveLabelFieldsFromProfile(t *testing.T) {
	profile := &profiler.DatasetProfile{
		Groups: []profiler.FileGroup{
			{
				SharedColumns: []profiler.ColumnProfile{
					{Name: "text", InferredType: "string", NonEmptyCount: 5},
					{Name: "label", InferredType: "integer", NonEmptyCount: 5, ExampleValues: []string{"0", "1"}},
				},
			},
		},
	}

	fields := deriveLabelFields(profile)
	if len(fields) != 1 {
		t.Fatalf("expected 1 label field, got %d", len(fields))
	}
	if fields[0].Name != "label" {
		t.Fatalf("expected label field named label, got %q", fields[0].Name)
	}
	if fields[0].Completeness != 1.0 {
		t.Fatalf("expected completeness 1.0, got %f", fields[0].Completeness)
	}
}

func TestLegacyLabelRowBecomesFieldEvidence(t *testing.T) {
	zero := 0.0
	label := Label{
		Name:        "label",
		Proportion:  &zero,
		SampleCount: 5,
	}

	field, ok := legacyLabelField(label, 1.0)
	if !ok {
		t.Fatal("expected legacy label row to be treated as label-field evidence")
	}
	if field.Name != "label" || field.NonEmptyCount != 5 || field.Completeness != 1.0 {
		t.Fatalf("unexpected field evidence: %+v", field)
	}
}

func TestLegacyClassDistributionStaysLabel(t *testing.T) {
	prop := 0.53
	label := Label{
		Name:        "human",
		Proportion:  &prop,
		SampleCount: 100,
	}

	if _, ok := legacyLabelField(label, 1.0); ok {
		t.Fatal("expected real class distribution to stay in labels")
	}
}

func TestDetailLabelShouldDropInferredEntityWithoutDistributionEvidence(t *testing.T) {
	label := Label{Name: "entity_group", SampleCount: 5}

	if detailLabelShouldBeClass(label, &profiler.DatasetProfile{}, nil) {
		t.Fatal("expected inferred entity label with no distribution evidence to be hidden")
	}
}

func TestDetailLabelShouldDropProfileFeatureColumn(t *testing.T) {
	label := Label{Name: "measurement_value", SampleCount: 12}
	profile := &profiler.DatasetProfile{
		Groups: []profiler.FileGroup{
			{
				SharedColumns: []profiler.ColumnProfile{
					{Name: "measurement_value", InferredType: "number", NonEmptyCount: 12},
				},
			},
		},
	}

	if detailLabelShouldBeClass(label, profile, nil) {
		t.Fatal("expected profile feature column to be hidden from class labels")
	}
}

func TestReconcileLabelMetadataUsesDetectedLabelField(t *testing.T) {
	analysis := &ai.DatasetAnalysis{
		DatasetType:       "unsupervised",
		LabelCompleteness: 0,
		Labels: []ai.Label{
			{Name: "label", SampleCount: 5},
		},
	}
	profile := &profiler.DatasetProfile{
		Groups: []profiler.FileGroup{
			{
				SharedColumns: []profiler.ColumnProfile{
					{Name: "text", InferredType: "string", NonEmptyCount: 5},
					{Name: "label", InferredType: "integer", NonEmptyCount: 5, ExampleValues: []string{"0", "1"}},
				},
			},
		},
	}

	reconcileLabelMetadata(analysis, profile)

	if analysis.DatasetType != "supervised" {
		t.Fatalf("expected dataset type supervised, got %q", analysis.DatasetType)
	}
	if analysis.LabelCompleteness != 1.0 {
		t.Fatalf("expected label completeness 1.0, got %f", analysis.LabelCompleteness)
	}
	if len(analysis.Labels) != 0 {
		t.Fatalf("expected field-name label to be removed from class labels, got %+v", analysis.Labels)
	}
}

func TestReconcileLabelMetadataDowngradesSupervisedGuessWithoutTargetEvidence(t *testing.T) {
	analysis := &ai.DatasetAnalysis{
		DatasetType:       "supervised",
		LabelCompleteness: 0.8,
		Labels: []ai.Label{
			{Name: "descriptive_group", SampleCount: 5},
		},
	}
	profile := &profiler.DatasetProfile{
		Groups: []profiler.FileGroup{
			{
				SharedColumns: []profiler.ColumnProfile{
					{Name: "descriptive_group", InferredType: "string", NonEmptyCount: 5},
					{Name: "event_flag", InferredType: "number", NonEmptyCount: 5, ExampleValues: []string{"0", "1"}},
					{Name: "measurement_value", InferredType: "number", NonEmptyCount: 4, EmptyCount: 1},
				},
			},
		},
	}

	reconcileLabelMetadata(analysis, profile)

	if analysis.DatasetType != "unsupervised" {
		t.Fatalf("expected conservative dataset type unsupervised, got %q", analysis.DatasetType)
	}
	if analysis.LabelCompleteness != 0 {
		t.Fatalf("expected label completeness reset to 0, got %f", analysis.LabelCompleteness)
	}
	if len(analysis.Labels) != 0 {
		t.Fatalf("expected ordinary data-field label to be removed, got %+v", analysis.Labels)
	}
}

func TestReconcileLabelMetadataResolvesUnknownAnalyzableProfileToUnsupervised(t *testing.T) {
	analysis := &ai.DatasetAnalysis{
		DatasetType:       "unknown",
		LabelCompleteness: 0,
	}
	profile := &profiler.DatasetProfile{
		Groups: []profiler.FileGroup{
			{
				DetectedType: "csv",
				SharedColumns: []profiler.ColumnProfile{
					{Name: "title", InferredType: "string", NonEmptyCount: 5},
					{Name: "rating", InferredType: "number", NonEmptyCount: 5},
					{Name: "genre", InferredType: "string", NonEmptyCount: 5},
				},
			},
		},
	}

	reconcileLabelMetadata(analysis, profile)

	if analysis.DatasetType != "unsupervised" {
		t.Fatalf("expected analyzable profile without label evidence to become unsupervised, got %q", analysis.DatasetType)
	}
}

func TestReconcileLabelMetadataKeepsSparseUnsupportedProfileUnknown(t *testing.T) {
	analysis := &ai.DatasetAnalysis{
		DatasetType:       "unknown",
		LabelCompleteness: 0,
	}
	profile := &profiler.DatasetProfile{
		FileTypes: []profiler.TypeSummary{{DetectedType: "blob", FileCount: 1}},
	}

	reconcileLabelMetadata(analysis, profile)

	if analysis.DatasetType != "unknown" {
		t.Fatalf("expected sparse unsupported profile to stay unknown, got %q", analysis.DatasetType)
	}
}

func TestConservativeTargetNamesDoNotIncludeGenericFeatureNames(t *testing.T) {
	for _, name := range []string{"event_flag", "status_code", "score_value", "measurement_value", "descriptive_group"} {
		if isTargetLikeColumnName(name) {
			t.Fatalf("expected %q to require explicit user target intent", name)
		}
	}
}
