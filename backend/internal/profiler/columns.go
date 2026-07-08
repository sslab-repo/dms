package profiler

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

func normalizeHeader(header []string) []string {
	normalized := make([]string, len(header))
	for i, name := range header {
		name = strings.TrimSpace(name)
		if name == "" {
			name = fmt.Sprintf("column_%d", i+1)
		}
		normalized[i] = name
	}
	return normalized
}

// maxTrackedValues caps how many unique values are counted per column.
// Avoids memory blowup on high-cardinality columns (UUIDs, free text, etc.).
const maxTrackedValues = 100

type columnStat struct {
	name        string
	nonEmpty    int
	empty       int
	examples    []string
	kinds       map[string]int
	valueCounts map[string]int // nil once maxTrackedValues is exceeded
	overflowed  bool           // true when cardinality exceeded maxTrackedValues
}

func newColumnStats(header []string) []*columnStat {
	stats := make([]*columnStat, len(header))
	for i, name := range header {
		stats[i] = &columnStat{name: name}
	}
	return stats
}

func (s *columnStat) observe(value string) {
	if s.kinds == nil {
		s.kinds = map[string]int{}
	}
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "null") || strings.EqualFold(value, "nan") {
		s.empty++
		return
	}
	s.nonEmpty++
	t := inferScalarType(value)
	s.kinds[t]++

	// Track value counts for low-cardinality columns (strings + small integers).
	if !s.overflowed && (t == "string" || t == "integer" || t == "boolean") {
		if s.valueCounts == nil {
			s.valueCounts = map[string]int{}
		}
		if _, seen := s.valueCounts[value]; seen || len(s.valueCounts) < maxTrackedValues {
			s.valueCounts[value]++
		} else {
			// Too many unique values — stop tracking, not a useful categorical column.
			s.valueCounts = nil
			s.overflowed = true
		}
	}

	if len(s.examples) < 5 && !contains(s.examples, value) {
		s.examples = append(s.examples, truncate(value, 80))
	}
}

func finalizeColumns(stats []*columnStat) []ColumnProfile {
	cols := make([]ColumnProfile, 0, len(stats))
	for _, stat := range stats {
		cols = append(cols, stat.toProfile())
	}
	return cols
}

func finalizeNamedColumns(statsByName map[string]*columnStat) []ColumnProfile {
	names := make([]string, 0, len(statsByName))
	for name := range statsByName {
		names = append(names, name)
	}
	sort.Strings(names)
	cols := make([]ColumnProfile, 0, len(names))
	for _, name := range names {
		cols = append(cols, statsByName[name].toProfile())
	}
	return cols
}

func (s *columnStat) toProfile() ColumnProfile {
	p := ColumnProfile{
		Name:          s.name,
		InferredType:  dominantType(s.kinds),
		NonEmptyCount: s.nonEmpty,
		EmptyCount:    s.empty,
		ExampleValues: s.examples,
	}
	if len(s.valueCounts) > 0 {
		top := make([]ValueCount, 0, len(s.valueCounts))
		for v, c := range s.valueCounts {
			top = append(top, ValueCount{Value: v, Count: c})
		}
		sort.Slice(top, func(i, j int) bool {
			if top[i].Count == top[j].Count {
				return top[i].Value < top[j].Value
			}
			return top[i].Count > top[j].Count
		})
		if len(top) > 20 {
			top = top[:20]
		}
		p.TopValues = top
	}
	return p
}

func dominantType(kinds map[string]int) string {
	if len(kinds) == 0 {
		return "unknown"
	}
	bestKind := "unknown"
	bestCount := -1
	for kind, count := range kinds {
		if count > bestCount {
			bestKind = kind
			bestCount = count
		}
	}
	return bestKind
}

func inferScalarType(value string) string {
	if _, err := strconv.ParseInt(value, 10, 64); err == nil {
		return "integer"
	}
	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return "number"
	}
	if _, err := time.Parse(time.RFC3339, value); err == nil {
		return "datetime"
	}
	if _, err := time.Parse("2006-01-02", value); err == nil {
		return "date"
	}
	lower := strings.ToLower(value)
	if lower == "true" || lower == "false" {
		return "boolean"
	}
	return "string"
}
