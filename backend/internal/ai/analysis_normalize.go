package ai

import "strings"

func normalizeAnalysisForRequest(analysis *DatasetAnalysis, req AnalyzeRequest) {
	analysis.Summary = strings.TrimSpace(analysis.Summary)
	analysis.Modality = strings.ToLower(strings.TrimSpace(analysis.Modality))
	analysis.DatasetType = strings.ToLower(strings.TrimSpace(analysis.DatasetType))
	analysis.AnnotationFormat = strings.TrimSpace(analysis.AnnotationFormat)

	if req.ResearcherName != "" && !strings.Contains(analysis.Summary, req.ResearcherName) {
		analysis.Summary = strings.TrimSpace(analysis.Summary + " Uploaded by " + req.ResearcherName + ".")
	}

	filterUngroundedLabels(analysis)
	if len(analysis.Labels) == 0 {
		analysis.LabelCompleteness = 0
		if analysis.DatasetType == "supervised" || analysis.DatasetType == "semi-supervised" {
			analysis.DatasetType = "unknown"
		}
		if analysis.DatasetType == "" || analysis.DatasetType == "unknown" {
			analysis.DatasetType = fallbackDatasetType(req)
		}
	}
}

func summaryHasUnsupportedContent(summary string) bool {
	lower := strings.ToLower(summary)
	badPhrases := []string{
		"bounded samples",
		"schema extracted",
		"extracted schema",
		"profile json",
		"dataset profile",
		"metadata profile",
		"extracted from metadata",
		"with extracted schema",
		"consists of tabular parquet files",
		"contains parquet files",
	}
	for _, phrase := range badPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func summaryIsTooShort(summary string) bool {
	sentenceCount := 0
	for _, part := range strings.Split(summary, ".") {
		if strings.TrimSpace(part) != "" {
			sentenceCount++
		}
	}
	return sentenceCount < 5 || len(strings.Fields(summary)) < 45
}

func containsPlaceholderResearcher(summary string) bool {
	lower := strings.ToLower(summary)
	return strings.Contains(lower, "researcherx") ||
		strings.Contains(lower, "researcher x") ||
		strings.Contains(lower, "<researcher")
}

func fallbackDatasetType(req AnalyzeRequest) string {
	profile := strings.ToLower(req.ProfileJSON)
	if strings.Contains(profile, `"detected_type":"`) ||
		strings.Contains(profile, `"columns":`) ||
		strings.Contains(profile, `"sample_rows":`) ||
		strings.Contains(profile, `"sample_text":`) ||
		strings.Contains(profile, `"annotations":`) {
		return "unsupervised"
	}
	return "unknown"
}

func filterUngroundedLabels(analysis *DatasetAnalysis) {
	fieldOnlyNames := map[string]bool{
		"label": true, "labels": true, "class": true, "classes": true,
		"category": true, "categories": true, "target": true, "targets": true,
		"outcome": true, "outcomes": true, "ground_truth": true,
	}
	filtered := analysis.Labels[:0]
	for _, label := range analysis.Labels {
		name := strings.ToLower(strings.TrimSpace(label.Name))
		if name == "" {
			continue
		}
		if fieldOnlyNames[name] && (label.Proportion == nil || *label.Proportion == 0) {
			continue
		}
		filtered = append(filtered, label)
	}
	analysis.Labels = filtered
}

func appendCaveat(analysis *DatasetAnalysis, caveat string) {
	for _, existing := range analysis.Caveats {
		if existing == caveat {
			return
		}
	}
	analysis.Caveats = append(analysis.Caveats, caveat)
}
