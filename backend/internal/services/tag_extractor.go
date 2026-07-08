package services

import (
	"sort"
	"strings"

	"dataset-platform/backend/internal/ai"
	"dataset-platform/backend/internal/profiler"
)

// deriveDomainTags returns domain tags grounded entirely in observable dataset
// signals: dataset name, file names, column names, annotation class names,
// profiler-detected patterns, and the researcher's own description.
// The AI summary is used as a secondary grounded source only (the AI prompt
// constrains it to profile evidence).
// A tag is emitted if and only if at least one of its trigger keywords appears
// literally in the corpus — there is no inference beyond keyword matching.
func DeriveDomainTags(
	dsName string,
	fileNames []string,
	userDescription string,
	profile *profiler.DatasetProfile,
	analysis *ai.DatasetAnalysis,
) []string {
	corpus := buildTagCorpus(dsName, fileNames, userDescription, profile, analysis)

	var tags []string
	for _, rule := range domainTagRules {
		for _, kw := range rule.keywords {
			if strings.Contains(corpus, kw) {
				tags = append(tags, rule.tag)
				break
			}
		}
	}

	// deduplicate and sort
	seen := map[string]bool{}
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		if !seen[tag] {
			seen[tag] = true
			result = append(result, tag)
		}
	}
	sort.Strings(result)
	return result
}

// mergeTags unions existing (user-provided) tags with newly derived tags,
// deduplicating case-insensitively and returning a sorted result.
func MergeTags(existing, derived []string) []string {
	seen := map[string]bool{}
	var merged []string
	for _, tag := range existing {
		norm := strings.ToLower(strings.TrimSpace(tag))
		if norm != "" && !seen[norm] {
			seen[norm] = true
			merged = append(merged, tag) // preserve original casing for user tags
		}
	}
	for _, tag := range derived {
		norm := strings.ToLower(strings.TrimSpace(tag))
		if norm != "" && !seen[norm] {
			seen[norm] = true
			merged = append(merged, tag)
		}
	}
	sort.Strings(merged)
	return merged
}

// buildTagCorpus assembles a lowercase string from all grounded signals.
// Only deterministic / user-supplied or profile-derived text is included.
func buildTagCorpus(
	dsName string,
	fileNames []string,
	userDescription string,
	profile *profiler.DatasetProfile,
	analysis *ai.DatasetAnalysis,
) string {
	var parts []string

	add := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" {
			parts = append(parts, s)
		}
	}

	// User-supplied text
	add(dsName)
	add(userDescription)
	for _, fn := range fileNames {
		add(fn)
	}

	if profile != nil {
		// File names recorded by the profiler
		for _, f := range profile.Files {
			add(f.OriginalName)
			// Column names + example values (curated by profiler, not free-form)
			for _, col := range f.Columns {
				add(col.Name)
				for _, ex := range col.ExampleValues {
					add(ex)
				}
			}
		}
		// Shared column names and examples from groups
		for _, g := range profile.Groups {
			for _, col := range g.SharedColumns {
				add(col.Name)
				for _, ex := range col.ExampleValues {
					add(ex)
				}
			}
		}
		// Annotation class names (COCO/YOLO etc.)
		for _, ann := range profile.Annotations {
			for _, cls := range ann.Classes {
				add(cls.Name)
			}
		}
		// Profiler-detected patterns (heuristic strings, not invented)
		for _, p := range profile.DetectedPatterns {
			add(p)
		}
	}

	// AI summary and label names as secondary grounded sources.
	// These are constrained by the AI prompt to only describe what is
	// visible in the profile, so keyword matching against them is safe.
	if analysis != nil {
		add(analysis.Summary)
		for _, label := range analysis.Labels {
			add(label.Name)
		}
	}

	return strings.ToLower(strings.Join(parts, " "))
}

type tagRule struct {
	tag      string
	keywords []string // emit tag if ANY keyword is present in corpus
}

