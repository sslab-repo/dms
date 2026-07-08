package ai

import (
	"context"
	"fmt"
	"strings"
)

/*
AnalyzeDataset is called at upload time.

It sends everything we know about the dataset: the name, the
researcher, the file names, any user-supplied description to Flash
and asks for a structured JSON analysis in return.

The returned DatasetAnalysis is then stored in PostgreSQL and fed
into both search indexes.
*/
type AnalyzeRequest struct {
	DatasetName     string
	ResearcherName  string
	UserDescription string
	FileNames       []string
	TotalSizeBytes  int64
	ProfileJSON     string
}

func (c *Client) AnalyzeDateset(ctx context.Context, req AnalyzeRequest) (*DatasetAnalysis, error) {
	prompt := buildDatasetAnalysisPrompt(req)
	fmt.Printf("[AI] Prompt length: %d chars\n", len(prompt))

	raw, err := c.complete(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("AI analysis call failed: %w", err)
	}

	fmt.Printf("[AI] Raw response (first 500 chars): %s\n", truncateString(raw, 500))

	recoveryMode := "direct"
	analysis, err := parseDatasetAnalysis(raw)
	if err != nil {
		fmt.Printf("[AI] Direct JSON parse failed: %v\n", err)
		cleaned := stripMarkdownFences(raw)
		recoveryMode = "stripped"
		analysis, err = parseDatasetAnalysis(cleaned)
		if err != nil {
			fmt.Printf("[AI] Stripped JSON parse failed: %v\n", err)
			repaired := repairJSON(cleaned)
			recoveryMode = "repaired"
			analysis, err = parseDatasetAnalysis(repaired)
			if err != nil {
				fmt.Printf("[AI] Repaired JSON parse failed: %v\n", err)
				sanitized := sanitizeMalformedStringArray(cleaned, "pseudo_queries")
				recoveryMode = "sanitized"
				analysis, err = parseDatasetAnalysis(sanitized)
				if err != nil {
					fmt.Printf("[AI] Sanitized pseudo_queries parse failed: %v\n", err)
					fmt.Printf("[AI] Full raw response: %s\n", raw)
					return nil, fmt.Errorf("could not parse AI response as JSON after 4 attempts")
				}
			}
		}
	}
	analysis.ParseRecovery = recoveryMode

	normalizeAnalysisForRequest(analysis, req)
	return analysis, nil
}

