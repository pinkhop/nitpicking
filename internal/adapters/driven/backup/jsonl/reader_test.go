package jsonl_test

import (
	"io"
	"strings"
	"testing"
	"time"

	"github.com/pinkhop/nitpicking/internal/adapters/driven/backup/jsonl"
	"github.com/pinkhop/nitpicking/internal/domain"
)

// nopReadCloser wraps an io.Reader to satisfy io.ReadCloser.
type nopReadCloser struct {
	io.Reader
}

func (nopReadCloser) Close() error { return nil }

func TestReader_ReadHeader_ParsesValidHeader(t *testing.T) {
	t.Parallel()

	// Given
	input := `{"prefix":"NP","timestamp":"2026-03-27T12:00:00Z","version":2}` + "\n"
	r := jsonl.NewReader(&nopReadCloser{strings.NewReader(input)})
	defer func() {
		_ = r.Close()
	}()

	// When
	header, err := r.ReadHeader()
	if err != nil {
		t.Fatalf("ReadHeader returned unexpected error: %v", err)
	}

	// Then
	if header.Prefix != "NP" {
		t.Errorf("prefix = %q, want %q", header.Prefix, "NP")
	}
	if header.Version != 2 {
		t.Errorf("version = %d, want %d", header.Version, 2)
	}
	if !header.Timestamp.Equal(time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC)) {
		t.Errorf("timestamp = %v, want 2026-03-27T12:00:00Z", header.Timestamp)
	}
}

func TestReader_NextRecord_IteratesAllRecords(t *testing.T) {
	t.Parallel()

	// Given
	input := `{"prefix":"NP","timestamp":"2026-03-27T12:00:00Z","version":2}
{"issue_id":"FOO-aaaa1","role":"task","title":"First","state":"open","labels":[],"comments":[],"relationships":[],"claims":[],"history":[]}
{"issue_id":"FOO-bbbb2","role":"epic","title":"Second","state":"closed","labels":[],"comments":[],"relationships":[],"claims":[],"history":[]}
`
	r := jsonl.NewReader(&nopReadCloser{strings.NewReader(input)})
	defer func() {
		_ = r.Close()
	}()

	_, err := r.ReadHeader()
	if err != nil {
		t.Fatalf("ReadHeader failed: %v", err)
	}

	// When
	var records []domain.BackupIssueRecord
	for {
		rec, ok, readErr := r.NextRecord()
		if readErr != nil {
			t.Fatalf("NextRecord returned unexpected error: %v", readErr)
		}
		if !ok {
			break
		}
		records = append(records, rec)
	}

	// Then
	if len(records) != 2 {
		t.Fatalf("got %d records, want 2", len(records))
	}
	if records[0].IssueID != "FOO-aaaa1" {
		t.Errorf("first record issue_id = %q, want %q", records[0].IssueID, "FOO-aaaa1")
	}
	if records[1].IssueID != "FOO-bbbb2" {
		t.Errorf("second record issue_id = %q, want %q", records[1].IssueID, "FOO-bbbb2")
	}
}

func TestReader_NextRecord_EmptyBackup_ReturnsFalse(t *testing.T) {
	t.Parallel()

	// Given — header only, no records.
	input := `{"prefix":"NP","timestamp":"2026-03-27T12:00:00Z","version":2}` + "\n"
	r := jsonl.NewReader(&nopReadCloser{strings.NewReader(input)})
	defer func() {
		_ = r.Close()
	}()

	_, err := r.ReadHeader()
	if err != nil {
		t.Fatalf("ReadHeader failed: %v", err)
	}

	// When
	_, ok, readErr := r.NextRecord()

	// Then
	if readErr != nil {
		t.Fatalf("NextRecord returned unexpected error: %v", readErr)
	}
	if ok {
		t.Error("NextRecord returned true for empty backup, want false")
	}
}

