package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"dataset-platform/backend/internal/ai"
	"dataset-platform/backend/internal/filehandler"
	"dataset-platform/backend/internal/profiler"

	"github.com/lib/pq"
)

type PipelineService struct {
	db       *sql.DB
	aiClient *ai.Client
}

func NewPipelineService(db *sql.DB, aiClient *ai.Client) *PipelineService {
	return &PipelineService{db: db, aiClient: aiClient}
}

// Run executes the AI pipeline once all files for a dataset are assembled.
// Intended to be wired to filehandler.OnFileAssembled.
func (p *PipelineService) Run(datasetID int, files []filehandler.AssembledFile) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	defer func() {
		if r := recover(); r != nil {
			p.pipelineError(context.Background(), datasetID, fmt.Sprintf("AI pipeline recovered from internal error: %v", r))
		}
	}()

	// Idempotency guard: Check if already processed
	var status string
	err := p.db.QueryRowContext(ctx, "SELECT status FROM datasets WHERE id=$1", datasetID).Scan(&status)
	if err != nil {
		fmt.Printf("[AI pipeline] dataset %d: cannot read status: %v\n", datasetID, err)
		return
	}
	if status != "pending" {
		fmt.Printf("[AI pipeline] dataset %d: already processed (status=%s), skipping\n", datasetID, status)
		return
	}

	// Atomically set to 'processing' only if still 'pending'
	res, err := p.db.ExecContext(ctx,
		`UPDATE datasets
		    SET status='processing', processing_stage='profiling', updated_at=NOW()
		  WHERE id=$1 AND status='pending'`,
		datasetID,
	)
	if err != nil {
		fmt.Printf("[AI pipeline] dataset %d: cannot set processing status: %v\n", datasetID, err)
		return
	}
	if rows, _ := res.RowsAffected(); rows == 0 {
		fmt.Printf("[AI pipeline] dataset %d: already being processed by another goroutine\n", datasetID)
		return
	}
	if err := p.clearGeneratedMetadata(ctx, datasetID); err != nil {
		p.pipelineError(ctx, datasetID, fmt.Sprintf("generated metadata cleanup failed: %v", err))
		return
	}

	// Derive file names and total size from the assembled file list.
	fileNames := make([]string, len(files))
	var totalSize int64
	for i, f := range files {
		fileNames[i] = f.OriginalName
		totalSize += f.SizeBytes
	}

	fmt.Printf("[AI pipeline] dataset %d: starting — %d file(s), %d bytes total\n",
		datasetID, len(files), totalSize)

	datasetProfile, err := profiler.ProfileDataset(ctx, files)
	if err != nil {
		p.pipelineError(ctx, datasetID, fmt.Sprintf("dataset profiling failed: %v", err))
		return
	}
	profileBytes, err := json.Marshal(datasetProfile)
	if err != nil {
		p.pipelineError(ctx, datasetID, fmt.Sprintf("dataset profile serialization failed: %v", err))
		return
	}
	profileJSON := string(profileBytes)
	aiProfileJSON := buildAIProfileJSON(datasetProfile)
	if _, err := p.db.ExecContext(ctx,
		`UPDATE datasets
		    SET profile_json = $1::jsonb,
		        processing_stage = 'ai_metadata',
		        updated_at = NOW()
		  WHERE id = $2`,
		profileJSON, datasetID,
	); err != nil {
		p.pipelineError(ctx, datasetID, fmt.Sprintf("dataset profile storage failed: %v", err))
		return
	}
	fmt.Printf("[AI pipeline] dataset %d: profile stored=%d chars, ai excerpt=%d chars\n",
		datasetID, len(profileJSON), len(aiProfileJSON))

	// Read dataset metadata (including existing user-supplied tags and declared label column)
	var dsName, researcherName, userDescription string
	var existingTags []string
	var labelColumnNull sql.NullString
	if err := p.db.QueryRowContext(ctx,
		`SELECT name, researcher_name, coalesce(description, ''), coalesce(tags, '{}'), label_column
		   FROM datasets WHERE id = $1`,
		datasetID,
	).Scan(&dsName, &researcherName, &userDescription, pq.Array(&existingTags), &labelColumnNull); err != nil {
		p.pipelineError(ctx, datasetID, fmt.Sprintf("cannot read dataset row: %v", err))
		return
	}
	labelColumn := ""
	if labelColumnNull.Valid {
		labelColumn = labelColumnNull.String
	}

	analysis, err := p.aiClient.AnalyzeDateset(ctx, ai.AnalyzeRequest{
		DatasetName:     dsName,
		ResearcherName:  researcherName,
		UserDescription: userDescription,
		FileNames:       fileNames,
		TotalSizeBytes:  totalSize,
		ProfileJSON:     aiProfileJSON,
	})
	if err != nil {
		p.pipelineError(ctx, datasetID, fmt.Sprintf("AI analysis failed: %v", err))
		return
	}
	fmt.Printf("[AI pipeline] dataset %d: Flash returned — modality=%s type=%s completeness=%.2f labels=%d queries=%d\n",
		datasetID, analysis.Modality, analysis.DatasetType,
		analysis.LabelCompleteness, len(analysis.Labels), len(analysis.PseudoQueries))

	applyProfileCorrections(analysis, datasetProfile)
	reconcileLabelMetadata(analysis, datasetProfile)
	if labelColumn != "" {
		applyDeclaredLabelColumn(labelColumn, datasetProfile, analysis)
	}
	appendRecoveredMetadataCaveat(analysis)

	derivedTags := DeriveDomainTags(dsName, fileNames, userDescription, datasetProfile, analysis)
	mergedTags := MergeTags(existingTags, derivedTags)
	fmt.Printf("[AI pipeline] dataset %d: tags — existing=%v derived=%v merged=%v\n",
		datasetID, existingTags, derivedTags, mergedTags)

	fmt.Printf("[AI pipeline] dataset %d: profile-corrected metadata - modality=%s type=%s format=%s completeness=%.2f\n",
		datasetID, analysis.Modality, analysis.DatasetType,
		analysis.AnnotationFormat, analysis.LabelCompleteness)
	for _, label := range analysis.Labels {
		var proportion any
		if label.Proportion != nil {
			proportion = *label.Proportion
		}
		if _, err := p.db.ExecContext(ctx,
			`INSERT INTO labels (dataset_id, label_name, proportion, sample_count)
			 VALUES ($1, $2, $3, $4)`,
			datasetID, label.Name, proportion, label.SampleCount,
		); err != nil {
			fmt.Printf("[AI pipeline] dataset %d: warning — label %q insert failed: %v\n",
				datasetID, label.Name, err)
		}
	}

	seen := make(map[string]bool)
	queryCount := 0
	for _, q := range analysis.PseudoQueries {
		if queryCount >= 10 {
			break
		}
		q = strings.TrimSpace(q)
		if q == "" || seen[q] {
			continue
		}
		seen[q] = true
		queryCount++
		if _, err := p.db.ExecContext(ctx,
			`INSERT INTO pseudo_queries (dataset_id, query_text) VALUES ($1, $2)`,
			datasetID, q,
		); err != nil {
			fmt.Printf("[AI pipeline] dataset %d: warning — pseudo-query insert failed: %v\n",
				datasetID, err)
		}
	}

	if err := p.updateSearchVector(ctx, datasetID, analysis); err != nil {
		fmt.Printf("[AI pipeline] dataset %d: warning - search_vector update failed: %v\n", datasetID, err)
	}

	embeddingJSON := ""
	if analysis.Summary != "" {
		if _, err := p.db.ExecContext(ctx,
			`UPDATE datasets SET processing_stage='embedding', updated_at=NOW() WHERE id=$1`,
			datasetID,
		); err != nil {
			fmt.Printf("[AI pipeline] dataset %d: warning - processing_stage update failed: %v\n", datasetID, err)
		}
		vec, embErr := p.aiClient.GenerateEmbedding(ctx, buildEmbeddingText(analysis, datasetProfile, userDescription))
		if embErr != nil {
			fmt.Printf("[AI pipeline] dataset %d: warning — embedding failed (keyword search still works): %v\n",
				datasetID, embErr)
		} else {
			b, _ := json.Marshal(vec)
			embeddingJSON = string(b)
		}
	}

	if _, err := p.db.ExecContext(ctx,
		`UPDATE datasets
		    SET status             = 'ready',
		        ai_summary         = $1,
		        modality           = $2,
		        dataset_type       = $3,
		        annotation_format  = $4,
		        label_completeness = $5,
		        total_size_bytes   = $6,
		        embedding_json     = NULLIF($7, ''),
		        ai_confidence      = $8,
		        ai_caveats         = $9,
		        tags               = $10,
		        processing_stage   = '',
		        updated_at         = NOW()
		  WHERE id = $11`,
		analysis.Summary,
		analysis.Modality,
		analysis.DatasetType,
		analysis.AnnotationFormat,
		analysis.LabelCompleteness,
		totalSize,
		embeddingJSON,
		analysis.Confidence,
		pq.Array(analysis.Caveats),
		pq.Array(mergedTags),
		datasetID,
	); err != nil {
		p.pipelineError(ctx, datasetID, fmt.Sprintf("final dataset update failed: %v", err))
		return
	}

	fmt.Printf("[AI pipeline] dataset %d: READY\n", datasetID)
}

