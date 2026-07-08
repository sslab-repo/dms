package services

import (
	"math"
	"sort"
	"strings"

	"dataset-platform/backend/internal/ai"
	"dataset-platform/backend/internal/profiler"
)

type LabelField struct {
	Name          string   `json:"name"`
	NonEmptyCount int      `json:"non_empty_count"`
	EmptyCount    int      `json:"empty_count"`
	Completeness  float64  `json:"completeness"`
	Examples      []string `json:"examples"`
}

func deriveLabelFields(profile *profiler.DatasetProfile) []LabelField {
	if profile == nil {
		return []LabelField{}
	}

	byName := map[string]*LabelField{}
	for _, group := range profile.Groups {
		for _, column := range group.SharedColumns {
			addLabelFieldColumn(byName, column)
		}
	}
	if len(byName) == 0 {
		for _, file := range profile.Files {
			for _, column := range file.Columns {
				addLabelFieldColumn(byName, column)
			}
		}
	}

	fields := make([]LabelField, 0, len(byName))
	for _, field := range byName {
		field.Completeness = labelCompleteness(field.NonEmptyCount, field.EmptyCount)
		fields = append(fields, *field)
	}
	sort.Slice(fields, func(i, j int) bool {
		return fields[i].Name < fields[j].Name
	})
	return fields
}

func addLabelFieldColumn(fields map[string]*LabelField, column profiler.ColumnProfile) {
	name := strings.TrimSpace(column.Name)
	if name == "" || !isTargetLikeColumnName(name) {
		return
	}

	key := normalizedColumnName(name)
	field := fields[key]
	if field == nil {
		field = &LabelField{Name: name}
		fields[key] = field
	}
	field.NonEmptyCount += column.NonEmptyCount
	field.EmptyCount += column.EmptyCount
	field.Examples = appendUniqueExamples(field.Examples, column.ExampleValues, 5)
}

func legacyLabelField(label Label, datasetCompleteness float64) (LabelField, bool) {
	if !isTargetLikeColumnName(label.Name) {
		return LabelField{}, false
	}
	if label.Proportion != nil && *label.Proportion > 0 {
		return LabelField{}, false
	}

	nonEmpty := 0
	if label.SampleCount > 0 {
		nonEmpty = int(label.SampleCount)
	}
	completeness := labelCompleteness(nonEmpty, 0)
	if datasetCompleteness > 0 {
		completeness = clampCompleteness(datasetCompleteness)
	}
	return LabelField{
		Name:          strings.TrimSpace(label.Name),
		NonEmptyCount: nonEmpty,
		Completeness:  completeness,
		Examples:      []string{},
	}, true
}

func appendMissingLabelField(fields []LabelField, field LabelField) []LabelField {
	if field.Name == "" {
		return fields
	}
	key := normalizedColumnName(field.Name)
	for i := range fields {
		if normalizedColumnName(fields[i].Name) == key {
			return fields
		}
	}
	return append(fields, field)
}

func reconcileLabelMetadata(analysis *ai.DatasetAnalysis, profile *profiler.DatasetProfile) {
	if analysis == nil {
		return
	}

	fields := deriveLabelFields(profile)
	analysis.Labels = filterClassLabelsForProfile(analysis.Labels, profile, fields)

	// AllColumnNames evidence: label-like column name found in the full header
	// even if it was not included in the bounded sample window.
	hasHeaderLabelName := profileHasLabelColumnName(profile)

	hasLabelEvidence := len(fields) > 0 || len(analysis.Labels) > 0 ||
		profileHasAnnotationClasses(profile) || hasHeaderLabelName
	if !hasLabelEvidence {
		if analysis.DatasetType == "supervised" || analysis.DatasetType == "semi-supervised" {
			analysis.DatasetType = "unsupervised"
			analysis.LabelCompleteness = 0
		} else if (analysis.DatasetType == "" || analysis.DatasetType == "unknown") && profileHasAnalyzableEvidence(profile) {
			analysis.DatasetType = "unsupervised"
			analysis.LabelCompleteness = 0
		}
		return
	}

	if len(fields) > 0 {
		completeness := maxLabelFieldCompleteness(fields)
		if completeness > analysis.LabelCompleteness {
			analysis.LabelCompleteness = completeness
		}
		if analysis.DatasetType == "" || analysis.DatasetType == "unknown" || analysis.DatasetType == "unsupervised" {
			if completeness > 0 && completeness < 0.95 {
				analysis.DatasetType = "semi-supervised"
			} else if completeness >= 0.95 {
				analysis.DatasetType = "supervised"
			}
		}
		return
	}

	// Evidence from AllColumnNames (name-only, no sampled stats) or annotation
	// classes. Use a cautious default completeness when the header indicates a
	// label column exists but no row values were sampled for it.
	if hasHeaderLabelName && analysis.LabelCompleteness == 0 {
		analysis.LabelCompleteness = 0.8
	}
	if analysis.DatasetType == "" || analysis.DatasetType == "unknown" || analysis.DatasetType == "unsupervised" {
		analysis.DatasetType = "supervised"
	}
}

