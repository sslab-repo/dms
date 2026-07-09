package mlexport

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// Split configuration. The seed and ratios are recorded in manifest.json and
// splits/split_v1.json, and scripts/build.py implements the identical rule,
// so the assignment is reproducible outside DMS.
const (
	SplitSeed       = 42
	TrainRatio      = 0.8
	ValRatio        = 0.1
	SplitFileName   = "split_v1.json"
	SplitMethodHash = "hash"
	// SplitMethodProvided means the uploader shipped explicit train/val/test
	// files (detected by the profiler) and we honored that assignment.
	SplitMethodProvided = "provided"

	// maxExplicitSplitIDs caps how many sample IDs are written verbatim into
	// splits/split_v1.json. Beyond this the file records only the rule and
	// counts; the assignment stays fully deterministic via the hash rule and
	// the sample_id column in the processed output.
	maxExplicitSplitIDs = 250_000
)

// hashSplit deterministically assigns a sample ID to a split.
// Rule: frac = uint64(sha256("<seed>:<id>")[0:8]) / 2^64;
// frac < 0.8 -> train, < 0.9 -> val, else test.
func hashSplit(id string) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d:%s", SplitSeed, id)))
	frac := float64(binary.BigEndian.Uint64(sum[:8])) / float64(1<<63) / 2
	switch {
	case frac < TrainRatio:
		return "train"
	case frac < TrainRatio+ValRatio:
		return "val"
	default:
		return "test"
	}
}

// providedSplitFor maps a profiler role to a split name, or "" when the role
// does not pin the file to a specific split.
func providedSplitFor(role string) string {
	switch role {
	case "train-split":
		return "train"
	case "validation-split":
		return "val"
	case "test-split":
		return "test"
	}
	return ""
}

// useProvidedSplits reports whether the uploader shipped their own split
// files: at least two distinct splits must be present among the data-bearing
// files, otherwise a stray "train" in a filename would dump everything into
// one split.
func useProvidedSplits(files []SourceFile) bool {
	seen := map[string]bool{}
	for _, f := range files {
		if s := providedSplitFor(f.Role); s != "" {
			seen[s] = true
		}
	}
	return len(seen) >= 2
}

// splitCollector accumulates sample IDs per split while conversion streams
// rows, keeping explicit ID lists only below maxExplicitSplitIDs.
type splitCollector struct {
	Method string
	Counts SplitCounts
	ids    map[string][]string
	capped bool
}

func newSplitCollector(method string) *splitCollector {
	return &splitCollector{
		Method: method,
		ids:    map[string][]string{"train": {}, "val": {}, "test": {}},
	}
}

func (c *splitCollector) add(split, id string) {
	switch split {
	case "train":
		c.Counts.Train++
	case "val":
		c.Counts.Val++
	default:
		c.Counts.Test++
	}
	c.Counts.Total++
	if c.capped {
		return
	}
	if c.Counts.Total > maxExplicitSplitIDs {
		c.capped = true
		c.ids = nil
		return
	}
	c.ids[split] = append(c.ids[split], id)
}

// splitFile is the JSON shape of splits/split_v1.json.
type splitFile struct {
	Version  string             `json:"version"`
	Method   string             `json:"method"`
	Seed     int                `json:"seed"`
	Ratios   map[string]float64 `json:"ratios,omitempty"`
	IDScheme string             `json:"id_scheme"`
	Counts   SplitCounts        `json:"counts"`
	// IDsIncluded is false for very large datasets, where listing every
	// sample would bloat the package; the rule above still fully determines
	// membership, and processed rows carry their sample_id.
	IDsIncluded bool     `json:"ids_included"`
	Train       []string `json:"train,omitempty"`
	Val         []string `json:"val,omitempty"`
	Test        []string `json:"test,omitempty"`
}

func (c *splitCollector) toFile(idScheme string) *splitFile {
	sf := &splitFile{
		Version:     "split_v1",
		Method:      c.Method,
		Seed:        SplitSeed,
		IDScheme:    idScheme,
		Counts:      c.Counts,
		IDsIncluded: !c.capped,
	}
	if c.Method == SplitMethodHash {
		sf.Ratios = map[string]float64{"train": TrainRatio, "val": ValRatio, "test": 1 - TrainRatio - ValRatio}
	}
	if !c.capped {
		sf.Train = c.ids["train"]
		sf.Val = c.ids["val"]
		sf.Test = c.ids["test"]
	}
	return sf
}
