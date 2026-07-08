package profiler

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/parquet-go/parquet-go"
)

func profileParquet(path string, fp *FileProfile) {
	defer func() {
		if r := recover(); r != nil {
			fp.Warnings = append(fp.Warnings, fmt.Sprintf("Parquet profiling recovered from internal error: %v", r))
			fp.SampleRows = nil
		}
	}()

	file, err := os.Open(path)
	if err != nil {
		fp.Warnings = append(fp.Warnings, "could not open Parquet file")
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		fp.Warnings = append(fp.Warnings, "could not stat Parquet file")
		return
	}

	pf, err := parquet.OpenFile(file, info.Size())
	if err != nil {
		fp.Warnings = append(fp.Warnings, "could not read Parquet metadata")
		return
	}
	if pf.NumRows() > 0 {
		fp.SampledRows = int(minInt64(pf.NumRows(), maxSampleRows))
	}
	allNames := parquetColumnNames(pf.Schema())
	if len(allNames) > 0 {
		fp.AllColumnNames = allNames
	}
	fp.Columns = parquetColumns(pf.Schema())

	reader := parquet.NewReader(pf)
	defer reader.Close()

	rows := make([]parquet.Row, maxSampleRows)
	n, err := reader.ReadRows(rows)
	if err != nil && err != io.EOF {
		fp.Warnings = append(fp.Warnings, "could not sample Parquet rows")
		return
	}

	columnNames := parquetColumnNames(pf.Schema())
	fp.SampleRows = make([]map[string]string, 0, n)
	for _, row := range rows[:n] {
		sample := map[string]string{}
		row.Range(func(columnIndex int, columnValues []parquet.Value) bool {
			if columnIndex >= len(columnNames) || len(sample) >= maxColumns {
				return false
			}
			var values []string
			for _, value := range columnValues {
				if value.IsNull() {
					continue
				}
				values = append(values, value.String())
			}
			if len(values) > 0 {
				sample[columnNames[columnIndex]] = truncate(strings.Join(values, ", "), 120)
			}
			return true
		})
		fp.SampleRows = append(fp.SampleRows, sample)
	}
}

func parquetColumns(schema *parquet.Schema) []ColumnProfile {
	paths := schema.Columns()
	columns := make([]ColumnProfile, 0, minInt(len(paths), maxColumns))
	for _, path := range paths {
		if len(columns) >= maxColumns {
			break
		}
		leaf, ok := schema.Lookup(path...)
		inferredType := "unknown"
		if ok && leaf.Node != nil && leaf.Node.Leaf() {
			inferredType = fmt.Sprint(leaf.Node.Type())
		}
		columns = append(columns, ColumnProfile{
			Name:         strings.Join(path, "."),
			InferredType: inferredType,
		})
	}
	return columns
}

func parquetColumnNames(schema *parquet.Schema) []string {
	paths := schema.Columns()
	names := make([]string, len(paths))
	for i, path := range paths {
		names[i] = strings.Join(path, ".")
	}
	return names
}
