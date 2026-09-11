package fileeditor

import "strings"

// lineEndingStyle is the newline style of one file.
type lineEndingStyle int

const (
	lineEndingLF lineEndingStyle = iota
	lineEndingCRLF
)

// detectLineEnding reports the file's newline style. A file containing any
// CRLF pair is treated as CRLF style.
func detectLineEnding(content []byte) lineEndingStyle {
	if strings.Contains(string(content), "\r\n") {
		return lineEndingCRLF
	}
	return lineEndingLF
}

// applyLineEndingStyle rewrites text to the file's newline style. Text built
// from model input always uses "\n"; matching and writing then happen in the
// file's own style so existing bytes keep their line endings.
func applyLineEndingStyle(text string, style lineEndingStyle) string {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	if style == lineEndingCRLF {
		return strings.ReplaceAll(normalized, "\n", "\r\n")
	}
	return normalized
}
