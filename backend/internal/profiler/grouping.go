package profiler

import (
	"sort"
	"strings"
)

func buildGroup(key string, files []FileProfile) FileGroup {
	group := FileGroup{
		Key:            key,
		DetectedType:   files[0].DetectedType,
		Role:           files[0].Role,
		FileCount:      len(files),
		SharedColumns:  sharedColumns(files),
		AllColumnNames: unionColumnNames(files),
		TotalSizeBytes: 0,
	}
	for i, file := range files {
		group.TotalSizeBytes += file.SizeBytes
		if i < maxRepresentativeFiles {
			group.RepresentativeFileIDs = append(group.RepresentativeFileIDs, file.FileID)
			group.RepresentativeExamples = append(group.RepresentativeExamples, file)
		}
	}
	return group
}

func buildGroupKey(fp FileProfile) string {
	schema := "no-schema"
	if len(fp.Columns) > 0 {
		names := make([]string, 0, len(fp.Columns))
		for _, col := range fp.Columns {
			names = append(names, strings.ToLower(col.Name)+":"+col.InferredType)
		}
		sort.Strings(names)
		schema = strings.Join(names, "|")
	}
	return fp.DetectedType + "::" + fp.Role + "::" + schema
}

// unionColumnNames returns the ordered union of AllColumnNames across all files.
// Preserves first-seen order; deduplicates case-insensitively.
func unionColumnNames(files []FileProfile) []string {
	seen := map[string]bool{}
	var result []string
	for _, file := range files {
		for _, name := range file.AllColumnNames {
			key := strings.ToLower(name)
			if !seen[key] {
				seen[key] = true
				result = append(result, name)
			}
		}
	}
	return result
}

func sharedColumns(files []FileProfile) []ColumnProfile {
	if len(files) == 0 || len(files[0].Columns) == 0 {
		return nil
	}
	shared := files[0].Columns
	for _, file := range files[1:] {
		if len(file.Columns) == 0 {
			return nil
		}
		byName := map[string]ColumnProfile{}
		for _, col := range file.Columns {
			byName[strings.ToLower(col.Name)] = col
		}
		var next []ColumnProfile
		for _, col := range shared {
			if _, ok := byName[strings.ToLower(col.Name)]; ok {
				next = append(next, col)
			}
		}
		shared = next
	}
	return shared
}
