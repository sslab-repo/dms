package mlexport

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Builder produces one ML package. Progress (optional) receives monotonically
// increasing fractions in [0,1].
type Builder struct {
	Progress func(fraction float64)
}

// Stage weights for progress reporting: hashing raw files, converting to
// processed outputs, and zipping. Generated text files are negligible.
const (
	weightChecksum = 0.30
	weightConvert  = 0.40
	weightZip      = 0.30
)

// Build assembles the package for spec, staging generated files in workDir
// and writing the final archive to zipPath. Raw files are streamed into the
// zip straight from their storage paths — they are never copied on disk.
func (b *Builder) Build(ctx context.Context, spec *PackageSpec, workDir, zipPath string) (*BuildResult, error) {
	assignZipNames(spec.Files)

	processedDir := filepath.Join(workDir, "processed")
	splitsDir := filepath.Join(workDir, "splits")
	scriptsDir := filepath.Join(workDir, "scripts")
	for _, dir := range []string{processedDir, splitsDir, scriptsDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}

	report := func(f float64) {
		if b.Progress != nil {
			if f > 1 {
				f = 1
			}
			b.Progress(f)
		}
	}

	// Stage 1: checksum every raw file.
	var totalRawBytes int64
	for _, f := range spec.Files {
		totalRawBytes += f.SizeBytes
	}
	checksums := map[string]string{}
	rawChecksums := map[int]string{}
	var hashedBytes int64
	for i := range spec.Files {
		f := &spec.Files[i]
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		sum, _, err := hashFile(f.StoragePath)
		if err != nil {
			return nil, fmt.Errorf("checksum %s: %w", f.zipName, err)
		}
		rawChecksums[f.FileID] = sum
		checksums["raw/"+f.zipName] = "sha256:" + sum
		hashedBytes += f.SizeBytes
		if totalRawBytes > 0 {
			report(weightChecksum * float64(hashedBytes) / float64(totalRawBytes))
		}
	}
	report(weightChecksum)

	// Stage 2: build processed/.
	mode, tabularFiles := decideMode(spec.Files)
	provided := useProvidedSplits(spec.Files)

	var collector *splitCollector
	var fieldOrder []string
	var processedChecksums map[string]string
	var err error
	if mode == "tabular" {
		var tabularBytes int64
		for _, f := range tabularFiles {
			tabularBytes += f.SizeBytes
		}
		var readBytes int64
		onBytes := func(n int64) {
			readBytes += n
			if tabularBytes > 0 {
				report(weightChecksum + weightConvert*float64(readBytes)/float64(tabularBytes))
			}
		}
		collector, fieldOrder, processedChecksums, err = convertTabular(ctx, tabularFiles, processedDir, provided, onBytes)
	} else {
		collector, processedChecksums, err = convertFiles(ctx, spec.Files, processedDir, provided)
	}
	if err != nil {
		return nil, fmt.Errorf("build processed outputs: %w", err)
	}
	for path, sum := range processedChecksums {
		checksums[path] = "sha256:" + sum
	}
	report(weightChecksum + weightConvert)

	// Stage 3: splits, manifest, datasheet, build script.
	idScheme := "<raw file name>"
	if mode == "tabular" {
		idScheme = "<raw file name>#<0-based row index>"
	}
	splitJSON, err := marshalIndented(collector.toFile(idScheme))
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(splitsDir, SplitFileName), splitJSON, 0644); err != nil {
		return nil, err
	}

	manifest := buildManifest(spec, mode, fieldOrder, collector, checksums)
	manifestJSON, err := marshalIndented(manifest)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(workDir, "manifest.json"), manifestJSON, 0644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(workDir, "README.md"), []byte(renderDatasheet(spec, manifest)), 0644); err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "build.py"), []byte(buildPyScript), 0755); err != nil {
		return nil, err
	}

	// Stage 4: zip everything under a "<slug>/" root.
	zipProgress := func(done, total int64) {
		if total > 0 {
			report(weightChecksum + weightConvert + weightZip*float64(done)/float64(total))
		}
	}
	if err := writePackageZip(ctx, spec, workDir, zipPath, zipProgress); err != nil {
		return nil, fmt.Errorf("write zip: %w", err)
	}
	report(1)

	return &BuildResult{
		ZipPath:      zipPath,
		Mode:         mode,
		Counts:       collector.Counts,
		RawChecksums: rawChecksums,
	}, nil
}

// decideMode picks "tabular" when every data-bearing file is convertible
// delimited/JSONL content; anything else (images, audio, mixed content, or
// no data files at all) uses files mode.
func decideMode(files []SourceFile) (string, []SourceFile) {
	var tabular []SourceFile
	allTabular := true
	anyData := false
	for _, f := range files {
		if !isDataBearing(f) {
			continue
		}
		anyData = true
		if isTabularData(f) {
			tabular = append(tabular, f)
		} else {
			allTabular = false
		}
	}
	if anyData && allTabular {
		return "tabular", tabular
	}
	return "files", nil
}

