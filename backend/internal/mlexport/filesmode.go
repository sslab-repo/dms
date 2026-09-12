package mlexport

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// isDataBearing reports whether a file contributes samples (as opposed to
// annotations, docs, configs, or model artifacts, which ship in raw/ only).
func isDataBearing(f SourceFile) bool {
	switch f.Role {
	case "data", "instruction-data", "train-split", "validation-split", "test-split":
		return true
	}
	return false
}

// fileSample is one line of processed/{train,val,test}.jsonl in files mode.
// Path points into raw/ inside the package: raw data is never duplicated,
// the split manifests just reference it.
type fileSample struct {
	ID        string `json:"id"`
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	Type      string `json:"type"`
}

// convertFiles writes files-mode split manifests: every data-bearing file is
// one sample, assigned to a split by uploader-provided split roles or by the
// deterministic hash rule. Returns the collector and checksums of the
// generated jsonl files.
func convertFiles(
	ctx context.Context,
	files []SourceFile,
	processedDir string,
	provided bool,
) (*splitCollector, map[string]string, error) {
	method := SplitMethodHash
	if provided {
		method = SplitMethodProvided
	}
	collector := newSplitCollector(method)

	samples := map[string][]fileSample{"train": {}, "val": {}, "test": {}}
	for _, f := range files {
		if !isDataBearing(f) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		id := f.zipName
		split := ""
		if provided {
			if split = providedSplitFor(f.Role); split == "" {
				split = "train"
			}
		} else {
			split = hashSplit(id)
		}
		collector.add(split, id)
		samples[split] = append(samples[split], fileSample{
			ID:        id,
			Path:      "raw/" + f.zipName,
			SizeBytes: f.SizeBytes,
			Type:      f.DetectedType,
		})
	}

	checksums := map[string]string{}
	for split, list := range samples {
		relPath := split + ".jsonl"
		f, err := os.Create(filepath.Join(processedDir, relPath))
		if err != nil {
			return nil, nil, err
		}
		hw := newHashingWriter(f)
		enc := json.NewEncoder(hw)
		for _, s := range list {
			if err := enc.Encode(s); err != nil {
				f.Close()
				return nil, nil, fmt.Errorf("write %s: %w", relPath, err)
			}
		}
		if err := f.Close(); err != nil {
			return nil, nil, err
		}
		checksums["processed/"+relPath] = hw.sum()
	}
	return collector, checksums, nil
}
