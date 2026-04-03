package cmdutil

import "strings"

// WordWrap wraps text to the given width at word boundaries. Existing newlines
// are preserved. When width is 0, the text is returned unchanged (no wrapping).
// Words longer than width are not broken.
func WordWrap(text string, width int) string {
	if width <= 0 {
		return text
	}

	lines := strings.Split(text, "\n")
	var result strings.Builder

	for i, line := range lines {
		if i > 0 {
			result.WriteByte('\n')
		}
		wrapLine(&result, line, width)
	}

	return result.String()
}

// wrapLine wraps a single line (no embedded newlines) to the given width.
func wrapLine(b *strings.Builder, line string, width int) {
	words := strings.Fields(line)
	if len(words) == 0 {
		return
	}

	lineLen := 0
	for i, word := range words {
		wordLen := len(word)

		if i == 0 {
			b.WriteString(word)
			lineLen = wordLen
			continue
		}

		// Check if adding this word (with a space) exceeds the width.
		if lineLen+1+wordLen > width {
			b.WriteByte('\n')
			b.WriteString(word)
			lineLen = wordLen
		} else {
			b.WriteByte(' ')
			b.WriteString(word)
			lineLen += 1 + wordLen
		}
	}
}
