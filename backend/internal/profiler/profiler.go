package profiler

import (
	"context"
	"fmt"
	"sort"
	"time"

	"dataset-platform/backend/internal/filehandler"
)

const (
	maxProfiledFiles       = 30
	maxRepresentativeFiles = 3
	maxColumns             = 20
	maxSampleRows          = 8    // rows shown as examples in the AI prompt
	maxStatRows            = 2000 // rows scanned for value-distribution statistics
	maxTextLines           = 12
	maxReadBytes           = 8 * 1024 * 1024 // 8 MB — enough for 2000 rows of wide CSV
)

func ProfileDataset(ctx context.Context, files []filehandler.AssembledFile) (*DatasetProfile, error) {
	profile := &DatasetProfile{
		Version:     ProfileVersion,
		GeneratedAt: time.Now().UTC(),
		TotalFiles:  len(files),
		Notes: []string{
			"Profile is generated from file metadata and bounded samples, not the full dataset contents.",
		},
	}

	typeStats := map[string]*TypeSummary{}
	groupBuckets := map[string][]FileProfile{}
	allProfiles := make([]FileProfile, 0, len(files))

	for i, f := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		profile.TotalSizeBytes += f.SizeBytes
		fp := profileFile(f)
		allProfiles = append(allProfiles, fp)

		stat := typeStats[fp.DetectedType]
		if stat == nil {
			stat = &TypeSummary{DetectedType: fp.DetectedType}
			typeStats[fp.DetectedType] = stat
		}
		stat.FileCount++
		stat.TotalSizeBytes += fp.SizeBytes

		groupKey := buildGroupKey(fp)
		groupBuckets[groupKey] = append(groupBuckets[groupKey], fp)

		if i < maxProfiledFiles {
			profile.Files = append(profile.Files, fp)
		}
	}

	for _, stat := range typeStats {
		profile.FileTypes = append(profile.FileTypes, *stat)
	}
	sort.Slice(profile.FileTypes, func(i, j int) bool {
		return profile.FileTypes[i].FileCount > profile.FileTypes[j].FileCount
	})

	for key, bucket := range groupBuckets {
		profile.Groups = append(profile.Groups, buildGroup(key, bucket))
	}
	sort.Slice(profile.Groups, func(i, j int) bool {
		if profile.Groups[i].FileCount == profile.Groups[j].FileCount {
			return profile.Groups[i].TotalSizeBytes > profile.Groups[j].TotalSizeBytes
		}
		return profile.Groups[i].FileCount > profile.Groups[j].FileCount
	})

	if len(files) > maxProfiledFiles {
		profile.Notes = append(profile.Notes,
			fmt.Sprintf("Only the first %d files include individual samples in this profile; all files are included in aggregate group counts.", maxProfiledFiles))
	}
	profile.Annotations = buildAnnotationSummaries(allProfiles)
	profile.DetectedPatterns = detectDatasetPatterns(profile.Groups)

	return profile, nil
}
