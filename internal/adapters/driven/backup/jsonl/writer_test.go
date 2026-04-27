package jsonl_test

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/backup/jsonl"
	"github.com/pinkhop/nitpicking/internal/domain"
)

// nopCloser wraps an io.Writer to satisfy io.WriteCloser.
type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }

func TestWriter_WriteHeader_ProducesValidJSONLine(t *testing.T) {
	t.Parallel()

	// Given
	var buf bytes.Buffer
	w := jsonl.NewWriter(&nopWriteCloser{&buf})

	header := domain.BackupHeader{
		Prefix:    "NP",
		Timestamp: time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC),
		Version:   domain.BackupAlgorithmVersion,
	}

	// When
	err := w.WriteHeader(header)
	if err != nil {
		t.Fatalf("WriteHeader returned unexpected error: %v", err)
	}

	// Then
	line := strings.TrimSpace(buf.String())
	var got domain.BackupHeader
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if got.Prefix != "NP" {
		t.Errorf("prefix = %q, want %q", got.Prefix, "NP")
	}
	if got.Version != domain.BackupAlgorithmVersion {
		t.Errorf("version = %d, want %d", got.Version, domain.BackupAlgorithmVersion)
	}
}

func TestWriter_WriteRecord_ProducesValidJSONLine(t *testing.T) {
	t.Parallel()

	// Given
	var buf bytes.Buffer
	w := jsonl.NewWriter(&nopWriteCloser{&buf})

	record := domain.BackupIssueRecord{
		IssueID:  "FOO-a3bxr",
		Role:     "task",
		Title:    "Test issue",
		Priority: "P2",
		State:    "open",
		Labels: []domain.BackupLabelRecord{
			{Key: "kind", Value: "feat"},
		},
		Comments: []domain.BackupCommentRecord{
			{CommentID: 1, Author: "alice", Body: "hello", CreatedAt: time.Now()},
		},
	}

	// When
	err := w.WriteRecord(record)
	if err != nil {
		t.Fatalf("WriteRecord returned unexpected error: %v", err)
	}

	// Then
	line := strings.TrimSpace(buf.String())
	var got domain.BackupIssueRecord
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	if got.IssueID != "FOO-a3bxr" {
		t.Errorf("issue_id = %q, want %q", got.IssueID, "FOO-a3bxr")
	}
	if len(got.Labels) != 1 {
		t.Errorf("labels count = %d, want 1", len(got.Labels))
	}
	if len(got.Comments) != 1 {
		t.Errorf("comments count = %d, want 1", len(got.Comments))
	}
}

func TestWriter_HTMLCharactersNotEscaped(t *testing.T) {
	t.Parallel()

	// Given
	var buf bytes.Buffer
	w := jsonl.NewWriter(&nopWriteCloser{&buf})

	record := domain.BackupIssueRecord{
		IssueID: "FOO-html1",
		Role:    "task",
		Title:   "Fix <script> injection & XSS",
		State:   "open",
	}

	// When
	err := w.WriteRecord(record)
	if err != nil {
		t.Fatalf("WriteRecord returned unexpected error: %v", err)
	}

	// Then — the raw output must contain literal angle brackets, not
	// \u003c / \u003e escapes.
	output := buf.String()
	if strings.Contains(output, `\u003c`) || strings.Contains(output, `\u003e`) {
		t.Errorf("HTML characters were escaped: %s", output)
	}
	if !strings.Contains(output, "<script>") {
		t.Errorf("expected literal <script> in output: %s", output)
	}
}
