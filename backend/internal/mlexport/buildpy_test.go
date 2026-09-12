package mlexport

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// TestBuildPyReproducesProcessed verifies the package's determinism promise:
// running scripts/build.py over raw/ must reproduce exactly the rows and
// split assignment that the Go builder wrote to processed/.
//
// Requires a Python with pyarrow; point MLEXPORT_PYTHON at it, e.g.
//
//	MLEXPORT_PYTHON=/path/to/venv/bin/python go test ./internal/mlexport/
//
// The test is skipped when the variable is unset.
func TestBuildPyReproducesProcessed(t *testing.T) {
	python := os.Getenv("MLEXPORT_PYTHON")
	if python == "" {
		t.Skip("MLEXPORT_PYTHON not set; skipping build.py reproduction test")
	}

	// Enough rows to populate all three splits under the hash rule, plus
	// JSONL quirks: varying keys, number lexemes, nested values.
	src := t.TempDir()
	var csvRows strings.Builder
	csvRows.WriteString("text,label\n")
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&csvRows, "sample text %d,label-%d\n", i, i%3)
	}
	csvPath, csvSize := writeTempFile(t, src, "data.csv", csvRows.String())
	jsonl := `{"text":"a","score":1.50}` + "\n" +
		`{"text":"b","extra":{"k":[1,2]}}` + "\n" +
		`{"text":"c","flag":true,"gone":null}` + "\n"
	jsonlPath, jsonlSize := writeTempFile(t, src, "more.jsonl", jsonl)

	spec := &PackageSpec{
		DatasetID: 11, Name: "repro", Slug: "repro",
		Files: []SourceFile{
			{FileID: 1, OriginalName: "data.csv", StoragePath: csvPath, SizeBytes: csvSize, DetectedType: "csv", Role: "data"},
			{FileID: 2, OriginalName: "more.jsonl", StoragePath: jsonlPath, SizeBytes: jsonlSize, DetectedType: "jsonl", Role: "data"},
		},
	}
	work := t.TempDir()
	zipPath := filepath.Join(work, "pkg.zip")
	if _, err := (&Builder{}).Build(context.Background(), spec, filepath.Join(work, "staging"), zipPath); err != nil {
		t.Fatalf("Build: %v", err)
	}

	// Extract the package.
	extractDir := t.TempDir()
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		dest := filepath.Join(extractDir, f.Name)
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			t.Fatal(err)
		}
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dest, data, 0644); err != nil {
			t.Fatal(err)
		}
	}
	zr.Close()
	pkgRoot := filepath.Join(extractDir, "repro")

	// Run the shipped build script into a fresh directory.
	rebuiltDir := filepath.Join(pkgRoot, "rebuilt")
	cmd := exec.Command(python, filepath.Join(pkgRoot, "scripts", "build.py"), "--out", rebuiltDir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build.py failed: %v\n%s", err, out)
	}

	// Compare row sets per split.
	for _, split := range []string{"train", "val", "test"} {
		want := readParquetFileRows(t, filepath.Join(pkgRoot, "processed", split+".parquet"))
		got := readParquetFileRows(t, filepath.Join(rebuiltDir, split+".parquet"))
		if !reflect.DeepEqual(want, got) {
			t.Errorf("%s.parquet differs:\n go: %v\n py: %v", split, want, got)
		}
	}
}

// readParquetFileRows reads all rows sorted by sample ID for comparison.
func readParquetFileRows(t *testing.T, path string) []map[string]string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	rows := readParquetRows(t, data)
	sort.Slice(rows, func(i, j int) bool {
		return rows[i][SampleIDColumn] < rows[j][SampleIDColumn]
	})
	return rows
}