// domainTagRules maps a canonical domain tag to highly-discriminative trigger
// keywords. Each keyword is specific enough that a false positive is unlikely.
// Rules are ordered from most specific to broadest within each domain.
var domainTagRules = []tagRule{
	// ── Cybersecurity ────────────────────────────────────────────────────────
	{
		tag: "intrusion detection",
		keywords: []string{
			"intrusion detection", "intrusion prevention",
			"cic-ids", "cicids", "nsl-kdd", "kdd cup", "kyoto dataset",
			"kdd99", "unsw-nb15",
		},
	},
	{
		tag: "network traffic",
		keywords: []string{
			"network traffic", "network flow", "netflow", "pcap", "packet capture",
			"flow duration", "fwd packet", "bwd packet", "flow bytes",
			"destination port", "source port", "flow iat",
		},
	},
	{
		tag: "malware analysis",
		keywords: []string{
			"malware", "ransomware", "trojan", "spyware", "rootkit",
			"pe file", "binary analysis", "apk analysis",
		},
	},
	{
		tag: "vulnerability",
		keywords: []string{
			"vulnerability", "exploit", "cve-", "cwe-",
			"buffer overflow", "sql injection", "cross-site scripting",
			"zero-day", "patch management",
		},
	},
	{
		tag: "cybersecurity",
		keywords: []string{
			"cybersecurity", "cyber security", "cyber attack",
			"ddos", "dos attack", "denial of service",
			"portscan", "port scan", "brute force", "botnet",
			"phishing", "man-in-the-middle", "lateral movement",
			"dos hulk", "dos goldeneye", "dos slowloris", "ftp-patator", "ssh-patator",
		},
	},

	// ── Medical / Health ─────────────────────────────────────────────────────
	{
		tag: "medical imaging",
		keywords: []string{
			"dicom", "radiograph", "x-ray", "xray", "mri", "ct scan",
			"ultrasound", "mammogram", "histology", "pathology slide", "fundus",
		},
	},
	{
		tag: "genomics",
		keywords: []string{
			"genome", "genomics", "dna sequence", "rna sequence", "mrna",
			"snp", "gene expression", "whole genome", "variant calling", "proteomics",
		},
	},
	{
		tag: "clinical",
		keywords: []string{
			"clinical trial", "patient record", "ehr", "emr",
			"diagnosis code", "icd-", "prescription data", "mortality",
			"hospital admission", "medical record",
		},
	},

	// ── NLP ──────────────────────────────────────────────────────────────────
	{
		tag: "sentiment analysis",
		keywords: []string{
			"sentiment", "opinion mining", "positive review", "negative review",
			"polarity", "emotion classification", "subjectivity",
		},
	},
	{
		tag: "named entity recognition",
		keywords: []string{
			"named entity", " ner ", "entity recognition",
			"bio tagging", "iob label", "entity span",
		},
	},
	{
		tag: "machine translation",
		keywords: []string{
			"machine translation", "parallel corpus", "bilingual corpus",
			"source language", "target language", "bleu score",
		},
	},
	{
		tag: "natural language processing",
		keywords: []string{
			" nlp ", "text classification", "question answering",
			"text summarization", "language model", "tokenization",
			"document classification",
		},
	},

	// ── Computer Vision ──────────────────────────────────────────────────────
	{
		tag: "object detection",
		keywords: []string{
			"bounding box", "bbox", "object detection",
			"faster rcnn", "ssd detection", "anchor box",
			"yolo", "coco annotation",
		},
	},
	{
		tag: "image segmentation",
		keywords: []string{
			"segmentation mask", "semantic segmentation", "instance segmentation",
			"panoptic", "pixel label", "polygon annotation",
		},
	},
	{
		tag: "facial recognition",
		keywords: []string{
			"face recognition", "facial landmark", "face detection",
			"face verification", "face embedding", "celeba", "lfw dataset",
		},
	},
	{
		tag: "computer vision",
		keywords: []string{
			"image classification", "imagenet", "visual recognition",
			"convolutional neural", "image dataset",
		},
	},

	// ── Autonomous Driving ───────────────────────────────────────────────────
	{
		tag: "autonomous driving",
		keywords: []string{
			"autonomous driving", "self-driving", "lidar", "point cloud",
			"lane detection", "ego vehicle", "depth estimation",
		},
	},

	// ── IoT / Sensors ────────────────────────────────────────────────────────
	{
		tag: "time series",
		keywords: []string{
			"time series", "timeseries", "temporal sequence",
			"sensor reading", "signal processing", "waveform",
		},
	},
	{
		tag: "iot",
		keywords: []string{
			"internet of things", " iot ", "mqtt", "zigbee",
			"smart home", "smart grid", "edge device",
		},
	},

	// ── Social Media ─────────────────────────────────────────────────────────
	{
		tag: "social media",
		keywords: []string{
			"twitter", "reddit", "instagram", "social network",
			"tweet", "hashtag", "retweet",
		},
	},

	// ── Finance ──────────────────────────────────────────────────────────────
	{
		tag: "fraud detection",
		keywords: []string{
			"fraud detection", "fraudulent transaction", "credit card fraud",
			"financial fraud", "anti-money laundering", "aml transaction",
		},
	},
	{
		tag: "financial",
		keywords: []string{
			"stock price", "stock market", "trading volume",
			"cryptocurrency", "bitcoin", "forex", "equity return",
		},
	},

	// ── Robotics ─────────────────────────────────────────────────────────────
	{
		tag: "robotics",
		keywords: []string{
			"robotic arm", " ros ", "ros2", "kinematics",
			"gripper", "end effector", "robot trajectory",
		},
	},

	// ── Recommender Systems ──────────────────────────────────────────────────
	{
		tag: "recommender system",
		keywords: []string{
			"collaborative filtering", "user-item matrix", "movie rating",
			"product rating", "user preference", "item embedding",
		},
	},

	// ── Geospatial ───────────────────────────────────────────────────────────
	{
		tag: "geospatial",
		keywords: []string{
			"latitude", "longitude", "geospatial", "gps coordinate",
			"shapefile", "geojson", "satellite image", "remote sensing",
		},
	},

	// ── Climate / Environment ────────────────────────────────────────────────
	{
		tag: "climate",
		keywords: []string{
			"climate change", "temperature anomaly", "precipitation",
			"greenhouse gas", "carbon emission", "sea level rise",
		},
	},
	{
		tag: "environmental",
		keywords: []string{
			"air quality", "pm2.5", "water quality",
			"biodiversity", "deforestation", "wildfire",
		},
	},

	// ── Audio / Speech ───────────────────────────────────────────────────────
	{
		tag: "speech recognition",
		keywords: []string{
			"speech recognition", " asr ", "speech-to-text",
			"phoneme", "spoken language", "audio transcription",
		},
	},
	{
		tag: "audio",
		keywords: []string{
			"audio classification", "sound event", "music information",
			"acoustic feature", "mel spectrogram",
		},
	},

	// ── Large Language Models / Generative AI ────────────────────────────────
	{
		tag: "instruction tuning",
		keywords: []string{
			"instruction tuning", "instruction-tuning", "instruction following",
			"alpaca", "alpaca json", "sharegpt", "flan dataset",
			"instruction", "output", // Alpaca {instruction, input, output}
		},
	},
	{
		tag: "preference learning",
		keywords: []string{
			"preference learning", "dpo", "direct preference optimization",
			"rlhf", "reinforcement learning from human feedback",
			"reward model", "chosen", "rejected",
			"human feedback", "preference pair",
		},
	},
	{
		tag: "question answering",
		keywords: []string{
			"question answering", "reading comprehension",
			"squad", "squad2", "triviaqa", "natural questions",
			"open-domain qa", "extractive qa", "generative qa",
		},
	},
	{
		tag: "text generation",
		keywords: []string{
			"text generation", "text summarization", "abstractive summarization",
			"story generation", "dialogue generation", "open-ended generation",
			"creative writing", "commonsense generation",
		},
	},
	{
		tag: "large language model",
		keywords: []string{
			"large language model", " llm ", "llms", "foundation model",
			"pre-training corpus", "pretraining data", "language model pre-training",
			"web crawl", "common crawl", "the pile", "openwebtext",
			"next-token prediction", "masked language model",
			"chat fine-tuning", "chat model", "conversational ai",
		},
	},
	{
		tag: "generative ai",
		keywords: []string{
			"generative ai", "generative model", "diffusion model",
			"text-to-image", "image generation", "stable diffusion",
			"gpt fine-tuning", "openai jsonl", "chatgpt", "gpt-4",
			"anthropic", "claude fine-tuning",
		},
	},
	{
		tag: "dense retrieval",
		keywords: []string{
			"dense retrieval", "dense passage retrieval", " dpr ",
			"retrieval augmented", "rag dataset", "bi-encoder",
			"query", "positive", "negatives", "hard negatives",
			"vector search", "semantic search dataset",
		},
	},
	{
		tag: "code dataset",
		keywords: []string{
			"code generation", "code completion", "code summarization",
			"programming dataset", "github code", "the-stack",
			"humaneval", "mbpp", "code benchmark",
		},
	},
}
