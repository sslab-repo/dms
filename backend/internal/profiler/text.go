package profiler

import "strings"

func profileText(path string, fp *FileProfile) {
	b, err := readLimited(path, maxReadBytes)
	if err != nil {
		fp.Warnings = append(fp.Warnings, "could not open text sample")
		return
	}
	lines := strings.Split(string(b), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fp.SampleText = append(fp.SampleText, truncate(line, 220))
		fp.SampledRows++
		if len(fp.SampleText) >= maxTextLines {
			break
		}
	}
	if fp.Role == "annotations" {
		if annotation, ok := parseYOLOAnnotationText(lines, fp.OriginalName); ok {
			fp.Annotation = annotation
		}
	}
}