func buildDatasetAnalysisPrompt(req AnalyzeRequest) string {
	return fmt.Sprintf(
		`You are a research data analyst for a university dataset discovery platform.

TASK
Analyze an uploaded dataset using only the supplied dataset metadata and bounded profile. The profile may include file names, file types, schemas, column names, inferred types, sample rows, text snippets, annotation summaries, and file group summaries.

You do not have access to the full dataset. Do not claim that you inspected the full dataset. Use cautious language when evidence comes from samples.

INPUT
- Dataset name: %s
- Uploaded by: %s
- Researcher description: %s
- Files: %s
- Total size: %d bytes
- Compact dataset profile JSON:
%s

STRICT OUTPUT (return ONLY valid JSON; no markdown, no prose, no extra keys)
{
	"summary": "<5 or more complete sentences>",
	"labels": [
		{
			"name": "<label/class/target name>",
			"proportion": <NUMBER 0.0-1.0 or null>,
			"sample_count": <INTEGER, use -1 if unknown>
		}
	],
	"label_completeness": <NUMBER 0.0-1.0>,
	"modality": "<text|image|tabular|audio|multimodal|unknown>",
	"dataset_type": "<supervised|unsupervised|semi-supervised|self-supervised|unknown>",
	"annotation_format": "<CSV|JSONL|JSON|Parquet|COCO JSON|YOLO TXT|Pascal VOC XML|KITTI TXT|CoNLL|HDF5|TFRecord|Arrow|Alpaca JSON|ShareGPT JSON|OpenAI JSONL|DPO pairs|SQuAD JSON|plain text|image files|none|unknown>",
	"pseudo_queries": ["<5-10 unique search queries>"],
	"confidence": <NUMBER 0.0-1.0>,
	"caveats": ["<short practical caveat>"]
}

STRICTNESS
- JSON ONLY. Do not wrap in code fences.
- Return only the JSON object.
- Do not include explanations, markdown, or extra keys.
- Numeric fields MUST be numbers (no quotes). Use null only for proportion when unknown.
- Array fields MUST be arrays. "labels", "pseudo_queries", and "caveats" must always be JSON arrays, even when empty.
- Do not invent columns, labels, annotation formats, or data contents that are not supported by filenames, schema, sample rows, or user description.
- Sample rows are illustrative records, not necessarily the dataset topic. Prefer dataset name, researcher description, file structure, column names, label columns, annotations, and repeated schema patterns over one-off sample text.

SUMMARY REQUIREMENTS
Write a researcher-facing summary of what the dataset appears to contain.

The summary must:
- Describe what the data represents in plain language.
- Mention important fields, measurements, entities, labels, annotations, or file groups visible in the profile.
- Explain what analyses, research questions, or modeling tasks the dataset may support.
- Describe who the dataset may be useful for.
- Mention the uploader by exact name: %s.
- Avoid implementation language such as "bounded samples", "profile JSON", "schema extracted", or "metadata profile".
- If the profile has grouped files, summarize the groups as a collection instead of describing every file individually.
- The summary is used for keyword and semantic search. It must include specific domain terms, important columns, measured quantities, and likely research uses.
- Put uncertainty, sampling limits, and unsupported-file warnings in caveats, not in the summary.

MODALITY RULES
- Modality means the dataset's primary content, not its storage/container format.
- CSV, TSV, Excel, Parquet, and JSON describe file or annotation format; they do not automatically mean modality is "tabular".
- Use "tabular" when the primary content is structured records, metadata, measurements, dates, identifiers, ratings, categorical attributes, operational metrics, or table-like JSON.
- Use "image" for image files or image datasets.
- Use "text" for plain text, documents, transcripts, articles, reviews, prompts, responses, or natural-language records, even when those records are stored in CSV/JSON/Parquet rows.
- Use "audio" for audio files.
- Use "multimodal" when the dataset combines more than one primary modality.
- Use "unknown" only when the evidence is insufficient.

ANNOTATION FORMAT RULES
- Report the concrete detected format when visible from the profile. Use the most specific matching format below.
- "CSV": tabular data in comma-separated format.
- "JSONL": newline-delimited JSON, one object per line — common for LLM training corpora and streaming data.
- "JSON": standard JSON document — could be a list of records, a single annotated document, or a structured format.
- "Parquet": columnar binary format for tabular data.
- "COCO JSON": JSON with a top-level "categories" array and an "annotations" array — for object detection, segmentation, or keypoints.
- "YOLO TXT": one label file per image with rows of <class_id cx cy w h> or similar space-separated bounding-box format.
- "Pascal VOC XML": per-image XML files containing <annotation><object><bndbox> — the original VOC annotation schema.
- "KITTI TXT": space-separated per-object annotation with 15 fields (type, truncated, occluded, alpha, 4×bbox, 3×dims, 3×loc, rotation_y) — used for 3D object detection.
- "CoNLL": tab-separated token-per-line format with one or more BIO/IOB tag columns — used for NER, POS tagging, chunking.
- "HDF5": binary hierarchical data format (.h5/.hdf5) — common for scientific arrays, medical imaging, and large-scale ML datasets.
- "TFRecord": TensorFlow binary protobuf record format (.tfrecord) — used with tf.data pipelines.
- "Arrow": Apache Arrow or Feather binary columnar format (.arrow/.feather) — used for fast in-memory ML datasets (Hugging Face datasets library).
- "Alpaca JSON": JSON array of {instruction, input, output} objects — the standard instruction-tuning fine-tuning format.
- "ShareGPT JSON": JSON objects with a "conversations" array of {from, value} pairs — for multi-turn conversation fine-tuning.
- "OpenAI JSONL": JSONL with {messages: [{role, content}]} objects — for fine-tuning OpenAI-compatible chat models.
- "DPO pairs": JSON/JSONL with {prompt, chosen, rejected} — for Direct Preference Optimization or reward model training.
- "SQuAD JSON": JSON with {data: [{paragraphs: [{context, qas}]}]} — Stanford Question Answering Dataset format for reading comprehension.
- "plain text": unstructured natural-language text — pre-training corpora, documents, web crawls.
- "image files": when images themselves are the primary data format and no separate annotation file format is present.
- Use "none" only when the dataset has no annotation format and the storage format is incidental.
- Use "unknown" only when the format cannot be determined from available evidence.

LABEL RULES
- In the "labels" array, include actual class names, target values, annotation categories, or outcomes only.
- Do not put the name of a label column itself in "labels". For example, a CSV column named "label" is label-field evidence, not a class named "label".
- Do not treat ordinary categorical feature columns as labels merely because they have repeated values.
- Treat a column as a label/target only when its name or description explicitly identifies it as the intended ground-truth, label, class, target, answer, outcome, or diagnosis field.
- Treat identifiers, grouping variables, measurements, timestamps, locations, descriptive attributes, operational metrics, and business/domain facts as ordinary data fields unless the researcher description explicitly identifies one as the prediction target.
- If labels are annotation classes from COCO/YOLO/etc., include those class names.
- If a label/target column is visible but the profile does not show its class/value distribution, return an empty labels array, keep label_completeness based on the visible column population, and mention the uncertainty in caveats.
- If there are no clear class names, target values, annotation classes, or outcomes, return an empty labels array.

DATASET TYPE RULES
- Use "supervised" only when the dataset has explicit label/class/target/ground-truth evidence, annotation classes, or the researcher description clearly identifies a prediction/classification target.
  - Instruction-tuning datasets (Alpaca, ShareGPT, OpenAI JSONL, DPO pairs) are supervised — each example has a ground-truth output.
  - SQuAD and QA datasets with answer spans are supervised.
  - RLHF/preference datasets with {chosen, rejected} pairs are supervised.
- Use "semi-supervised" only when that explicit label/target evidence exists but appears partially populated or mixed with unlabeled records.
- Use "unsupervised" for structured datasets that have no clear labels/classes/targets but are still useful for exploratory analysis, clustering, retrieval, summarization, descriptive statistics, trend analysis, dashboards, reporting, or feature engineering.
  - A column that could hypothetically be predicted is not enough to make a dataset supervised.
- Use "self-supervised" when the dataset structure is used for self-supervised learning: masked language modeling, next-token prediction, contrastive pre-training, masked autoencoding, or similar tasks without human-annotated labels.
  - Large raw text corpora (.txt, .jsonl with only a "text" field, WARC/WET), Wikipedia dumps, book corpora, and web-crawl datasets used for language model pre-training are self-supervised.
  - Image datasets used for contrastive learning (SimCLR, DINO, MAE) without bounding boxes or class labels are self-supervised.
  - Audio datasets for wav2vec-style pre-training are self-supervised.
- Use "unknown" only when the profile is too sparse, unsupported, corrupt, or ambiguous to tell what kind of dataset it is.

LABEL COMPLETENESS RULES
- label_completeness must be between 0.0 and 1.0.
- If labels/classes/targets are present, estimate completeness from the profile evidence.
- Base the estimate on non-empty label/target values in the sample or annotation summaries.
- If labels/classes/targets are absent, use 0.0.
- If the sample is very small or not representative, still return a numeric estimate but mention uncertainty in caveats.
- Do not overstate completeness from only a few rows.

CONFIDENCE RULES
- confidence must be between 0.0 and 1.0 and should reflect how strong the profile evidence is.
- Use higher confidence when file types, schema, columns, samples, and user description agree.
- Use lower confidence when evidence is sparse, samples are tiny, file types are mixed, columns are ambiguous, or the user description is missing.
- Confidence should describe confidence in the generated metadata, not dataset quality.

CAVEAT RULES
- Include practical uncertainty notes.
- Mention small sample sizes, missing values in sampled rows, ambiguous labels, mixed file types, unsupported files, or inferred schema limitations.
- Do not put caveats in the summary.

PSEUDO QUERY RULES
- Generate 5-10 search queries that would help researchers find this dataset.
- Include both keyword-style queries and natural-language queries.
- Use specific domain terms visible in the profile.
- Do not include duplicate or near-duplicate queries.

Return ONLY the JSON object.`,
		req.DatasetName,
		req.ResearcherName,
		req.UserDescription,
		strings.Join(req.FileNames, ", "),
		req.TotalSizeBytes,
		req.ProfileJSON,
		req.ResearcherName,
	)
}
