package mlexport

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"
)

func writeTempFile(t *testing.T, dir, name, content string) (string, int64) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path, int64(len(content))
}

func buildTestPackage(t *testing.T, spec *PackageSpec) (*BuildResult, string) {
	t.Helper()
	work := t.TempDir()
	zipPath := filepath.Join(work, "pkg.zip")
	builder := &Builder{}
	result, err := builder.Build(context.Background(), spec, filepath.Join(work, "staging"), zipPath)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return result, zipPath
}

func readZipEntries(t *testing.T, zipPath string) map[string][]byte {
	t.Helper()
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	entries := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		entries[f.Name] = data
	}
	return entries
}

// readParquetRows reads every row of a parquet blob as column-name -> string.
func readParquetRows(t *testing.T, data []byte) []map[string]string {
	t.Helper()
	pf, err := parquet.OpenFile(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open parquet: %v", err)
	}
	fields := pf.Schema().Fields()
	names := make([]string, len(fields))
	for i, f := range fields {
		names[i] = f.Name()
	}
	reader := parquet.NewReader(pf)
	defer reader.Close()

	var out []map[string]string
	buf := make([]parquet.Row, 8)
	for {
		n, err := reader.ReadRows(buf)
		for i := 0; i < n; i++ {
			m := map[string]string{}
			for _, v := range buf[i] {
				m[names[v.Column()]] = v.String()
			}
			out = append(out, m)
		}
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatalf("read parquet rows: %v", err)
		}
		if n == 0 {
			return out
		}
	}
}

func TestBuildTabularPackage(t *testing.T) {
	src := t.TempDir()
	csvContent := "text,label\nhello,pos\nworld,neg\nfoo,pos\nbar,neg\nbaz,pos\n"
	csvPath, csvSize := writeTempFile(t, src, "reviews.csv", csvContent)
	jsonlContent := `{"text":"json row","label":"pos","score":1.50}` + "\n" +
		`{"text":"another","extra":{"nested":true}}` + "\n"
	jsonlPath, jsonlSize := writeTempFile(t, src, "extra.jsonl", jsonlContent)

	prop := 0.6
	spec := &PackageSpec{
		DatasetID:      7,
		Name:           "Review Sentiment! v2",
		Slug:           Slugify("Review Sentiment! v2"),
		ResearcherName: "Dr. Cho",
		AISummary:      "Short reviews with sentiment labels.",
		Modality:       "text",
		DatasetType:    "supervised",
		Labels:         []LabelDef{{Name: "pos", Proportion: &prop, SampleCount: 4}},
		Files: []SourceFile{
			{FileID: 1, OriginalName: "reviews.csv", StoragePath: csvPath, SizeBytes: csvSize, DetectedType: "csv", Role: "data"},
			{FileID: 2, OriginalName: "extra.jsonl", StoragePath: jsonlPath, SizeBytes: jsonlSize, DetectedType: "jsonl", Role: "data"},
		},
		InferredColumnTypes: map[string]string{"label": "categorical"},
	}

	result, zipPath := buildTestPackage(t, spec)
	if result.Mode != "tabular" {
		t.Fatalf("mode = %q, want tabular", result.Mode)
	}
	if result.Counts.Total != 7 {
		t.Fatalf("total samples = %d, want 7", result.Counts.Total)
	}
	if got := result.Counts.Train + result.Counts.Val + result.Counts.Test; got != result.Counts.Total {
		t.Fatalf("split counts %d do not add up to total %d", got, result.Counts.Total)
	}

	entries := readZipEntries(t, zipPath)
	root := spec.Slug + "/"
	for _, want := range []string{
		root + "README.md", root + "manifest.json",
		root + "splits/split_v1.json", root + "scripts/build.py",
		root + "processed/train.parquet", root + "processed/val.parquet", root + "processed/test.parquet",
		root + "raw/reviews.csv", root + "raw/extra.jsonl",
	} {
		if _, ok := entries[want]; !ok {
			t.Errorf("zip missing entry %s", want)
		}
	}

	// Raw files are byte-identical to the source.
	if string(entries[root+"raw/reviews.csv"]) != csvContent {
		t.Error("raw/reviews.csv was modified")
	}

	var manifest Manifest
	if err := json.Unmarshal(entries[root+"manifest.json"], &manifest); err != nil {
		t.Fatalf("manifest.json: %v", err)
	}
	if manifest.ManifestVersion != ManifestVersion {
		t.Errorf("manifest_version = %q", manifest.ManifestVersion)
	}
	sum := sha256.Sum256([]byte(csvContent))
	if got := manifest.Checksums["raw/reviews.csv"]; got != "sha256:"+hex.EncodeToString(sum[:]) {
		t.Errorf("raw checksum mismatch: %s", got)
	}
	if manifest.Schema.IDColumn != SampleIDColumn {
		t.Errorf("id column = %q", manifest.Schema.IDColumn)
	}
	var labelCol *ManifestColumn
	for i := range manifest.Schema.Columns {
		if manifest.Schema.Columns[i].Name == "label" {
			labelCol = &manifest.Schema.Columns[i]
		}
	}
	if labelCol == nil || labelCol.InferredType != "categorical" {
		t.Errorf("label column inferred type not propagated: %+v", labelCol)
	}

	// Split file: explicit IDs, hash method, and assignments match the rule.
	var split splitFile
	if err := json.Unmarshal(entries[root+"splits/split_v1.json"], &split); err != nil {
		t.Fatalf("split_v1.json: %v", err)
	}
	if split.Method != SplitMethodHash || !split.IDsIncluded {
		t.Fatalf("split method=%q ids_included=%v", split.Method, split.IDsIncluded)
	}
	for splitName, ids := range map[string][]string{"train": split.Train, "val": split.Val, "test": split.Test} {
		for _, id := range ids {
			if got := hashSplit(id); got != splitName {
				t.Errorf("id %s recorded in %s but hash rule says %s", id, splitName, got)
			}
		}
	}

	// Parquet rows: read every split back, verify cells and ID mapping.
	rows := map[string]map[string]string{} // sample_id -> row
	var totalRows int
	for _, name := range []string{"train", "val", "test"} {
		for _, row := range readParquetRows(t, entries[root+"processed/"+name+".parquet"]) {
			rows[row[SampleIDColumn]] = row
			totalRows++
		}
	}
	if totalRows != 7 {
		t.Fatalf("parquet rows = %d, want 7", totalRows)
	}
	first := rows["reviews.csv#0"]
	if first == nil || first["text"] != "hello" || first["label"] != "pos" {
		t.Errorf("reviews.csv#0 = %v", first)
	}
	jrow := rows["extra.jsonl#0"]
	if jrow == nil || jrow["score"] != "1.50" {
		t.Errorf("json number lexeme not preserved: %v", jrow)
	}
	nested := rows["extra.jsonl#1"]
	if nested == nil || nested["extra"] != `{"nested":true}` || nested["label"] != "" {
		t.Errorf("nested/missing cell handling wrong: %v", nested)
	}

	if !strings.Contains(string(entries[root+"README.md"]), "## Provenance") {
		t.Error("README.md missing datasheet sections")
	}
}

