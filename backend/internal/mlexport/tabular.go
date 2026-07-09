package mlexport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/parquet-go/parquet-go"
)

// SampleIDColumn is the extra column added to every processed parquet row so
// rows can be joined back to splits/split_v1.json. If the source data already
// has a column with this name, the fallback name is used instead.
const (
	SampleIDColumn         = "sample_id"
	SampleIDColumnFallback = "_dms_sample_id"
)

// isTabularData reports whether a file should be converted into the
// processed parquet splits: delimited or JSONL content in a data-bearing role.
func isTabularData(f SourceFile) bool {
	switch f.DetectedType {
	case "csv", "tsv", "jsonl":
	default:
		return false
	}
	switch f.Role {
	case "data", "instruction-data", "train-split", "validation-split", "test-split":
		return true
	}
	return false
}

// rowSource streams one tabular file as (columns seen, rows as string maps).
type rowSource interface {
	// scanColumns returns every column name the file contributes.
	scanColumns(ctx context.Context) ([]string, error)
	// emit streams rows in file order. onBytes reports read progress.
	emit(ctx context.Context, onBytes func(int64), yield func(map[string]string) error) error
}

func newRowSource(f SourceFile) rowSource {
	if f.DetectedType == "jsonl" {
		return &jsonlSource{path: f.StoragePath}
	}
	comma := ','
	if f.DetectedType == "tsv" {
		comma = '\t'
	}
	return &delimitedSource{path: f.StoragePath, comma: comma}
}

// ---- delimited (CSV / TSV) ----

type delimitedSource struct {
	path  string
	comma rune
}

func (s *delimitedSource) header() ([]string, error) {
	f, err := os.Open(s.path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.Comma = s.comma
	r.FieldsPerRecord = -1
	raw, err := r.Read()
	if err != nil {
		if err == io.EOF {
			return nil, nil // empty file contributes no columns
		}
		return nil, fmt.Errorf("read header of %s: %w", s.path, err)
	}
	return dedupeColumns(raw), nil
}

func (s *delimitedSource) scanColumns(ctx context.Context) ([]string, error) {
	return s.header()
}

func (s *delimitedSource) emit(ctx context.Context, onBytes func(int64), yield func(map[string]string) error) error {
	f, err := os.Open(s.path)
	if err != nil {
		return err
	}
	defer f.Close()

	counting := &countingReader{r: f, onBytes: onBytes}
	r := csv.NewReader(counting)
	r.Comma = s.comma
	r.FieldsPerRecord = -1
	r.ReuseRecord = true

	header, err := r.Read()
	if err != nil {
		if err == io.EOF {
			return nil
		}
		return fmt.Errorf("read header: %w", err)
	}
	columns := dedupeColumns(header)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		record, err := r.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read row: %w", err)
		}
		row := make(map[string]string, len(columns))
		for i, name := range columns {
			if i < len(record) {
				row[name] = record[i]
			} else {
				row[name] = ""
			}
		}
		if err := yield(row); err != nil {
			return err
		}
	}
}

// ---- JSONL ----

type jsonlSource struct {
	path string
}

func (s *jsonlSource) decodeAll(ctx context.Context, onBytes func(int64), yield func(map[string]string) error) error {
	f, err := os.Open(s.path)
	if err != nil {
		return err
	}
	defer f.Close()

	counting := &countingReader{r: f, onBytes: onBytes}
	dec := json.NewDecoder(counting)
	dec.UseNumber()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		var v any
		if err := dec.Decode(&v); err == io.EOF {
			return nil
		} else if err != nil {
			return fmt.Errorf("decode JSONL value: %w", err)
		}
		if err := yield(jsonValueToRow(v)); err != nil {
			return err
		}
	}
}

func (s *jsonlSource) scanColumns(ctx context.Context) ([]string, error) {
	seen := map[string]bool{}
	var columns []string
	err := s.decodeAll(ctx, nil, func(row map[string]string) error {
		for k := range row {
			if !seen[k] {
				seen[k] = true
				columns = append(columns, k)
			}
		}
		return nil
	})
	return columns, err
}

func (s *jsonlSource) emit(ctx context.Context, onBytes func(int64), yield func(map[string]string) error) error {
	return s.decodeAll(ctx, onBytes, yield)
}

// jsonValueToRow flattens one JSONL record into string cells: top-level
// scalars keep their text form, nested structures are re-encoded as compact
// JSON, and non-object records land in a single "value" column.
func jsonValueToRow(v any) map[string]string {
	obj, ok := v.(map[string]any)
	if !ok {
		return map[string]string{"value": jsonCellString(v)}
	}
	row := make(map[string]string, len(obj))
	for k, val := range obj {
		row[k] = jsonCellString(val)
	}
	return row
}

func jsonCellString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		// Nested values are re-encoded as compact JSON with sorted keys and
		// no HTML escaping. build.py mirrors this encoding exactly (numbers
		// keep their original lexeme thanks to UseNumber above), so the
		// rebuild script reproduces these cells byte-for-byte.
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(t); err != nil {
			return fmt.Sprintf("%v", t)
		}
		return strings.TrimSuffix(buf.String(), "\n")
	}
}

// ---- conversion driver ----

