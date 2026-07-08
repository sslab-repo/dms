package profiler

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
)

func profileDelimited(path string, comma rune, fp *FileProfile) {
	file, err := os.Open(path)
	if err != nil {
		fp.Warnings = append(fp.Warnings, "could not open file sample")
		return
	}
	defer file.Close()

	reader := csv.NewReader(io.LimitReader(file, maxReadBytes))
	reader.Comma = comma
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = false

	rawHeader, err := reader.Read()
	if err != nil {
		fp.Warnings = append(fp.Warnings, "could not read header row")
		return
	}
	rawHeader = normalizeHeader(rawHeader)

	// Store ALL column names before any truncation so downstream code can
	// detect label-like columns even when they fall outside the sample window.
	if len(rawHeader) > 0 {
		fp.AllColumnNames = make([]string, len(rawHeader))
		copy(fp.AllColumnNames, rawHeader)
	}

	// csvIndices[i] = original CSV column position for profiled column i.
	// selectColumns keeps the first headCols + rescues label-like middles +
	// always includes the last tailCols columns (Option 2 + 3).
	header, csvIndices := selectColumns(rawHeader, fp)

	// Scan up to maxStatRows for value-distribution statistics but only keep
	// the first maxSampleRows rows as display examples in the AI prompt.
	stats := newColumnStats(header)
	for fp.SampledRows < maxStatRows {
		record, err := reader.Read()
		if err != nil {
			break
		}
		fp.SampledRows++
		row := map[string]string{}
		for i, name := range header {
			value := ""
			if csvIndices[i] < len(record) {
				value = strings.TrimSpace(record[csvIndices[i]])
			}
			stats[i].observe(value)
			if len(fp.SampleRows) < maxSampleRows {
				row[name] = truncate(value, 120)
			}
		}
		if len(fp.SampleRows) < maxSampleRows {
			fp.SampleRows = append(fp.SampleRows, row)
		}
	}
	fp.Columns = finalizeColumns(stats)
}

// selectColumns returns the column names to profile and their original CSV
// indices. When the header exceeds maxColumns the selection strategy is:
//   1. Always include the first headCols columns.
//   2. Rescue any label-like column names from the middle range.
//   3. Always include the last tailCols columns (label columns are commonly
//      placed last in ML datasets, so this catches them regardless of name).
//
// The budget is headCols + tailCols = maxColumns. Rescued label-like columns
// are additional beyond the budget.
func selectColumns(header []string, fp *FileProfile) ([]string, []int) {
	if len(header) <= maxColumns {
		indices := make([]int, len(header))
		for i := range header {
			indices[i] = i
		}
		return header, indices
	}

	const tailCols = 3
	const headCols = maxColumns - tailCols // 17

	// Track which original indices are already covered to avoid duplicates.
	covered := make(map[int]bool, maxColumns+4)

	chosen := make([]string, 0, maxColumns+4)
	indices := make([]int, 0, maxColumns+4)

	// First headCols columns.
	for i := 0; i < headCols; i++ {
		chosen = append(chosen, header[i])
		indices = append(indices, i)
		covered[i] = true
	}

	// Rescue label-like columns from the middle (headCols … len-tailCols-1).
	rescued := 0
	middleEnd := len(header) - tailCols
	for i := headCols; i < middleEnd; i++ {
		if !covered[i] && looksLikeLabelColumn(header[i]) {
			chosen = append(chosen, header[i])
			indices = append(indices, i)
			covered[i] = true
			rescued++
		}
	}

	// Last tailCols columns.
	for i := len(header) - tailCols; i < len(header); i++ {
		if !covered[i] {
			chosen = append(chosen, header[i])
			indices = append(indices, i)
			covered[i] = true
		}
	}

	msg := fmt.Sprintf(
		"profile sampled first %d and last %d of %d columns",
		headCols, tailCols, len(header),
	)
	if rescued > 0 {
		msg += fmt.Sprintf("; %d label-like column(s) also rescued", rescued)
	}
	fp.Warnings = append(fp.Warnings, msg)

	return chosen, indices
}
