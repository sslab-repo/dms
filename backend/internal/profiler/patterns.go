package profiler

func detectDatasetPatterns(groups []FileGroup) []string {
	var hasImage, hasAudio, hasVideo, hasAnnotations, hasSplits, hasParquet bool
	for _, group := range groups {
		switch group.DetectedType {
		case "image":
			hasImage = true
		case "audio":
			hasAudio = true
		case "video":
			hasVideo = true
		case "parquet":
			hasParquet = true
		}
		switch group.Role {
		case "annotations":
			hasAnnotations = true
		case "train-split", "validation-split", "test-split":
			hasSplits = true
		}
	}

	var patterns []string
	if hasImage && hasAnnotations {
		patterns = append(patterns, "image dataset with annotation files")
	}
	if hasAudio && hasAnnotations {
		patterns = append(patterns, "audio dataset with annotation files")
	}
	if hasVideo && hasAnnotations {
		patterns = append(patterns, "video dataset with annotation files")
	}
	if hasSplits {
		patterns = append(patterns, "dataset appears to include train/validation/test split files")
	}
	if hasParquet {
		patterns = append(patterns, "tabular dataset includes Parquet files with extracted schema")
	}
	return patterns
}