func TestRoundTrip_WriteAndReadProducesIdenticalData(t *testing.T) {
	t.Parallel()

	// Given
	header := domain.BackupHeader{
		Prefix:    "TEST",
		Timestamp: time.Date(2026, 1, 15, 8, 30, 0, 0, time.UTC),
		Version:   domain.BackupAlgorithmVersion,
	}

	records := []domain.BackupIssueRecord{
		{
			IssueID:            "TEST-abc01",
			Role:               "task",
			Title:              "A task with <HTML> & stuff",
			Description:        "Some description",
			AcceptanceCriteria: "Must work",
			Priority:           "P1",
			State:              "open",
			ParentID:           "TEST-xyz99",
			CreatedAt:          time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
			Labels: []domain.BackupLabelRecord{
				{Key: "kind", Value: "bug"},
				{Key: "area", Value: "backend"},
			},
			Comments: []domain.BackupCommentRecord{
				{CommentID: 1, Author: "alice", CreatedAt: time.Date(2026, 1, 11, 0, 0, 0, 0, time.UTC), Body: "Looks good"},
				{CommentID: 2, Author: "bob", CreatedAt: time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC), Body: "Agreed"},
			},
			Relationships: []domain.BackupRelationshipRecord{
				{TargetID: "TEST-xyz99", RelType: "blocked_by"},
			},
			// v2 backups exclude claim data; Claims is intentionally empty.
			Claims: []domain.BackupClaimRecord{},
			History: []domain.BackupHistoryRecord{
				{
					EntryID: 1, Revision: 0, Author: "alice",
					Timestamp: time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC),
					EventType: "created",
					Changes:   []domain.BackupFieldChangeRecord{{Field: "title", Before: "", After: "A task"}},
				},
			},
		},
		{
			IssueID:       "TEST-xyz99",
			Role:          "epic",
			Title:         "Parent epic",
			Priority:      "P2",
			State:         "open",
			Labels:        []domain.BackupLabelRecord{},
			Comments:      []domain.BackupCommentRecord{},
			Relationships: []domain.BackupRelationshipRecord{},
			Claims:        []domain.BackupClaimRecord{},
			History:       []domain.BackupHistoryRecord{},
		},
	}

	// When — write to a buffer, then read it back.
	var buf strings.Builder
	w := jsonl.NewWriter(&nopWriteCloser{&buf})

	if err := w.WriteHeader(header); err != nil {
		t.Fatalf("WriteHeader failed: %v", err)
	}
	for _, rec := range records {
		if err := w.WriteRecord(rec); err != nil {
			t.Fatalf("WriteRecord failed: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Writer.Close failed: %v", err)
	}

	r := jsonl.NewReader(&nopReadCloser{strings.NewReader(buf.String())})
	defer func() {
		_ = r.Close()
	}()

	gotHeader, err := r.ReadHeader()
	if err != nil {
		t.Fatalf("ReadHeader failed: %v", err)
	}

	var gotRecords []domain.BackupIssueRecord
	for {
		rec, ok, readErr := r.NextRecord()
		if readErr != nil {
			t.Fatalf("NextRecord failed: %v", readErr)
		}
		if !ok {
			break
		}
		gotRecords = append(gotRecords, rec)
	}

	// Then
	if gotHeader.Prefix != header.Prefix {
		t.Errorf("header prefix = %q, want %q", gotHeader.Prefix, header.Prefix)
	}
	if gotHeader.Version != header.Version {
		t.Errorf("header version = %d, want %d", gotHeader.Version, header.Version)
	}
	if !gotHeader.Timestamp.Equal(header.Timestamp) {
		t.Errorf("header timestamp = %v, want %v", gotHeader.Timestamp, header.Timestamp)
	}
	if len(gotRecords) != len(records) {
		t.Fatalf("got %d records, want %d", len(gotRecords), len(records))
	}

	// Verify first record in detail.
	got := gotRecords[0]
	want := records[0]
	if got.IssueID != want.IssueID {
		t.Errorf("record[0].IssueID = %q, want %q", got.IssueID, want.IssueID)
	}
	if got.Title != want.Title {
		t.Errorf("record[0].Title = %q, want %q", got.Title, want.Title)
	}
	if got.ParentID != want.ParentID {
		t.Errorf("record[0].ParentID = %q, want %q", got.ParentID, want.ParentID)
	}
	if len(got.Labels) != len(want.Labels) {
		t.Errorf("record[0].Labels count = %d, want %d", len(got.Labels), len(want.Labels))
	}
	if len(got.Comments) != len(want.Comments) {
		t.Errorf("record[0].Comments count = %d, want %d", len(got.Comments), len(want.Comments))
	}
	if len(got.Relationships) != len(want.Relationships) {
		t.Errorf("record[0].Relationships count = %d, want %d", len(got.Relationships), len(want.Relationships))
	}
	if len(got.Claims) != len(want.Claims) {
		t.Errorf("record[0].Claims count = %d, want %d", len(got.Claims), len(want.Claims))
	}
	if len(got.History) != len(want.History) {
		t.Errorf("record[0].History count = %d, want %d", len(got.History), len(want.History))
	}
	if len(got.History) > 0 && len(got.History[0].Changes) != len(want.History[0].Changes) {
		t.Errorf("record[0].History[0].Changes count = %d, want %d", len(got.History[0].Changes), len(want.History[0].Changes))
	}
}
