package services

import (
	"testing"

	"dataset-platform/backend/internal/ai"
	"dataset-platform/backend/internal/profiler"
)

func TestApplyProfileCorrectionsKeepsValidAIModality(t *testing.T) {
	analysis := &ai.DatasetAnalysis{
		Modality:         "text",
		DatasetType:      "supervised",
		AnnotationFormat: "none",
	}
	profile := &profiler.DatasetProfile{
		FileTypes: []profiler.TypeSummary{{DetectedType: "csv", FileCount: 1}},
	}

	applyProfileCorrections(analysis, profile)

	if analysis.Modality != "text" {
		t.Fatalf("expected AI modality to be preserved, got %q", analysis.Modality)
	}
	if analysis.AnnotationFormat != "CSV" {
		t.Fatalf("expected profiler-backed format CSV, got %q", analysis.AnnotationFormat)
	}
}

func TestApplyProfileCorrectionsKeepsValidTabularModality(t *testing.T) {
	analysis := &ai.DatasetAnalysis{
		Modality:         "tabular",
		DatasetType:      "unsupervised",
		AnnotationFormat: "unknown",
	}
	profile := &profiler.DatasetProfile{
		FileTypes: []profiler.TypeSummary{{DetectedType: "csv", FileCount: 1}},
	}

	applyProfileCorrections(analysis, profile)

	if analysis.Modality != "tabular" {
		t.Fatalf("expected valid AI modality tabular to be preserved, got %q", analysis.Modality)
	}
	if analysis.AnnotationFormat != "CSV" {
		t.Fatalf("expected profiler-backed format CSV, got %q", analysis.AnnotationFormat)
	}
}

func TestApplyProfileCorrectionsFallsBackForUnknownModality(t *testing.T) {
	analysis := &ai.DatasetAnalysis{
		Modality:         "unknown",
		DatasetType:      "supervised",
		AnnotationFormat: "none",
	}
	profile := &profiler.DatasetProfile{
		FileTypes: []profiler.TypeSummary{{DetectedType: "csv", FileCount: 1}},
	}

	applyProfileCorrections(analysis, profile)

	if analysis.Modality != "tabular" {
		t.Fatalf("expected profiler-backed modality tabular, got %q", analysis.Modality)
	}
	if analysis.AnnotationFormat != "CSV" {
		t.Fatalf("expected profiler-backed format CSV, got %q", analysis.AnnotationFormat)
	}
}

func TestApplyProfileCorrectionsFallsBackForEmptyOrInvalidModality(t *testing.T) {
	for _, modality := range []string{"", "csv", "structured"} {
		analysis := &ai.DatasetAnalysis{
			Modality:         modality,
			DatasetType:      "unsupervised",
			AnnotationFormat: "unknown",
		}
		profile := &profiler.DatasetProfile{
			FileTypes: []profiler.TypeSummary{{DetectedType: "csv", FileCount: 1}},
		}

		applyProfileCorrections(analysis, profile)

		if analysis.Modality != "tabular" {
			t.Fatalf("expected modality %q to fall back to tabular, got %q", modality, analysis.Modality)
		}
	}
}

func TestRecoveredMetadataCaveatAddedForMissingFields(t *testing.T) {
	analysis := &ai.DatasetAnalysis{
		Modality:              "unknown",
		DatasetType:           "unknown",
		AnnotationFormat:      "unknown",
		MissingMetadataFields: []string{"modality", "dataset_type", "annotation_format"},
	}
	profile := &profiler.DatasetProfile{
		FileTypes: []profiler.TypeSummary{{DetectedType: "csv", FileCount: 1}},
		Groups: []profiler.FileGroup{
			{DetectedType: "csv", SharedColumns: []profiler.ColumnProfile{{Name: "title", InferredType: "string"}}},
		},
	}

	applyProfileCorrections(analysis, profile)
	reconcileLabelMetadata(analysis, profile)
	appendRecoveredMetadataCaveat(analysis)

	if analysis.Modality != "tabular" {
		t.Fatalf("expected missing modality to fall back to tabular, got %q", analysis.Modality)
	}
	if analysis.DatasetType != "unsupervised" {
		t.Fatalf("expected missing dataset type to resolve to unsupervised, got %q", analysis.DatasetType)
	}
	if analysis.AnnotationFormat != "CSV" {
		t.Fatalf("expected missing annotation format to resolve to CSV, got %q", analysis.AnnotationFormat)
	}
	if len(analysis.Caveats) != 1 {
		t.Fatalf("expected recovered metadata caveat, got %+v", analysis.Caveats)
	}
}