func (p *PipelineService) updateSearchVector(ctx context.Context, datasetID int, analysis *ai.DatasetAnalysis) error {
	if analysis == nil {
		return nil
	}
	_, err := p.db.ExecContext(ctx, `
		UPDATE datasets d
		SET search_vector =
			setweight(to_tsvector('english', coalesce(d.name, '')), 'A') ||
			setweight(to_tsvector('english', coalesce(label_text.labels, '')), 'A') ||
			setweight(to_tsvector('english', coalesce($2::text, '')), 'B') ||
			setweight(to_tsvector('english', coalesce(query_text.queries, '')), 'B') ||
			setweight(to_tsvector('english', coalesce(d.description, '')), 'C') ||
			setweight(to_tsvector('english', array_to_string(coalesce(d.tags, '{}'), ' ')), 'C') ||
			setweight(to_tsvector('english', coalesce(d.researcher_name, '')), 'C') ||
			setweight(to_tsvector('english', coalesce($3::text, '')), 'C') ||
			setweight(to_tsvector('english', coalesce($4::text, '')), 'C') ||
			setweight(to_tsvector('english', coalesce($5::text, '')), 'C')
		FROM (
			SELECT string_agg(label_name, ' ') AS labels
			FROM labels
			WHERE dataset_id = $1
		) label_text,
		(
			SELECT string_agg(query_text, ' ') AS queries
			FROM pseudo_queries
			WHERE dataset_id = $1
		) query_text
		WHERE d.id = $1`,
		datasetID,
		analysis.Summary,
		analysis.Modality,
		analysis.DatasetType,
		analysis.AnnotationFormat,
	)
	return err
}

