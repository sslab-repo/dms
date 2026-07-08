package ai

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

/*
Dataset analysis is what we get back from Flash at upload time.
*/
type Label struct {
	Name        string   `json:"name"`
	Proportion  *float64 `json:"proportion"`
	SampleCount int64    `json:"sample_count"` // -1 means unknown
}

// parseLabel is a custom parser that handles sample_count being either a number or a string like "-"
func parseLabel(raw json.RawMessage) (Label, error) {
	var rawLabel struct {
		Name        string      `json:"name"`
		Proportion  interface{} `json:"proportion"`
		SampleCount interface{} `json:"sample_count"`
	}
	if err := json.Unmarshal(raw, &rawLabel); err != nil {
		return Label{}, err
	}

	label := Label{
		Name:        strings.TrimSpace(rawLabel.Name),
		SampleCount: -1,
	}
	if prop, ok := parseFloatFromAny(rawLabel.Proportion); ok {
		label.Proportion = &prop
	}
	if count, ok := parseInt64FromAny(rawLabel.SampleCount); ok {
		label.SampleCount = count
	}

	return label, nil
}

type DatasetAnalysis struct {
	Summary           string   `json:"summary"`
	Labels            []Label  `json:"labels"`
	LabelCompleteness float64  `json:"label_completeness"` // 0.0 – 1.0
	Modality          string   `json:"modality"`
	DatasetType       string   `json:"dataset_type"`
	AnnotationFormat  string   `json:"annotation_format"`
	PseudoQueries     []string `json:"pseudo_queries"`
	Confidence        float64  `json:"confidence"`
	Caveats           []string `json:"caveats"`

	ParseRecovery         string   `json:"-"`
	MissingRequiredFields []string `json:"-"`
	MissingMetadataFields []string `json:"-"`
}

type analysisPayload struct {
	Summary           string            `json:"summary"`
	Labels            []json.RawMessage `json:"labels"`
	LabelCompleteness interface{}       `json:"label_completeness"`
	Modality          string            `json:"modality"`
	DatasetType       string            `json:"dataset_type"`
	AnnotationFormat  string            `json:"annotation_format"`
	PseudoQueries     interface{}       `json:"pseudo_queries"`
	Confidence        interface{}       `json:"confidence"`
	Caveats           interface{}       `json:"caveats"`
}

func parseDatasetAnalysis(raw string) (*DatasetAnalysis, error) {
	var payload analysisPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, err
	}

	analysis := &DatasetAnalysis{
		Summary:               strings.TrimSpace(payload.Summary),
		Modality:              strings.TrimSpace(payload.Modality),
		DatasetType:           strings.TrimSpace(payload.DatasetType),
		AnnotationFormat:      strings.TrimSpace(payload.AnnotationFormat),
		PseudoQueries:         parseStringListFromAny(payload.PseudoQueries),
		Caveats:               parseStringListFromAny(payload.Caveats),
		MissingRequiredFields: missingRequiredFields(payload),
		MissingMetadataFields: missingMetadataFields(payload),
	}
	if val, ok := parseFloatFromAny(payload.LabelCompleteness); ok {
		analysis.LabelCompleteness = val
	}
	if val, ok := parseFloatFromAny(payload.Confidence); ok {
		analysis.Confidence = val
	}
	for _, rawLabel := range payload.Labels {
		label, err := parseLabel(rawLabel)
		if err != nil {
			continue
		}
		analysis.Labels = append(analysis.Labels, label)
	}

	normalizeAnalysis(analysis)
	return analysis, nil
}

func missingRequiredFields(payload analysisPayload) []string {
	var missing []string
	if strings.TrimSpace(payload.Summary) == "" {
		missing = append(missing, "summary")
	}
	if payload.Labels == nil {
		missing = append(missing, "labels")
	}
	if payload.LabelCompleteness == nil {
		missing = append(missing, "label_completeness")
	}
	if strings.TrimSpace(payload.Modality) == "" {
		missing = append(missing, "modality")
	}
	if strings.TrimSpace(payload.DatasetType) == "" {
		missing = append(missing, "dataset_type")
	}
	if strings.TrimSpace(payload.AnnotationFormat) == "" {
		missing = append(missing, "annotation_format")
	}
	if payload.PseudoQueries == nil {
		missing = append(missing, "pseudo_queries")
	}
	if payload.Confidence == nil {
		missing = append(missing, "confidence")
	}
	if payload.Caveats == nil {
		missing = append(missing, "caveats")
	}
	return missing
}

func missingMetadataFields(payload analysisPayload) []string {
	var missing []string
	if payload.LabelCompleteness == nil {
		missing = append(missing, "label_completeness")
	}
	if strings.TrimSpace(payload.Modality) == "" {
		missing = append(missing, "modality")
	}
	if strings.TrimSpace(payload.DatasetType) == "" {
		missing = append(missing, "dataset_type")
	}
	if strings.TrimSpace(payload.AnnotationFormat) == "" {
		missing = append(missing, "annotation_format")
	}
	return missing
}

func parseStringListFromAny(v interface{}) []string {
	var out []string
	switch val := v.(type) {
	case nil:
		return []string{}
	case []string:
		out = val
	case []interface{}:
		for _, item := range val {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
	case string:
		if strings.TrimSpace(val) == "" {
			return []string{}
		}
		out = strings.Split(val, "\n")
	}

	seen := map[string]bool{}
	clean := make([]string, 0, len(out))
	for _, item := range out {
		item = strings.TrimSpace(strings.Trim(item, "-*• "))
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		clean = append(clean, item)
	}
	return clean
}

func parseFloatFromAny(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	case json.Number:
		f, err := val.Float64()
		if err == nil {
			return f, true
		}
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(val), 64)
		if err == nil {
			return f, true
		}
	}
	return 0, false
}

func parseInt64FromAny(v interface{}) (int64, bool) {
	if v == nil {
		return -1, true
	}
	switch val := v.(type) {
	case float64:
		return int64(val), true
	case float32:
		return int64(val), true
	case int:
		return int64(val), true
	case int64:
		return val, true
	case json.Number:
		i, err := val.Int64()
		if err == nil {
			return i, true
		}
	case string:
		trimmed := strings.TrimSpace(val)
		if trimmed == "" || trimmed == "-" {
			return -1, true
		}
		i, err := strconv.ParseInt(trimmed, 10, 64)
		if err == nil {
			return i, true
		}
	}
	return -1, false
}

func normalizeAnalysis(analysis *DatasetAnalysis) {
	analysis.LabelCompleteness = clampFloat(analysis.LabelCompleteness, 0, 1)
	analysis.Confidence = clampFloat(analysis.Confidence, 0, 1)
	if analysis.Modality == "" {
		analysis.Modality = "unknown"
	}
	if analysis.DatasetType == "" {
		analysis.DatasetType = "unknown"
	}
	if analysis.AnnotationFormat == "" {
		analysis.AnnotationFormat = "unknown"
	}
	for i := range analysis.Labels {
		if analysis.Labels[i].Proportion != nil {
			prop := clampFloat(*analysis.Labels[i].Proportion, 0, 1)
			analysis.Labels[i].Proportion = &prop
		}
		if analysis.Labels[i].Name == "" {
			analysis.Labels[i].Name = fmt.Sprintf("label-%d", i+1)
		}
	}
}

func clampFloat(val, min, max float64) float64 {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}
