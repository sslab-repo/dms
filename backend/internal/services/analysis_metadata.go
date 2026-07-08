package services

import (
	"sort"
	"strings"

	"dataset-platform/backend/internal/ai"
	"dataset-platform/backend/internal/profiler"
)

// ReclassifyFromProfile re-runs label-metadata reconciliation against a freshly
// generated profile without re-invoking the AI. It is used by the reclassify
// command to fix existing datasets after a profiler update.
func ReclassifyFromProfile(currentType string, currentCompleteness float64, profile *profiler.DatasetProfile) (string, float64) {
	analysis := &ai.DatasetAnalysis{
		DatasetType:       currentType,
		LabelCompleteness: currentCompleteness,
	}
	applyProfileCorrections(analysis, profile)
	reconcileLabelMetadata(analysis, profile)
	return analysis.DatasetType, analysis.LabelCompleteness
}

func applyProfileCorrections(analysis *ai.DatasetAnalysis, profile *profiler.DatasetProfile) {
	if analysis == nil || profile == nil {
		return
	}

	if format := profileBackedFormat(profile); format != "" {
		analysis.AnnotationFormat = format
	}
	if normalizeModality(analysis.Modality) == "unknown" {
		if modality := profileBackedModality(profile); modality != "" {
			analysis.Modality = modality
		}
	}

	analysis.AnnotationFormat = normalizeDisplayValue(analysis.AnnotationFormat)
	analysis.Modality = normalizeModality(analysis.Modality)
	analysis.DatasetType = normalizeDatasetType(analysis.DatasetType)
}

func profileBackedFormat(profile *profiler.DatasetProfile) string {
	if len(profile.Annotations) > 0 {
		formats := make([]string, 0, len(profile.Annotations))
		for _, annotation := range profile.Annotations {
			if annotation.Format != "" {
				formats = append(formats, annotation.Format)
			}
		}
		if len(formats) > 0 {
			sort.Strings(formats)
			return strings.Join(uniqueStrings(formats), ", ")
		}
	}

	formats := make([]string, 0, len(profile.FileTypes))
	for _, fileType := range profile.FileTypes {
		if fileType.DetectedType == "" || fileType.DetectedType == "unknown" || fileType.DetectedType == "blob" {
			continue
		}
		formats = append(formats, normalizeDisplayValue(fileType.DetectedType))
	}
	sort.Strings(formats)
	return strings.Join(uniqueStrings(formats), ", ")
}

func profileBackedModality(profile *profiler.DatasetProfile) string {
	modalities := map[string]bool{}
	for _, fileType := range profile.FileTypes {
		switch strings.ToLower(fileType.DetectedType) {
		case "csv", "json", "parquet", "xml":
			modalities["tabular"] = true
		case "png", "jpg", "jpeg", "image":
			modalities["image"] = true
		case "text", "txt", "md":
			modalities["text"] = true
		case "audio", "wav", "mp3":
			modalities["audio"] = true
		}
	}
	if len(modalities) == 0 {
		return ""
	}
	if len(modalities) > 1 {
		return "multimodal"
	}
	for modality := range modalities {
		return modality
	}
	return ""
}

func normalizeDisplayValue(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "csv":
		return "CSV"
	case "json":
		return "JSON"
	case "xml":
		return "XML"
	case "parquet":
		return "Parquet"
	case "png":
		return "PNG"
	case "jpg", "jpeg":
		return "JPG"
	case "coco json":
		return "COCO JSON"
	case "yolo txt":
		return "YOLO TXT"
	case "plain text":
		return "plain text"
	case "image files":
		return "image files"
	case "none":
		return "none"
	case "unknown":
		return "unknown"
	default:
		return strings.TrimSpace(value)
	}
}

func normalizeModality(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "text", "image", "tabular", "audio", "multimodal":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "unknown"
	}
}

func normalizeDatasetType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "supervised", "unsupervised", "semi-supervised", "self-supervised":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "unknown"
	}
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	unique := values[:0]
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	return unique
}
