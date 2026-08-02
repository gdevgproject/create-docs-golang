package generator

import (
	"regexp"
	"strings"
)

var (
	trailingSpacesRegex = regexp.MustCompile(`(?m)[ \t]+$`)
	multiNewlinesRegex  = regexp.MustCompile(`\n{3,}`)
)

// SanitizeContent cleans source code string by normalizing newlines, trimming trailing whitespace, and collapsing blank lines
func SanitizeContent(content string) string {
	// Normalize CRLF / CR line endings to LF
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	// For large files (>200KB), line splitting is faster than regex replace
	if len(content) > 204800 {
		lines := strings.Split(content, "\n")
		for i, line := range lines {
			lines[i] = strings.TrimRight(line, " \t")
		}
		content = strings.Join(lines, "\n")
	} else {
		content = trailingSpacesRegex.ReplaceAllString(content, "")
	}

	// Collapse 3 or more consecutive newlines down to 2
	content = multiNewlinesRegex.ReplaceAllString(content, "\n\n")

	return content
}

// EscapeCDATA ensures CDATA closure string "]]>" in content does not break the XML enclosure
func EscapeCDATA(content string) string {
	if strings.Contains(content, "]]>") {
		return strings.ReplaceAll(content, "]]>", "]]]]><![CDATA[>")
	}
	return content
}
