package generator

import (
	"bytes"
	"strings"
	"unicode/utf16"
)

// CleanAndValidateText converts supported BOM encodings to UTF-8, detects
// binary NUL payloads, and strips control characters that should never enter a
// generated prompt.
func CleanAndValidateText(data []byte) (string, bool) {
	if len(data) == 0 {
		return "", true
	}

	switch {
	case len(data) >= 3 && bytes.Equal(data[:3], []byte{0xEF, 0xBB, 0xBF}):
		data = data[3:]
	case len(data) >= 2 && data[0] == 0xFF && data[1] == 0xFE:
		data = decodeUTF16(data[2:], true)
	case len(data) >= 2 && data[0] == 0xFE && data[1] == 0xFF:
		data = decodeUTF16(data[2:], false)
	}

	checkLength := min(len(data), 1024)
	if bytes.IndexByte(data[:checkLength], 0) >= 0 {
		return "", false
	}

	validText := strings.ToValidUTF8(string(data), "")
	var builder strings.Builder
	builder.Grow(len(validText))
	for _, current := range validText {
		if current == '\n' || current == '\r' || current == '\t' || (current >= 32 && current != 127) {
			builder.WriteRune(current)
		}
	}
	return builder.String(), true
}

func decodeUTF16(data []byte, littleEndian bool) []byte {
	codeUnits := make([]uint16, 0, len(data)/2)
	for index := 0; index+1 < len(data); index += 2 {
		if littleEndian {
			codeUnits = append(codeUnits, uint16(data[index])|uint16(data[index+1])<<8)
		} else {
			codeUnits = append(codeUnits, uint16(data[index])<<8|uint16(data[index+1]))
		}
	}
	return []byte(string(utf16.Decode(codeUnits)))
}