func TestBuildFilesModePackage(t *testing.T) {
	src := t.TempDir()
	img1Path, img1Size := writeTempFile(t, src, "cat.jpg", "\xff\xd8fakejpeg1")
	img2Path, img2Size := writeTempFile(t, src, "dog.jpg", "\xff\xd8fakejpeg22")
	annPath, annSize := writeTempFile(t, src, "annotations.json", `{"cat.jpg":"cat"}`)

	spec := &PackageSpec{
		DatasetID: 8,
		Name:      "pets",
		Slug:      "pets",
		Files: []SourceFile{
			{FileID: 1, OriginalName: "cat.jpg", StoragePath: img1Path, SizeBytes: img1Size, DetectedType: "image", Role: "data"},
			{FileID: 2, OriginalName: "dog.jpg", StoragePath: img2Path, SizeBytes: img2Size, DetectedType: "image", Role: "data"},
			{FileID: 3, OriginalName: "annotations.json", StoragePath: annPath, SizeBytes: annSize, DetectedType: "json", Role: "annotations"},
		},
	}

	result, zipPath := buildTestPackage(t, spec)
	if result.Mode != "files" {
		t.Fatalf("mode = %q, want files", result.Mode)
	}
	if result.Counts.Total != 2 {
		t.Fatalf("samples = %d, want 2 (annotation file must not be a sample)", result.Counts.Total)
	}

	entries := readZipEntries(t, zipPath)
	var allSamples []fileSample
	for _, name := range []string{"train", "val", "test"} {
		data, ok := entries["pets/processed/"+name+".jsonl"]
		if !ok {
			t.Fatalf("missing processed/%s.jsonl", name)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if line == "" {
				continue
			}
			var s fileSample
			if err := json.Unmarshal([]byte(line), &s); err != nil {
				t.Fatalf("bad jsonl line %q: %v", line, err)
			}
			allSamples = append(allSamples, s)
		}
	}
	if len(allSamples) != 2 {
		t.Fatalf("jsonl samples = %d, want 2", len(allSamples))
	}
	for _, s := range allSamples {
		if _, ok := entries["pets/"+s.Path]; !ok {
			t.Errorf("sample path %s not present in zip", s.Path)
		}
	}
	// Annotation file still ships under raw/.
	if _, ok := entries["pets/raw/annotations.json"]; !ok {
		t.Error("annotations.json missing from raw/")
	}
}

func TestProvidedSplitsRespected(t *testing.T) {
	src := t.TempDir()
	trainPath, trainSize := writeTempFile(t, src, "train.csv", "x,y\n1,a\n2,b\n")
	testPath, testSize := writeTempFile(t, src, "test.csv", "x,y\n3,c\n")

	spec := &PackageSpec{
		DatasetID: 9, Name: "presplit", Slug: "presplit",
		Files: []SourceFile{
			{FileID: 1, OriginalName: "train.csv", StoragePath: trainPath, SizeBytes: trainSize, DetectedType: "csv", Role: "train-split"},
			{FileID: 2, OriginalName: "test.csv", StoragePath: testPath, SizeBytes: testSize, DetectedType: "csv", Role: "test-split"},
		},
	}
	result, zipPath := buildTestPackage(t, spec)
	if result.Counts.Train != 2 || result.Counts.Test != 1 || result.Counts.Val != 0 {
		t.Fatalf("provided split not respected: %+v", result.Counts)
	}
	entries := readZipEntries(t, zipPath)
	var split splitFile
	if err := json.Unmarshal(entries["presplit/splits/split_v1.json"], &split); err != nil {
		t.Fatal(err)
	}
	if split.Method != SplitMethodProvided {
		t.Fatalf("method = %q, want provided", split.Method)
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Review Sentiment! v2": "review-sentiment-v2",
		"  --weird__name--  ":  "weird-name",
		"한국어":                  "dataset",
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}