// profileHasLabelColumnName returns true when any file's AllColumnNames list
// contains a label-like column name. This catches datasets where the label
// column sits beyond the profiler's row-sampling budget.
func profileHasLabelColumnName(profile *profiler.DatasetProfile) bool {
	if profile == nil {
		return false
	}
	for _, g := range profile.Groups {
		for _, name := range g.AllColumnNames {
			if isTargetLikeColumnName(name) {
				return true
			}
		}
	}
	for _, f := range profile.Files {
		for _, name := range f.AllColumnNames {
			if isTargetLikeColumnName(name) {
				return true
			}
		}
	}
	return false
}

// applyDeclaredLabelColumn applies a researcher-declared label column name as
// definitive evidence. It looks the column up in sampled stats for completeness;
// if the column was not sampled it assumes full completeness (user declaration).
func applyDeclaredLabelColumn(labelColumn string, profile *profiler.DatasetProfile, analysis *ai.DatasetAnalysis) {
	if analysis == nil || strings.TrimSpace(labelColumn) == "" {
		return
	}
	normTarget := normalizedColumnName(labelColumn)

	completeness := 0.0
	for _, g := range profile.Groups {
		for _, col := range g.SharedColumns {
			if normalizedColumnName(col.Name) == normTarget {
				c := labelCompleteness(col.NonEmptyCount, col.EmptyCount)
				if c > completeness {
					completeness = c
				}
			}
		}
	}
	for _, f := range profile.Files {
		for _, col := range f.Columns {
			if normalizedColumnName(col.Name) == normTarget {
				c := labelCompleteness(col.NonEmptyCount, col.EmptyCount)
				if c > completeness {
					completeness = c
				}
			}
		}
	}

	if completeness == 0 {
		completeness = 1.0 // declared but not sampled; assume complete
	}
	if completeness > analysis.LabelCompleteness {
		analysis.LabelCompleteness = completeness
	}
	if analysis.DatasetType == "" || analysis.DatasetType == "unknown" || analysis.DatasetType == "unsupervised" {
		if completeness >= 0.95 {
			analysis.DatasetType = "supervised"
		} else {
			analysis.DatasetType = "semi-supervised"
		}
	}
}

func profileHasAnnotationClasses(profile *profiler.DatasetProfile) bool {
	if profile == nil {
		return false
	}
	for _, annotation := range profile.Annotations {
		if len(annotation.Classes) > 0 || annotation.ClassCount > 0 {
			return true
		}
	}
	return false
}

func profileHasAnalyzableEvidence(profile *profiler.DatasetProfile) bool {
	if profile == nil {
		return false
	}
	for _, annotation := range profile.Annotations {
		if annotation.Format != "" || len(annotation.Classes) > 0 || annotation.TotalAnnotations > 0 {
			return true
		}
	}
	for _, group := range profile.Groups {
		if knownAnalyzableType(group.DetectedType) {
			return true
		}
		if len(group.SharedColumns) > 0 || len(group.RepresentativeExamples) > 0 {
			return true
		}
	}
	for _, file := range profile.Files {
		if knownAnalyzableType(file.DetectedType) {
			return true
		}
		if len(file.Columns) > 0 || len(file.SampleRows) > 0 || len(file.SampleText) > 0 || file.Annotation != nil {
			return true
		}
	}
	for _, fileType := range profile.FileTypes {
		if knownAnalyzableType(fileType.DetectedType) && fileType.FileCount > 0 {
			return true
		}
	}
	return false
}

func knownAnalyzableType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "csv", "tsv", "json", "jsonl", "parquet", "xml", "text", "txt", "md",
		"png", "jpg", "jpeg", "image", "audio", "wav", "mp3":
		return true
	default:
		return false
	}
}

