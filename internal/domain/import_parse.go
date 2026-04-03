package domain

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
)

// Parse reads JSONL from r and returns the parsed lines. Each non-empty line
// is decoded as a JSON object into a RawLine. Blank lines are skipped.
// Parsing stops at the first JSON decode error and returns a ParseError
// identifying the line.
func Parse(r io.Reader) ([]RawLine, error) {
	var lines []RawLine
	scanner := bufio.NewScanner(r)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		text := scanner.Text()
		if text == "" {
			continue
		}

		var line RawLine
		if err := json.Unmarshal([]byte(text), &line); err != nil {
			return nil, ParseError{Line: lineNum, Err: fmt.Errorf("invalid JSON: %w", err)}
		}
		lines = append(lines, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading JSONL: %w", err)
	}

	return lines, nil
}