func (p *PipelineService) pipelineError(ctx context.Context, datasetID int, msg string) {
	fmt.Printf("[AI pipeline] dataset %d ERROR: %s\n", datasetID, msg)
	p.db.ExecContext(ctx,
		`UPDATE datasets
		    SET status = 'error',
		        processing_stage = 'error',
		        error_message = $1,
		        updated_at = NOW()
		  WHERE id = $2`,
		msg, datasetID,
	)
}

func appendRecoveredMetadataCaveat(analysis *ai.DatasetAnalysis) {
	if analysis == nil || len(analysis.MissingMetadataFields) == 0 {
		return
	}
	addAnalysisCaveat(analysis, "AI response was incomplete; missing metadata fields were completed from profile evidence.")
}

func addAnalysisCaveat(analysis *ai.DatasetAnalysis, caveat string) {
	for _, existing := range analysis.Caveats {
		if existing == caveat {
			return
		}
	}
	analysis.Caveats = append(analysis.Caveats, caveat)
}

func buildEmbeddingText(analysis *ai.DatasetAnalysis, profile *profiler.DatasetProfile, userDescription string) string {
	const maxEmbeddingChars = 24000

	var b strings.Builder
	appendSection(&b, "AI summary", analysis.Summary)
	appendSection(&b, "User upload description", userDescription)
	for _, label := range analysis.Labels {
		appendLine(&b, "Label/class: "+label.Name)
	}
	for _, q := range analysis.PseudoQueries {
		appendLine(&b, "Search query: "+q)
	}
	appendProfileSearchText(&b, profile)

	result := b.String()
	if len(result) > maxEmbeddingChars {
		fmt.Printf("[Embedding] text truncated from %d to %d chars\n", len(result), maxEmbeddingChars)
		result = result[:maxEmbeddingChars]
	}
	return result
}

