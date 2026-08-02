package generator

import (
	"strings"
)

// SanitizeContent cleans source code string by normalizing line endings (CRLF/CR -> LF),
// trimming trailing whitespace per line, and collapsing 3+ consecutive newlines efficiently
func SanitizeContent(content string) string {
	if content == "" {
		return ""
	}

	// Normalize line endings
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	// Trim trailing space per line
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	content = strings.Join(lines, "\n")

	// Collapse 3 or more consecutive newlines down to 2
	for strings.Contains(content, "\n\n\n") {
		content = strings.ReplaceAll(content, "\n\n\n", "\n\n")
	}

	return content
}

// EscapeCDATA ensures CDATA closure string "]]>" in content does not break XML enclosure
func EscapeCDATA(content string) string {
	if strings.Contains(content, "]]>") {
		return strings.ReplaceAll(content, "]]>", "]]]]><![CDATA[>")
	}
	return content
}