// convertTabular streams every tabular data file into per-split parquet
// files under processedDir, assigning each row a stable sample ID
// "<raw name>#<row index>". Returns the split collector, the final parquet
// column order, and sha256 checksums of the written parquet files.
func convertTabular(
	ctx context.Context,
	files []SourceFile,
	processedDir string,
	provided bool,
	onBytes func(int64),
) (*splitCollector, []string, map[string]string, error) {
	// Pass 1: union of columns across files (header-only for CSV/TSV,
	// full scan for JSONL since keys can vary per line).
	seen := map[string]bool{}
	var union []string
	for _, f := range files {
		cols, err := newRowSource(f).scanColumns(ctx)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("%s: %w", f.zipName, err)
		}
		for _, c := range cols {
			if !seen[c] {
				seen[c] = true
				union = append(union, c)
			}
		}
	}
	sort.Strings(union)

	idColumn := SampleIDColumn
	if seen[idColumn] {
		idColumn = SampleIDColumnFallback
	}
	columns := append([]string{idColumn}, union...)
	sort.Strings(columns) // parquet-go orders group fields alphabetically; match it

	group := parquet.Group{}
	for _, name := range columns {
		group[name] = parquet.String()
	}
	schema := parquet.NewSchema("sample", group)

	// Field order of the built schema is authoritative for row assembly.
	fieldOrder := make([]string, 0, len(columns))
	for _, f := range schema.Fields() {
		fieldOrder = append(fieldOrder, f.Name())
	}

	writers := map[string]*splitParquetWriter{}
	for _, split := range []string{"train", "val", "test"} {
		w, err := newSplitParquetWriter(processedDir, split, schema)
		if err != nil {
			return nil, nil, nil, err
		}
		writers[split] = w
	}
	defer func() {
		for _, w := range writers {
			w.abort()
		}
	}()

	method := SplitMethodHash
	if provided {
		method = SplitMethodProvided
	}
	collector := newSplitCollector(method)

	row := make(parquet.Row, len(fieldOrder))
	for _, f := range files {
		fileSplit := ""
		if provided {
			if fileSplit = providedSplitFor(f.Role); fileSplit == "" {
				fileSplit = "train" // unsplit data files default to train
			}
		}
		rowIdx := 0
		zipName := f.zipName
		err := newRowSource(f).emit(ctx, onBytes, func(cells map[string]string) error {
			id := fmt.Sprintf("%s#%d", zipName, rowIdx)
			rowIdx++
			split := fileSplit
			if !provided {
				split = hashSplit(id)
			}
			collector.add(split, id)

			for i, name := range fieldOrder {
				value := cells[name]
				if name == idColumn {
					value = id
				}
				row[i] = parquet.ByteArrayValue([]byte(value)).Level(0, 0, i)
			}
			return writers[split].writeRow(row)
		})
		if err != nil {
			return nil, nil, nil, fmt.Errorf("%s: %w", f.zipName, err)
		}
	}

	checksums := map[string]string{}
	for split, w := range writers {
		sum, err := w.finish()
		if err != nil {
			return nil, nil, nil, fmt.Errorf("finalize %s.parquet: %w", split, err)
		}
		checksums["processed/"+split+".parquet"] = sum
	}
	return collector, fieldOrder, checksums, nil
}

// splitParquetWriter writes one split's parquet file while hashing it.
type splitParquetWriter struct {
	file    *os.File
	writer  *parquet.GenericWriter[any]
	hasher  *hashingWriter
	done    bool
	relPath string
}

func newSplitParquetWriter(dir, split string, schema *parquet.Schema) (*splitParquetWriter, error) {
	f, err := os.Create(filepath.Join(dir, split+".parquet"))
	if err != nil {
		return nil, err
	}
	hw := newHashingWriter(f)
	return &splitParquetWriter{
		file:    f,
		hasher:  hw,
		writer:  parquet.NewGenericWriter[any](hw, schema, parquet.Compression(&parquet.Snappy)),
		relPath: split + ".parquet",
	}, nil
}

func (w *splitParquetWriter) writeRow(row parquet.Row) error {
	_, err := w.writer.WriteRows([]parquet.Row{row})
	return err
}

func (w *splitParquetWriter) finish() (string, error) {
	if err := w.writer.Close(); err != nil {
		w.file.Close()
		return "", err
	}
	if err := w.file.Close(); err != nil {
		return "", err
	}
	w.done = true
	return w.hasher.sum(), nil
}

func (w *splitParquetWriter) abort() {
	if !w.done {
		w.file.Close()
	}
}

// ---- small IO helpers ----

type countingReader struct {
	r       io.Reader
	onBytes func(int64)
}

func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	if n > 0 && c.onBytes != nil {
		c.onBytes(int64(n))
	}
	return n, err
}

type hashingWriter struct {
	w io.Writer
	h interface {
		io.Writer
		Sum([]byte) []byte
	}
}

func newHashingWriter(w io.Writer) *hashingWriter {
	return &hashingWriter{w: w, h: sha256.New()}
}

func (h *hashingWriter) Write(p []byte) (int, error) {
	h.h.Write(p)
	return h.w.Write(p)
}

func (h *hashingWriter) sum() string {
	return fmt.Sprintf("%x", h.h.Sum(nil))
}

// dedupeColumns trims whitespace and makes duplicate header names unique by
// suffixing ".2", ".3", ... so map-based rows never silently drop a column.
func dedupeColumns(raw []string) []string {
	used := map[string]bool{}
	out := make([]string, len(raw))
	for i, name := range raw {
		name = strings.TrimSpace(name)
		if name == "" {
			name = fmt.Sprintf("column_%d", i+1)
		}
		candidate := name
		for n := 2; used[candidate]; n++ {
			candidate = fmt.Sprintf("%s.%d", name, n)
		}
		used[candidate] = true
		out[i] = candidate
	}
	return out
}
