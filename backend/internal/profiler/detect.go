package profiler

import "strings"

func detectType(ext, mimeType, name string) string {
	ext = strings.ToLower(ext)
	mimeType = strings.ToLower(mimeType)
	switch ext {
	case ".csv":
		return "csv"
	case ".tsv", ".conll", ".conllu":
		return "tsv" // tab-separated; CoNLL profiled as TSV
	case ".jsonl", ".ndjson":
		return "jsonl"
	case ".json", ".geojson":
		return "json"
	case ".xml":
		return "xml"
	case ".txt", ".md", ".log", ".text", ".yaml", ".yml":
		return "text"
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".tif", ".tiff":
		return "image"
	case ".wav", ".mp3", ".flac", ".ogg", ".m4a", ".opus":
		return "audio"
	case ".mp4", ".mov", ".avi", ".mkv":
		return "video"
	case ".parquet":
		return "parquet"
	case ".h5", ".hdf5":
		return "hdf5"
	case ".tfrecord", ".tfrecords":
		return "tfrecord"
	case ".arrow", ".feather":
		return "arrow"
	case ".warc", ".wet", ".wat":
		return "webarchive"
	case ".pt", ".pth", ".ckpt", ".onnx", ".safetensors", ".bin":
		return "model-or-binary"
	}
	if strings.Contains(mimeType, "text/") {
		return "text"
	}
	if strings.Contains(mimeType, "json") {
		return "json"
	}
	if strings.Contains(mimeType, "image/") {
		return "image"
	}
	if strings.Contains(strings.ToLower(name), "readme") {
		return "text"
	}
	return "unknown"
}

// looksLikeLabelColumn returns true for column names that are likely
// label/target/class fields. Used to preserve them even when the column
// limit would otherwise cut them off.
func looksLikeLabelColumn(name string) bool {
	parts := strings.FieldsFunc(strings.ToLower(strings.TrimSpace(name)), func(r rune) bool {
		return r == ' ' || r == '-' || r == '.' || r == '/'
	})
	norm := strings.Join(parts, "_")
	switch norm {
	case "label", "labels", "class", "classes", "target", "targets",
		"outcome", "outcomes", "sentiment", "diagnosis", "ground_truth",
		"groundtruth", "answer", "answers", "y":
		return true
	}
	return strings.Contains(norm, "label") ||
		strings.Contains(norm, "ground_truth") ||
		strings.HasSuffix(norm, "_class") ||
		strings.HasSuffix(norm, "_target") ||
		strings.HasSuffix(norm, "_outcome")
}

func inferRole(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "annotation"), strings.Contains(lower, "annot"),
		strings.Contains(lower, "label"), strings.Contains(lower, "bbox"),
		strings.Contains(lower, "coco"), strings.Contains(lower, "yolo"),
		strings.Contains(lower, "groundtruth"), strings.Contains(lower, "ground_truth"),
		strings.Contains(lower, "classes"), strings.HasSuffix(lower, ".names"):
		return "annotations"
	case strings.Contains(lower, "train"):
		return "train-split"
	case strings.Contains(lower, "valid"), strings.Contains(lower, "val"):
		return "validation-split"
	case strings.Contains(lower, "test"):
		return "test-split"
	case strings.Contains(lower, "readme"), strings.Contains(lower, "metadata"):
		return "documentation"
	case strings.Contains(lower, "config"), strings.Contains(lower, "params"):
		return "configuration"
	case strings.Contains(lower, "model"), strings.Contains(lower, "checkpoint"), strings.Contains(lower, "weights"):
		return "model-artifact"
	case strings.Contains(lower, "prompt"), strings.Contains(lower, "instruction"),
		strings.Contains(lower, "alpaca"), strings.Contains(lower, "sharegpt"),
		strings.Contains(lower, "conversation"), strings.Contains(lower, "dialogue"),
		strings.Contains(lower, "preference"), strings.Contains(lower, "reward"),
		strings.Contains(lower, "chosen"), strings.Contains(lower, "rejected"):
		return "instruction-data"
	default:
		return "data"
	}
}