func filterClassLabelsForProfile(labels []ai.Label, profile *profiler.DatasetProfile, fields []LabelField) []ai.Label {
	if len(labels) == 0 {
		return labels
	}

	profileIndex := buildProfileLabelIndex(profile, fields)
	filtered := labels[:0]
	for _, label := range labels {
		if shouldExposeClassLabel(label.Name, label.Proportion, profileIndex) {
			filtered = append(filtered, label)
		}
	}
	return filtered
}

func detailLabelShouldBeClass(label Label, profile *profiler.DatasetProfile, fields []LabelField) bool {
	return shouldExposeClassLabel(label.Name, label.Proportion, buildProfileLabelIndex(profile, fields))
}

type profileLabelIndex struct {
	columnNames      map[string]bool
	labelFieldNames  map[string]bool
	labelFieldValues map[string]bool
	annotationLabels map[string]bool
	hasLabelEvidence bool
}

func buildProfileLabelIndex(profile *profiler.DatasetProfile, fields []LabelField) profileLabelIndex {
	index := profileLabelIndex{
		columnNames:      map[string]bool{},
		labelFieldNames:  map[string]bool{},
		labelFieldValues: map[string]bool{},
		annotationLabels: map[string]bool{},
	}

	for _, field := range fields {
		index.hasLabelEvidence = true
		index.labelFieldNames[normalizedColumnName(field.Name)] = true
		for _, example := range field.Examples {
			index.labelFieldValues[normalizedColumnName(example)] = true
		}
	}
	if profile == nil {
		return index
	}
	for _, group := range profile.Groups {
		for _, column := range group.SharedColumns {
			index.columnNames[normalizedColumnName(column.Name)] = true
		}
	}
	for _, file := range profile.Files {
		for _, column := range file.Columns {
			index.columnNames[normalizedColumnName(column.Name)] = true
		}
	}
	for _, annotation := range profile.Annotations {
		if len(annotation.Classes) > 0 {
			index.hasLabelEvidence = true
		}
		for _, class := range annotation.Classes {
			index.annotationLabels[normalizedColumnName(class.Name)] = true
			if class.ID != "" {
				index.annotationLabels[normalizedColumnName(class.ID)] = true
			}
		}
	}
	return index
}

func shouldExposeClassLabel(name string, proportion *float64, index profileLabelIndex) bool {
	normalized := normalizedColumnName(name)
	if normalized == "" {
		return false
	}
	if index.labelFieldNames[normalized] {
		return false
	}
	if index.annotationLabels[normalized] || index.labelFieldValues[normalized] {
		return true
	}
	if index.columnNames[normalized] {
		return false
	}
	if proportion == nil || *proportion <= 0 {
		return false
	}
	return index.hasLabelEvidence
}

func maxLabelFieldCompleteness(fields []LabelField) float64 {
	maxValue := 0.0
	for _, field := range fields {
		if field.Completeness > maxValue {
			maxValue = field.Completeness
		}
	}
	return clampCompleteness(maxValue)
}

func isTargetLikeColumnName(name string) bool {
	normalized := normalizedColumnName(name)
	switch normalized {
	case "label", "labels", "class", "classes", "target", "targets",
		"outcome", "outcomes", "sentiment", "diagnosis", "ground_truth",
		"groundtruth", "answer", "answers", "y":
		return true
	}
	return strings.Contains(normalized, "label") ||
		strings.Contains(normalized, "ground_truth") ||
		strings.HasSuffix(normalized, "_class") ||
		strings.HasSuffix(normalized, "_target") ||
		strings.HasSuffix(normalized, "_outcome")
}

func normalizedColumnName(name string) string {
	parts := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(name)), func(r rune) bool {
		return r == ' ' || r == '-' || r == '.' || r == '/'
	})
	return strings.Join(parts, "_")
}

func labelCompleteness(nonEmpty, empty int) float64 {
	total := nonEmpty + empty
	if total <= 0 {
		return 0
	}
	return clampCompleteness(float64(nonEmpty) / float64(total))
}

func clampCompleteness(value float64) float64 {
	if math.IsNaN(value) || value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func appendUniqueExamples(existing []string, examples []string, max int) []string {
	seen := map[string]bool{}
	for _, value := range existing {
		seen[value] = true
	}
	for _, example := range examples {
		example = strings.TrimSpace(example)
		if example == "" || seen[example] {
			continue
		}
		existing = append(existing, example)
		seen[example] = true
		if len(existing) >= max {
			break
		}
	}
	return existing
}
