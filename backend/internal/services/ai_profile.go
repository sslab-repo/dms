package services

import (
	"encoding/json"
	"sort"
	"strings"

	"dataset-platform/backend/internal/profiler"
)

const (
	maxAIProfileGroups     = 4
	maxAIProfileColumns    = 18
	maxAIProfileRows       = 3
	maxAIProfileExamples   = 2
	maxAIProfileValueChars = 80
	maxAIProfileTextChars  = 120
)

type aiProfile struct {
	TotalFiles       int                    `json:"total_files"`
	TotalSizeBytes   int64                  `json:"total_size_bytes"`
	FileTypes        []profiler.TypeSummary `json:"file_types"`
	DetectedPatterns []string               `json:"detected_patterns,omitempty"`
	Notes            []string               `json:"notes,omitempty"`
	Annotations      []aiAnnotation         `json:"annotations,omitempty"`
	Groups           []aiProfileGroup       `json:"groups"`
}

type aiProfileGroup struct {
	DetectedType   string              `json:"detected_type"`
	Role           string              `json:"role"`
	FileCount      int                 `json:"file_count"`
	TotalSize      int64               `json:"total_size_bytes"`
	Columns        []aiProfileColumn   `json:"columns,omitempty"`
	AllColumnNames []string            `json:"all_column_names,omitempty"`
	SampleRows     []map[string]string `json:"sample_rows,omitempty"`
	Warnings       []string            `json:"warnings,omitempty"`
}

type aiProfileColumn struct {
	Name          string   `json:"name"`
	InferredType  string   `json:"inferred_type"`
	NonEmptyCount int      `json:"non_empty_count"`
	EmptyCount    int      `json:"empty_count"`
	Examples      []string `json:"examples,omitempty"`
}

type aiAnnotation struct {
	Format           string   `json:"format"`
	ClassCount       int      `json:"class_count"`
	TotalAnnotations int64    `json:"total_annotations"`
	Classes          []string `json:"classes,omitempty"`
	Notes            []string `json:"notes,omitempty"`
}

func buildAIProfileJSON(profile *profiler.DatasetProfile) string {
	if profile == nil {
		return "{}"
	}

	compact := aiProfile{
		TotalFiles:       profile.TotalFiles,
		TotalSizeBytes:   profile.TotalSizeBytes,
		FileTypes:        profile.FileTypes,
		DetectedPatterns: limitStrings(profile.DetectedPatterns, 8, maxAIProfileValueChars),
		Notes:            limitStrings(profile.Notes, 5, maxAIProfileTextChars),
		Annotations:      compactAnnotations(profile.Annotations),
		Groups:           compactGroups(profile.Groups),
	}

	b, err := json.Marshal(compact)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func compactGroups(groups []profiler.FileGroup) []aiProfileGroup {
	out := make([]aiProfileGroup, 0, minInt(len(groups), maxAIProfileGroups))
	for _, group := range groups {
		if len(out) >= maxAIProfileGroups {
			break
		}

		compact := aiProfileGroup{
			DetectedType:   group.DetectedType,
			Role:           group.Role,
			FileCount:      group.FileCount,
			TotalSize:      group.TotalSizeBytes,
			Columns:        compactColumns(group.SharedColumns),
			AllColumnNames: group.AllColumnNames,
		}
		for _, example := range group.RepresentativeExamples {
			if len(compact.SampleRows) >= maxAIProfileRows {
				break
			}
			compact.SampleRows = append(compact.SampleRows, compactRows(example.SampleRows)...)
			compact.Warnings = append(compact.Warnings, limitStrings(example.Warnings, 3, maxAIProfileTextChars)...)
		}
		if len(compact.SampleRows) > maxAIProfileRows {
			compact.SampleRows = compact.SampleRows[:maxAIProfileRows]
		}
		out = append(out, compact)
	}
	return out
}

func compactColumns(columns []profiler.ColumnProfile) []aiProfileColumn {
	out := make([]aiProfileColumn, 0, minInt(len(columns), maxAIProfileColumns))
	for _, column := range columns {
		if len(out) >= maxAIProfileColumns {
			break
		}
		out = append(out, aiProfileColumn{
			Name:          column.Name,
			InferredType:  column.InferredType,
			NonEmptyCount: column.NonEmptyCount,
			EmptyCount:    column.EmptyCount,
			Examples:      limitStrings(column.ExampleValues, maxAIProfileExamples, maxAIProfileValueChars),
		})
	}
	return out
}

func compactRows(rows []map[string]string) []map[string]string {
	out := make([]map[string]string, 0, minInt(len(rows), maxAIProfileRows))
	for _, row := range rows {
		if len(out) >= maxAIProfileRows {
			break
		}
		compact := map[string]string{}
		keys := make([]string, 0, len(row))
		for key := range row {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for i, key := range keys {
			if i >= maxAIProfileColumns {
				break
			}
			compact[key] = truncateForAI(row[key], maxAIProfileValueChars)
		}
		out = append(out, compact)
	}
	return out
}

func compactAnnotations(annotations []profiler.AnnotationProfile) []aiAnnotation {
	out := make([]aiAnnotation, 0, minInt(len(annotations), 3))
	for _, annotation := range annotations {
		classes := make([]string, 0, minInt(len(annotation.Classes), 20))
		for _, class := range annotation.Classes {
			if len(classes) >= 20 {
				break
			}
			classes = append(classes, truncateForAI(class.Name, maxAIProfileValueChars))
		}
		out = append(out, aiAnnotation{
			Format:           annotation.Format,
			ClassCount:       annotation.ClassCount,
			TotalAnnotations: annotation.TotalAnnotations,
			Classes:          classes,
			Notes:            limitStrings(annotation.Notes, 3, maxAIProfileTextChars),
		})
	}
	return out
}

func limitStrings(values []string, maxItems int, maxChars int) []string {
	out := make([]string, 0, minInt(len(values), maxItems))
	for _, value := range values {
		if len(out) >= maxItems {
			break
		}
		value = truncateForAI(value, maxChars)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func truncateForAI(value string, maxChars int) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\x00", ""))
	if len(value) <= maxChars {
		return value
	}
	return value[:maxChars] + "..."
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
