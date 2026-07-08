package ai

import (
	"encoding/json"
	"strings"
)

// stripMarkdownFences removes markdown code fences and extracts the JSON payload.
// Handles ```json, ```, and cases where JSON is embedded in other text.
func stripMarkdownFences(s string) string {
	s = strings.TrimSpace(s)

	// Try to find the first { and last } to extract JSON payload
	firstBrace := strings.Index(s, "{")
	lastBrace := strings.LastIndex(s, "}")

	if firstBrace != -1 && lastBrace != -1 && lastBrace > firstBrace {
		return s[firstBrace : lastBrace+1]
	}

	// Fallback: strip ``` fences if present
	if strings.HasPrefix(s, "```") {
		idx := strings.Index(s, "\n")
		if idx != -1 {
			s = s[idx+1:]
		}
	}
	if strings.HasSuffix(s, "```") {
		s = s[:strings.LastIndex(s, "```")]
	}
	return strings.TrimSpace(s)
}

// truncateString returns the first n characters of s, or the whole string if shorter.
func truncateString(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// repairJSON attempts to fix truncated JSON by closing unclosed braces, brackets, and quotes.
// This is a last-resort fallback when the model hits a token limit mid-response.
func repairJSON(s string) string {
	s = strings.TrimSpace(s)

	// Track open delimiters
	var stack []rune
	inString := false
	escaped := false

	for _, ch := range s {
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch ch {
		case '{', '[':
			stack = append(stack, ch)
		case '}':
			if len(stack) > 0 && stack[len(stack)-1] == '{' {
				stack = stack[:len(stack)-1]
			}
		case ']':
			if len(stack) > 0 && stack[len(stack)-1] == '[' {
				stack = stack[:len(stack)-1]
			}
		}
	}

	// Close any unclosed strings
	if inString {
		s += `"`
	}

	// Close unclosed delimiters in reverse order
	for i := len(stack) - 1; i >= 0; i-- {
		switch stack[i] {
		case '{':
			s += "}"
		case '[':
			s += "]"
		}
	}

	return s
}

func sanitizeMalformedStringArray(s, field string) string {
	start, end, ok := findTopLevelArrayBlock(s, `"`+field+`"`)
	if !ok {
		return s
	}

	items := salvageStringArrayItems(s[start+1 : end])
	if len(items) == 0 {
		return s[:start+1] + "]" + s[end+1:]
	}

	b, err := json.Marshal(items)
	if err != nil {
		return s
	}
	return s[:start] + string(b) + s[end+1:]
}

func findTopLevelArrayBlock(s, key string) (int, int, bool) {
	keyIdx := strings.Index(s, key)
	if keyIdx < 0 {
		return 0, 0, false
	}
	open := strings.Index(s[keyIdx:], "[")
	if open < 0 {
		return 0, 0, false
	}
	start := keyIdx + open

	for i := start + 1; i < len(s); i++ {
		if s[i] != ']' {
			continue
		}
		rest := strings.TrimLeft(s[i+1:], " \t\r\n")
		if strings.HasPrefix(rest, ",") || strings.HasPrefix(rest, "}") || rest == "" {
			return start, i, true
		}
	}
	return 0, 0, false
}

func salvageStringArrayItems(block string) []string {
	lines := strings.Split(block, "\n")
	items := make([]string, 0, len(lines))
	var pending strings.Builder

	flush := func() {
		item := cleanMalformedStringItem(pending.String())
		pending.Reset()
		if item != "" {
			items = append(items, item)
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if pending.Len() > 0 {
			pending.WriteString(" ")
		}
		pending.WriteString(trimmed)
		if strings.HasSuffix(trimmed, ",") || balancedQuoteCount(pending.String()) {
			flush()
		}
	}
	if pending.Len() > 0 {
		flush()
	}
	return uniqueStringItems(items)
}

func cleanMalformedStringItem(item string) string {
	item = strings.TrimSpace(item)
	item = strings.TrimSuffix(item, ",")
	item = strings.TrimSpace(item)
	item = strings.ReplaceAll(item, `\"`, `"`)
	item = strings.ReplaceAll(item, `\n`, " ")
	item = strings.ReplaceAll(item, "\r", " ")
	item = strings.ReplaceAll(item, "\n", " ")
	item = strings.Trim(item, `"`)
	item = strings.Join(strings.Fields(item), " ")
	return item
}

func balancedQuoteCount(s string) bool {
	count := 0
	escaped := false
	for _, ch := range s {
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' {
			escaped = true
			continue
		}
		if ch == '"' {
			count++
		}
	}
	return count > 0 && count%2 == 0
}

func uniqueStringItems(values []string) []string {
	seen := make(map[string]bool, len(values))
	unique := values[:0]
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		unique = append(unique, value)
	}
	return unique
}
