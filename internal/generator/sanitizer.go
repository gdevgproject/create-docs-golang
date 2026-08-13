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

	output := make([]byte, 0, len(content))
	for index := 0; index < len(content); index++ {
		current := content[index]
		if current == '\r' {
			if index+1 < len(content) && content[index+1] == '\n' {
				index++
			}
			current = '\n'
		}
		if current == '\n' {
			for len(output) > 0 && (output[len(output)-1] == ' ' || output[len(output)-1] == '\t') {
				output = output[:len(output)-1]
			}
			if len(output) >= 2 && output[len(output)-1] == '\n' && output[len(output)-2] == '\n' {
				continue
			}
		}
		output = append(output, current)
	}
	for len(output) > 0 && (output[len(output)-1] == ' ' || output[len(output)-1] == '\t') {
		output = output[:len(output)-1]
	}
	return string(output)
}

// EscapeCDATA ensures CDATA closure string "]]>" in content does not break XML enclosure
func EscapeCDATA(content string) string {
	if strings.Contains(content, "]]>") {
		return strings.ReplaceAll(content, "]]>", "]]]]><![CDATA[>")
	}
	return content
}
