package profiler

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

func profileJSONLines(path string, fp *FileProfile) {
	file, err := os.Open(path)
	if err != nil {
		fp.Warnings = append(fp.Warnings, "could not open file sample")
		return
	}
	defer file.Close()

	// Scan up to maxStatRows for statistics; keep only maxSampleRows for display.
	decoder := json.NewDecoder(io.LimitReader(file, maxReadBytes))
	statsByName := map[string]*columnStat{}
	var firstObj map[string]any
	for fp.SampledRows < maxStatRows {
		var value any
		if err := decoder.Decode(&value); err != nil {
			break
		}
		obj, ok := value.(map[string]any)
		if !ok {
			if len(fp.SampleText) < maxTextLines {
				fp.SampleText = append(fp.SampleText, truncate(fmt.Sprintf("%v", value), 200))
			}
			fp.SampledRows++
			continue
		}
		if firstObj == nil {
			firstObj = obj
		}
		fp.SampledRows++
		row := map[string]string{}
		for key, raw := range obj {
			if len(statsByName) >= maxColumns {
				break
			}
			name := strings.TrimSpace(key)
			stat := statsByName[name]
			if stat == nil {
				stat = &columnStat{name: name}
				statsByName[name] = stat
			}
			value := stringifySample(raw)
			stat.observe(value)
			if len(fp.SampleRows) < maxSampleRows {
				row[name] = truncate(value, 120)
			}
		}
		if len(fp.SampleRows) < maxSampleRows {
			fp.SampleRows = append(fp.SampleRows, row)
		}
	}
	fp.Columns = finalizeNamedColumns(statsByName)
	if note := detectLLMFormat(firstObj); note != "" {
		fp.Warnings = append(fp.Warnings, note)
	}
}

func profileJSON(path string, fp *FileProfile) {
	b, err := readLimited(path, maxReadBytes)
	if err != nil {
		fp.Warnings = append(fp.Warnings, "could not open JSON sample")
		return
	}

	var value any
	if err := json.Unmarshal(b, &value); err != nil {
		fp.SampleText = []string{truncate(string(b), 500)}
		fp.Warnings = append(fp.Warnings, "JSON sample could not be parsed as a complete JSON document")
		return
	}
	if fp.Role == "annotations" {
		if annotation, ok := parseCOCOAnnotation(value, fp.OriginalName); ok {
			fp.Annotation = annotation
		}
	}

	// Detect SQuAD-style reading comprehension format.
	if note := detectSQuADFormat(value); note != "" {
		fp.Warnings = append(fp.Warnings, note)
	}

	var rows []map[string]any
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			if obj, ok := item.(map[string]any); ok {
				rows = append(rows, obj)
			}
			if len(rows) >= maxSampleRows {
				break
			}
		}
	case map[string]any:
		rows = append(rows, v)
	}

	statsByName := map[string]*columnStat{}
	var firstRow map[string]any
	for _, obj := range rows {
		if firstRow == nil {
			firstRow = obj
		}
		fp.SampledRows++
		row := map[string]string{}
		for key, raw := range obj {
			if len(statsByName) >= maxColumns {
				break
			}
			name := strings.TrimSpace(key)
			stat := statsByName[name]
			if stat == nil {
				stat = &columnStat{name: name}
				statsByName[name] = stat
			}
			value := stringifySample(raw)
			stat.observe(value)
			row[name] = truncate(value, 120)
		}
		fp.SampleRows = append(fp.SampleRows, row)
	}
	fp.Columns = finalizeNamedColumns(statsByName)
	if note := detectLLMFormat(firstRow); note != "" {
		fp.Warnings = append(fp.Warnings, note)
	}
}

// detectLLMFormat identifies common LLM fine-tuning/alignment dataset schemas
// from the keys of a sample JSON object. Returns a descriptive note or "".
func detectLLMFormat(obj map[string]any) string {
	if obj == nil {
		return ""
	}
	has := func(key string) bool { _, ok := obj[key]; return ok }

	switch {
	case has("chosen") && has("rejected"):
		if has("prompt") {
			return "Detected DPO/preference-pair format: {prompt, chosen, rejected} — used for RLHF/DPO alignment training"
		}
		return "Detected preference-pair format: {chosen, rejected} — used for reward model or DPO training"
	case has("conversations") || has("dialogue"):
		return "Detected ShareGPT/conversation format: multi-turn chat pairs used for instruction tuning"
	case has("messages"):
		return "Detected OpenAI chat-completion format: {messages: [{role, content}]} — used for fine-tuning chat models"
	case has("instruction") && has("output"):
		if has("input") {
			return "Detected Alpaca instruction-tuning format: {instruction, input, output}"
		}
		return "Detected instruction-tuning format: {instruction, output}"
	case has("prompt") && has("completion"):
		return "Detected prompt-completion format: used for GPT-style fine-tuning"
	case has("prompt") && has("response"):
		return "Detected prompt-response format: used for instruction tuning or RLHF"
	case has("question") && has("answer"):
		return "Detected QA pair format: {question, answer} — used for reading comprehension or open-domain QA"
	case has("query") && (has("positive") || has("negatives")):
		return "Detected retrieval training format: {query, positive, negatives} — used for dense retrieval / RAG fine-tuning"
	}
	return ""
}

// detectSQuADFormat checks if a JSON document matches the SQuAD reading-
// comprehension structure: {data: [{paragraphs: [{context, qas}]}]}.
func detectSQuADFormat(value any) string {
	obj, ok := value.(map[string]any)
	if !ok {
		return ""
	}
	data, ok := obj["data"].([]any)
	if !ok || len(data) == 0 {
		return ""
	}
	first, ok := data[0].(map[string]any)
	if !ok {
		return ""
	}
	if _, hasPara := first["paragraphs"]; hasPara {
		return "Detected SQuAD reading-comprehension format: {data: [{paragraphs: [{context, qas}]}]}"
	}
	return ""
}