func buildManifest(spec *PackageSpec, mode string, fieldOrder []string, collector *splitCollector, checksums map[string]string) *Manifest {
	m := &Manifest{
		ManifestVersion: ManifestVersion,
		GeneratedAt:     time.Now().UTC(),
		Dataset: ManifestDataset{
			ID:                spec.DatasetID,
			Name:              spec.Name,
			Researcher:        spec.ResearcherName,
			UploadedAt:        spec.UploadedAt,
			Modality:          spec.Modality,
			DatasetType:       spec.DatasetType,
			AnnotationFormat:  spec.AnnotationFormat,
			LabelCompleteness: spec.LabelCompleteness,
			Tags:              spec.Tags,
		},
		Counts: ManifestCounts{Files: len(spec.Files), Samples: collector.Counts},
		Schema: ManifestSchema{Mode: mode},
		Labels: spec.Labels,
		Split: ManifestSplit{
			Method:   collector.Method,
			Seed:     SplitSeed,
			IDScheme: "<raw file name>",
			File:     "splits/" + SplitFileName,
		},
		Checksums: checksums,
	}
	if m.Labels == nil {
		m.Labels = []LabelDef{}
	}
	if m.Dataset.Tags == nil {
		m.Dataset.Tags = []string{}
	}
	if collector.Method == SplitMethodHash {
		m.Split.Ratios = map[string]float64{"train": TrainRatio, "val": ValRatio, "test": 1 - TrainRatio - ValRatio}
	}
	if mode == "tabular" {
		m.Split.IDScheme = "<raw file name>#<0-based row index>"
		m.Schema.Note = "All values are stored as strings; inferred_type is advisory, from sampled profiling."
		if len(fieldOrder) > 0 {
			m.Schema.IDColumn = fieldOrder[0]
			for _, name := range fieldOrder {
				if name == SampleIDColumn || name == SampleIDColumnFallback {
					m.Schema.IDColumn = name
					break
				}
			}
			for _, name := range fieldOrder {
				col := ManifestColumn{Name: name, StoredType: "string"}
				if name != m.Schema.IDColumn {
					col.InferredType = spec.InferredColumnTypes[name]
				}
				m.Schema.Columns = append(m.Schema.Columns, col)
			}
		}
	}
	for _, f := range spec.Files {
		m.Files = append(m.Files, ManifestFile{
			Name:         f.zipName,
			OriginalName: f.OriginalName,
			Path:         "raw/" + f.zipName,
			SizeBytes:    f.SizeBytes,
			Sha256:       checksums["raw/"+f.zipName],
			DetectedType: f.DetectedType,
			Role:         f.Role,
		})
	}
	return m
}

// alreadyCompressed lists detected types where zip deflate would waste CPU.
func alreadyCompressed(detectedType string) bool {
	switch detectedType {
	case "image", "audio", "video", "parquet", "hdf5", "tfrecord", "arrow", "model-or-binary", "webarchive":
		return true
	}
	return false
}

// writePackageZip streams the staged generated files plus every raw file
// into zipPath under a "<slug>/" root directory.
func writePackageZip(ctx context.Context, spec *PackageSpec, workDir, zipPath string, progress func(done, total int64)) error {
	out, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer out.Close()
	zw := zip.NewWriter(out)

	var total, done int64
	var staged []string
	err = filepath.Walk(workDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		staged = append(staged, path)
		total += info.Size()
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(staged)
	for _, f := range spec.Files {
		total += f.SizeBytes
	}

	now := time.Now().UTC()
	addFile := func(entryName, srcPath string, method uint16) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		src, err := os.Open(srcPath)
		if err != nil {
			return err
		}
		defer src.Close()
		w, err := zw.CreateHeader(&zip.FileHeader{
			Name:     spec.Slug + "/" + entryName,
			Method:   method,
			Modified: now,
		})
		if err != nil {
			return err
		}
		counting := &countingReader{r: src, onBytes: func(n int64) {
			done += n
			progress(done, total)
		}}
		_, err = io.Copy(w, counting)
		return err
	}

	for _, path := range staged {
		rel, err := filepath.Rel(workDir, path)
		if err != nil {
			return err
		}
		if err := addFile(filepath.ToSlash(rel), path, zip.Deflate); err != nil {
			return fmt.Errorf("zip %s: %w", rel, err)
		}
	}
	for _, f := range spec.Files {
		method := uint16(zip.Deflate)
		if alreadyCompressed(f.DetectedType) {
			method = zip.Store
		}
		if err := addFile("raw/"+f.zipName, f.StoragePath, method); err != nil {
			return fmt.Errorf("zip raw/%s: %w", f.zipName, err)
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return out.Close()
}
