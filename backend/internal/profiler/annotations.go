package profiler

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

func parseCOCOAnnotation(value any, sourceName string) (*AnnotationProfile, bool) {
	obj, ok := value.(map[string]any)
	if !ok {
		return nil, false
	}

	categories, ok := obj["categories"].([]any)
	if !ok || len(categories) == 0 {
		return nil, false
	}

	idToName := map[string]string{}
	for _, raw := range categories {
		cat, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		id := stringifyCategoryID(cat["id"])
		name, _ := cat["name"].(string)
		name = strings.TrimSpace(name)
		if id == "" && name == "" {
			continue
		}
		if name == "" {
			name = "category_" + id
		}
		idToName[id] = name
	}
	if len(idToName) == 0 {
		return nil, false
	}

	counts := map[string]int64{}
	if annotations, ok := obj["annotations"].([]any); ok {
		for _, raw := range annotations {
			ann, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			id := stringifyCategoryID(ann["category_id"])
			if id != "" {
				counts[id]++
			}
		}
	}

	profile := &AnnotationProfile{
		Format:      "COCO JSON",
		SourceFiles: []string{sourceName},
		ClassCount:  len(idToName),
		Notes:       []string{"COCO categories were parsed from annotation JSON metadata."},
	}
	for id, count := range counts {
		profile.TotalAnnotations += count
		_ = id
	}
	for id, name := range idToName {
		profile.Classes = append(profile.Classes, ClassProfile{
			ID:    id,
			Name:  name,
			Count: counts[id],
		})
	}
	finalizeClassProfiles(profile)
	return profile, true
}

func parseYOLOAnnotationText(lines []string, sourceName string) (*AnnotationProfile, bool) {
	classNames := parseClassNameList(lines)
	if len(classNames) > 0 {
		profile := &AnnotationProfile{
			Format:      "YOLO class names",
			SourceFiles: []string{sourceName},
			ClassCount:  len(classNames),
			Notes:       []string{"Class names were parsed from a YOLO-style names/classes file."},
		}
		for id, name := range classNames {
			profile.Classes = append(profile.Classes, ClassProfile{
				ID:   strconv.Itoa(id),
				Name: name,
			})
		}
		return profile, true
	}

	counts := map[string]int64{}
	for _, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if !isYOLOLabelLine(fields) {
			continue
		}
		classID := fields[0]
		counts[classID]++
	}
	if len(counts) == 0 {
		return nil, false
	}

	profile := &AnnotationProfile{
		Format:      "YOLO TXT",
		SourceFiles: []string{sourceName},
		ClassCount:  len(counts),
		Notes:       []string{"YOLO class IDs were parsed from label rows; class names require a names/classes file."},
	}
	for id, count := range counts {
		profile.TotalAnnotations += count
		profile.Classes = append(profile.Classes, ClassProfile{
			ID:    id,
			Name:  "class_" + id,
			Count: count,
		})
	}
	finalizeClassProfiles(profile)
	return profile, true
}

type annotationAggregate struct {
	format      string
	sourceFiles []string
	counts      map[string]int64
	names       map[string]string
	notes       map[string]bool
}

func buildAnnotationSummaries(files []FileProfile) []AnnotationProfile {
	byFormat := map[string]*annotationAggregate{}
	for _, file := range files {
		if file.Annotation == nil {
			continue
		}
		ann := file.Annotation
		agg := byFormat[ann.Format]
		if agg == nil {
			agg = &annotationAggregate{
				format: ann.Format,
				counts: map[string]int64{},
				names:  map[string]string{},
				notes:  map[string]bool{},
			}
			byFormat[ann.Format] = agg
		}
		agg.sourceFiles = append(agg.sourceFiles, ann.SourceFiles...)
		for _, note := range ann.Notes {
			agg.notes[note] = true
		}
		for _, class := range ann.Classes {
			key := class.ID
			if key == "" {
				key = class.Name
			}
			agg.counts[key] += class.Count
			if class.Name != "" && !strings.HasPrefix(class.Name, "class_") {
				agg.names[key] = class.Name
			} else if _, ok := agg.names[key]; !ok {
				agg.names[key] = class.Name
			}
		}
	}

	applyYOLOClassNames(byFormat)

	summaries := make([]AnnotationProfile, 0, len(byFormat))
	for _, agg := range byFormat {
		summary := AnnotationProfile{
			Format:      agg.format,
			SourceFiles: uniqueStrings(agg.sourceFiles),
			ClassCount:  len(agg.names),
		}
		for note := range agg.notes {
			summary.Notes = append(summary.Notes, note)
		}
		for key, count := range agg.counts {
			summary.TotalAnnotations += count
			name := agg.names[key]
			if name == "" {
				name = key
			}
			summary.Classes = append(summary.Classes, ClassProfile{
				ID:    key,
				Name:  name,
				Count: count,
			})
		}
		for key, name := range agg.names {
			if _, ok := agg.counts[key]; !ok {
				summary.Classes = append(summary.Classes, ClassProfile{
					ID:   key,
					Name: name,
				})
			}
		}
		finalizeClassProfiles(&summary)
		summaries = append(summaries, summary)
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].TotalAnnotations > summaries[j].TotalAnnotations
	})
	return summaries
}

func applyYOLOClassNames(byFormat map[string]*annotationAggregate) {
	labelAgg := byFormat["YOLO TXT"]
	namesAgg := byFormat["YOLO class names"]
	if labelAgg == nil || namesAgg == nil {
		return
	}
	for id, name := range namesAgg.names {
		if _, ok := labelAgg.counts[id]; ok {
			labelAgg.names[id] = name
		}
	}
	labelAgg.notes["YOLO class IDs were mapped to names from a names/classes file."] = true
}

func parseClassNameList(lines []string) []string {
	var names []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if isYOLOLabelLine(fields) {
			continue
		}
		if strings.HasPrefix(line, "-") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
		} else if parts := strings.SplitN(line, ":", 2); len(parts) == 2 {
			left := strings.TrimSpace(parts[0])
			right := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			if left == "names" || right == "" {
				continue
			}
			if _, err := strconv.Atoi(left); err == nil {
				line = right
			}
		}
		line = strings.Trim(line, `"'`)
		if line == "" {
			continue
		}
		names = append(names, line)
	}
	if len(names) < 2 {
		return nil
	}
	return names
}

func isYOLOLabelLine(fields []string) bool {
	if len(fields) < 5 {
		return false
	}
	if _, err := strconv.Atoi(fields[0]); err != nil {
		return false
	}
	for _, field := range fields[1:5] {
		if _, err := strconv.ParseFloat(field, 64); err != nil {
			return false
		}
	}
	return true
}

func finalizeClassProfiles(profile *AnnotationProfile) {
	sort.Slice(profile.Classes, func(i, j int) bool {
		if profile.Classes[i].Count == profile.Classes[j].Count {
			return profile.Classes[i].Name < profile.Classes[j].Name
		}
		return profile.Classes[i].Count > profile.Classes[j].Count
	})
	if len(profile.Classes) > 20 {
		profile.Classes = profile.Classes[:20]
		profile.Notes = append(profile.Notes, "Only the top 20 classes are included in the profile.")
	}
	if profile.TotalAnnotations > 0 {
		for i := range profile.Classes {
			profile.Classes[i].Proportion = float64(profile.Classes[i].Count) / float64(profile.TotalAnnotations)
		}
	}
	sort.Strings(profile.Notes)
}

func stringifyCategoryID(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case float64:
		return strconv.FormatInt(int64(v), 10)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
