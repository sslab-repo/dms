package profiler

import (
	"strings"
)

// profileXML reads an XML file and attempts to identify its annotation format.
// Currently detects Pascal VOC XML (the dominant CV annotation format pre-COCO).
func profileXML(path string, fp *FileProfile) {
	b, err := readLimited(path, maxReadBytes)
	if err != nil {
		fp.Warnings = append(fp.Warnings, "could not open XML file")
		return
	}
	content := string(b)

	if isPascalVOC(content) {
		fp.Annotation = parsePascalVOC(content, fp.OriginalName)
		return
	}

	// Generic XML: show a text sample so the AI can identify the format.
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "<?" || strings.HasPrefix(line, "<?") {
			continue
		}
		fp.SampleText = append(fp.SampleText, truncate(line, 220))
		fp.SampledRows++
		if len(fp.SampleText) >= maxTextLines {
			break
		}
	}
}

func isPascalVOC(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "<annotation") &&
		strings.Contains(lower, "<object")
}

func parsePascalVOC(content, sourceName string) *AnnotationProfile {
	lower := strings.ToLower(content)

	// Count <object> blocks — each is one annotation.
	objCount := int64(strings.Count(lower, "<object>") + strings.Count(lower, "<object "))

	// Extract class names from <name> tags inside <object> blocks.
	classCounts := map[string]int64{}
	remaining := content
	for {
		start := strings.Index(strings.ToLower(remaining), "<object")
		if start < 0 {
			break
		}
		block := remaining[start:]
		end := strings.Index(strings.ToLower(block), "</object>")
		if end < 0 {
			break
		}
		block = block[:end]
		remaining = remaining[start+end+9:]

		nameStart := strings.Index(strings.ToLower(block), "<name>")
		nameEnd := strings.Index(strings.ToLower(block), "</name>")
		if nameStart >= 0 && nameEnd > nameStart {
			cls := strings.TrimSpace(block[nameStart+6 : nameEnd])
			if cls != "" {
				classCounts[cls]++
			}
		}
	}

	profile := &AnnotationProfile{
		Format:           "Pascal VOC XML",
		SourceFiles:      []string{sourceName},
		ClassCount:       len(classCounts),
		TotalAnnotations: objCount,
		Notes:            []string{"Pascal VOC XML bounding-box annotations detected."},
	}
	for name, count := range classCounts {
		profile.Classes = append(profile.Classes, ClassProfile{
			Name:  name,
			Count: count,
		})
	}
	finalizeClassProfiles(profile)
	return profile
}