func appendProfileSearchText(b *strings.Builder, profile *profiler.DatasetProfile) {
	if profile == nil {
		return
	}
	appendLine(b, "Profile evidence:")
	for _, pattern := range firstN(profile.DetectedPatterns, 8) {
		appendLine(b, "Detected pattern: "+pattern)
	}
	for _, note := range firstN(profile.Notes, 8) {
		appendLine(b, "Profile note: "+note)
	}
	for _, ft := range firstNTypeSummaries(profile.FileTypes, 8) {
		appendLine(b, fmt.Sprintf("File type: %s files=%d", ft.DetectedType, ft.FileCount))
	}

	columnCount := 0
	for _, group := range profile.Groups {
		appendLine(b, fmt.Sprintf("File group: %s %s files=%d", group.Role, group.DetectedType, group.FileCount))
		for _, col := range group.SharedColumns {
			if columnCount >= 40 {
				break
			}
			appendColumnSearchText(b, col)
			columnCount++
		}
		for _, example := range firstNFileProfiles(group.RepresentativeExamples, 2) {
			appendFileSampleSearchText(b, example)
		}
	}

	for _, file := range firstNFileProfiles(profile.Files, 3) {
		appendFileSampleSearchText(b, file)
	}

	for _, annotation := range profile.Annotations {
		appendLine(b, fmt.Sprintf("Annotation profile: classes=%d annotations=%d", annotation.ClassCount, annotation.TotalAnnotations))
		for _, class := range firstNClasses(annotation.Classes, 40) {
			appendLine(b, "Annotation class: "+class.Name)
		}
	}
}

func appendColumnSearchText(b *strings.Builder, col profiler.ColumnProfile) {
	line := "Column: " + col.Name
	if col.InferredType != "" {
		line += " " + col.InferredType
	}
	if len(col.ExampleValues) > 0 {
		line += " examples: " + strings.Join(firstN(col.ExampleValues, 4), ", ")
	}
	appendLine(b, line)
}

func appendFileSampleSearchText(b *strings.Builder, file profiler.FileProfile) {
	for _, text := range firstN(file.SampleText, 3) {
		appendLine(b, "Sample text: "+truncateSearchText(text, 220))
	}
	for _, row := range firstNRows(file.SampleRows, 3) {
		keys := make([]string, 0, len(row))
		for key := range row {
			keys = append(keys, key)
		}
		sort.Strings(keys)

		pairs := make([]string, 0, len(row))
		for _, key := range keys {
			value := row[key]
			if strings.TrimSpace(value) == "" {
				continue
			}
			pairs = append(pairs, key+"="+truncateSearchText(value, 80))
			if len(pairs) >= 8 {
				break
			}
		}
		if len(pairs) > 0 {
			appendLine(b, "Sample row: "+strings.Join(pairs, "; "))
		}
	}
}

func appendSection(b *strings.Builder, title, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	appendLine(b, title+": "+value)
}

func appendLine(b *strings.Builder, line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	b.WriteString(line)
	b.WriteString("\n")
}

func truncateSearchText(value string, max int) string {
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= max {
		return value
	}
	return strings.TrimSpace(value[:max]) + "..."
}

func firstN(values []string, n int) []string {
	if len(values) < n {
		return values
	}
	return values[:n]
}

func firstNRows(values []map[string]string, n int) []map[string]string {
	if len(values) < n {
		return values
	}
	return values[:n]
}

func firstNTypeSummaries(values []profiler.TypeSummary, n int) []profiler.TypeSummary {
	if len(values) < n {
		return values
	}
	return values[:n]
}

func firstNFileProfiles(values []profiler.FileProfile, n int) []profiler.FileProfile {
	if len(values) < n {
		return values
	}
	return values[:n]
}

func firstNClasses(values []profiler.ClassProfile, n int) []profiler.ClassProfile {
	if len(values) < n {
		return values
	}
	return values[:n]
}
