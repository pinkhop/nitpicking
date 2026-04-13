package backupcmd_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pinkhop/nitpicking/internal/cmd/admincmd/backupcmd"
)

// --- Tests: default path (no --output) ---

func TestRun_DefaultPath_WritesToNpDirectory(t *testing.T) {
	t.Parallel()

	// Given — a temporary .np/ directory simulating a discovered database.
	tmpDir := t.TempDir()
	npDir := filepath.Join(tmpDir, ".np")
	if err := os.MkdirAll(npDir, 0o755); err != nil {
		t.Fatalf("creating .np dir: %v", err)
	}
	dbPath := filepath.Join(npDir, "np.db")

	var buf bytes.Buffer
	input := backupcmd.RunInput{
		DiscoverFunc: func() (string, error) { return dbPath, nil },
		BackupFunc:   func(w io.WriteCloser) (int, error) { return 3, nil },
		WriteTo:      &buf,
	}

	// When
	err := backupcmd.Run(t.Context(), input)
	// Then — no error and backup file created in .np/ directory.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, err := os.ReadDir(npDir)
	if err != nil {
		t.Fatalf("reading .np dir: %v", err)
	}

	var found bool
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "backup.") && strings.HasSuffix(e.Name(), ".jsonl.gz") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected backup file in .np/ directory, found none")
	}

	// Human-readable output should mention the path and count.
	output := buf.String()
	if !strings.Contains(output, "3 issues") {
		t.Errorf("output should mention issue count, got: %s", output)
	}
}

func TestRun_DefaultPath_JSON_ReturnsStructuredOutput(t *testing.T) {
	t.Parallel()

	// Given
	tmpDir := t.TempDir()
	npDir := filepath.Join(tmpDir, ".np")
	if err := os.MkdirAll(npDir, 0o755); err != nil {
		t.Fatalf("creating .np dir: %v", err)
	}
	dbPath := filepath.Join(npDir, "np.db")

	var buf bytes.Buffer
	input := backupcmd.RunInput{
		DiscoverFunc: func() (string, error) { return dbPath, nil },
		BackupFunc:   func(w io.WriteCloser) (int, error) { return 5, nil },
		WriteTo:      &buf,
		JSON:         true,
	}

	// When
	err := backupcmd.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if jsonErr := json.Unmarshal(buf.Bytes(), &result); jsonErr != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", jsonErr, buf.String())
	}
	if result["issue_count"] != float64(5) {
		t.Errorf("issue_count: got %v, want 5", result["issue_count"])
	}
	path, ok := result["path"].(string)
	if !ok || !strings.HasPrefix(path, npDir) {
		t.Errorf("path should be in .np/ dir, got: %v", result["path"])
	}
}

func TestRun_DefaultPath_WithPrefix_IncludesPrefixInFilename(t *testing.T) {
	t.Parallel()

	// Given — a temporary .np/ directory and a prefix.
	tmpDir := t.TempDir()
	npDir := filepath.Join(tmpDir, ".np")
	if err := os.MkdirAll(npDir, 0o755); err != nil {
		t.Fatalf("creating .np dir: %v", err)
	}
	dbPath := filepath.Join(npDir, "np.db")

	var buf bytes.Buffer
	input := backupcmd.RunInput{
		DiscoverFunc: func() (string, error) { return dbPath, nil },
		Prefix:       "PKHP",
		BackupFunc:   func(w io.WriteCloser) (int, error) { return 3, nil },
		WriteTo:      &buf,
	}

	// When
	err := backupcmd.Run(t.Context(), input)
	// Then — backup filename includes lowercase prefix.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, err := os.ReadDir(npDir)
	if err != nil {
		t.Fatalf("reading .np dir: %v", err)
	}

	var found bool
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "backup-pkhp.") && strings.HasSuffix(e.Name(), ".jsonl.gz") {
			found = true
			break
		}
	}
	if !found {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected backup file with prefix in .np/ directory, found: %v", names)
	}
}

func TestRun_DefaultPath_WithPrefix_JSON_IncludesPrefixInPath(t *testing.T) {
	t.Parallel()

	// Given
	tmpDir := t.TempDir()
	npDir := filepath.Join(tmpDir, ".np")
	if err := os.MkdirAll(npDir, 0o755); err != nil {
		t.Fatalf("creating .np dir: %v", err)
	}
	dbPath := filepath.Join(npDir, "np.db")

	var buf bytes.Buffer
	input := backupcmd.RunInput{
		DiscoverFunc: func() (string, error) { return dbPath, nil },
		Prefix:       "TST",
		BackupFunc:   func(w io.WriteCloser) (int, error) { return 5, nil },
		WriteTo:      &buf,
		JSON:         true,
	}

	// When
	err := backupcmd.Run(t.Context(), input)
	// Then — JSON output path includes the prefix.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if jsonErr := json.Unmarshal(buf.Bytes(), &result); jsonErr != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", jsonErr, buf.String())
	}
	path, ok := result["path"].(string)
	if !ok {
		t.Fatalf("path not a string: %v", result["path"])
	}
	if !strings.Contains(path, "backup-tst.") {
		t.Errorf("path should contain 'backup-tst.', got: %s", path)
	}
}

func TestRun_DefaultPath_EmptyPrefix_FallsBackToOriginalFormat(t *testing.T) {
	t.Parallel()

	// Given — no prefix provided.
	tmpDir := t.TempDir()
	npDir := filepath.Join(tmpDir, ".np")
	if err := os.MkdirAll(npDir, 0o755); err != nil {
		t.Fatalf("creating .np dir: %v", err)
	}
	dbPath := filepath.Join(npDir, "np.db")

	var buf bytes.Buffer
	input := backupcmd.RunInput{
		DiscoverFunc: func() (string, error) { return dbPath, nil },
		BackupFunc:   func(w io.WriteCloser) (int, error) { return 1, nil },
		WriteTo:      &buf,
	}

	// When
	err := backupcmd.Run(t.Context(), input)
	// Then — backup filename uses the original format without prefix.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, err := os.ReadDir(npDir)
	if err != nil {
		t.Fatalf("reading .np dir: %v", err)
	}

	var found bool
	for _, e := range entries {
		// Original format: backup.<timestamp>.jsonl.gz (no prefix dash).
		if strings.HasPrefix(e.Name(), "backup.") && strings.HasSuffix(e.Name(), ".jsonl.gz") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected backup file with original format in .np/ directory")
	}
}

// --- Tests: prefix sanitization ---

func TestRun_DefaultPath_MaliciousPrefix_SanitizedInFilename(t *testing.T) {
	t.Parallel()

	// Given — a prefix containing path traversal characters.
	tmpDir := t.TempDir()
	npDir := filepath.Join(tmpDir, ".np")
	if err := os.MkdirAll(npDir, 0o755); err != nil {
		t.Fatalf("creating .np dir: %v", err)
	}
	dbPath := filepath.Join(npDir, "np.db")

	var buf bytes.Buffer
	input := backupcmd.RunInput{
		DiscoverFunc: func() (string, error) { return dbPath, nil },
		BackupFunc:   func(w io.WriteCloser) (int, error) { return 1, nil },
		WriteTo:      &buf,
		Prefix:       "../../etc/NP",
	}

	// When
	err := backupcmd.Run(t.Context(), input)
	// Then — backup stays inside .np/ and path traversal characters are stripped.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, err := os.ReadDir(npDir)
	if err != nil {
		t.Fatalf("reading .np dir: %v", err)
	}

	var found bool
	for _, e := range entries {
		// Path-unsafe characters stripped, only letters remain: "etcnp"
		if strings.HasPrefix(e.Name(), "backup-etcnp.") && strings.HasSuffix(e.Name(), ".jsonl.gz") {
			found = true
			break
		}
	}
	if !found {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected sanitized backup file in .np/ directory, found: %v", names)
	}

	// Verify no files were created outside .np/.
	parentEntries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("reading parent dir: %v", err)
	}
	for _, e := range parentEntries {
		if e.Name() != ".np" {
			t.Errorf("unexpected file outside .np/: %s", e.Name())
		}
	}
}

func TestRun_OutputFlagDirectory_MaliciousPrefix_StaysInDirectory(t *testing.T) {
	t.Parallel()

	// Given — a directory output with a malicious prefix.
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "backups")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatalf("creating output dir: %v", err)
	}

	var buf bytes.Buffer
	input := backupcmd.RunInput{
		DiscoverFunc: func() (string, error) { return "", nil },
		BackupFunc:   func(w io.WriteCloser) (int, error) { return 1, nil },
		WriteTo:      &buf,
		Output:       outputDir,
		Prefix:       "../../../tmp/EVIL",
	}

	// When
	err := backupcmd.Run(t.Context(), input)
	// Then — backup stays inside the output directory.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatalf("reading output dir: %v", err)
	}

	var found bool
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "backup-tmpevil.") && strings.HasSuffix(e.Name(), ".jsonl.gz") {
			found = true
			break
		}
	}
	if !found {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected sanitized backup file in output directory, found: %v", names)
	}
}

// --- Tests: --output flag ---

func TestRun_OutputFlag_WritesToSpecifiedPath(t *testing.T) {
	t.Parallel()

	// Given — a custom output path.
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "custom-backup.jsonl.gz")

	// Discovery is still needed for the service, but the file goes to outputPath.
	npDir := filepath.Join(tmpDir, ".np")
	if err := os.MkdirAll(npDir, 0o755); err != nil {
		t.Fatalf("creating .np dir: %v", err)
	}
	dbPath := filepath.Join(npDir, "np.db")

	var buf bytes.Buffer
	input := backupcmd.RunInput{
		DiscoverFunc: func() (string, error) { return dbPath, nil },
		BackupFunc:   func(w io.WriteCloser) (int, error) { return 7, nil },
		WriteTo:      &buf,
		Output:       outputPath,
	}

	// When
	err := backupcmd.Run(t.Context(), input)
	// Then — file created at specified path.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, statErr := os.Stat(outputPath); statErr != nil {
		t.Fatalf("backup file not found at %s: %v", outputPath, statErr)
	}

	// Human-readable output should reference the custom path.
	output := buf.String()
	if !strings.Contains(output, outputPath) {
		t.Errorf("output should mention custom path %q, got: %s", outputPath, output)
	}
}

func TestRun_OutputFlag_JSON_PathMatchesOutputFlag(t *testing.T) {
	t.Parallel()

	// Given
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "my-backup.jsonl.gz")

	npDir := filepath.Join(tmpDir, ".np")
	if err := os.MkdirAll(npDir, 0o755); err != nil {
		t.Fatalf("creating .np dir: %v", err)
	}
	dbPath := filepath.Join(npDir, "np.db")

	var buf bytes.Buffer
	input := backupcmd.RunInput{
		DiscoverFunc: func() (string, error) { return dbPath, nil },
		BackupFunc:   func(w io.WriteCloser) (int, error) { return 2, nil },
		WriteTo:      &buf,
		Output:       outputPath,
		JSON:         true,
	}

	// When
	err := backupcmd.Run(t.Context(), input)
	// Then
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if jsonErr := json.Unmarshal(buf.Bytes(), &result); jsonErr != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", jsonErr, buf.String())
	}
	if result["path"] != outputPath {
		t.Errorf("path: got %v, want %q", result["path"], outputPath)
	}
}

func TestRun_OutputFlag_CreatesGzipFile(t *testing.T) {
	t.Parallel()

	// Given — output path and a backup function that writes data.
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "test.jsonl.gz")

	npDir := filepath.Join(tmpDir, ".np")
	if err := os.MkdirAll(npDir, 0o755); err != nil {
		t.Fatalf("creating .np dir: %v", err)
	}
	dbPath := filepath.Join(npDir, "np.db")

	var buf bytes.Buffer
	input := backupcmd.RunInput{
		DiscoverFunc: func() (string, error) { return dbPath, nil },
		BackupFunc: func(w io.WriteCloser) (int, error) {
			_, _ = w.Write([]byte(`{"test": true}` + "\n"))
			return 1, nil
		},
		WriteTo: &buf,
		Output:  outputPath,
	}

	// When
	err := backupcmd.Run(t.Context(), input)
	// Then — the file should be valid gzip.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	file, err := os.Open(outputPath)
	if err != nil {
		t.Fatalf("opening backup: %v", err)
	}
	defer func() { _ = file.Close() }()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("not a valid gzip file: %v", err)
	}
	defer func() { _ = gzr.Close() }()

	data, err := io.ReadAll(gzr)
	if err != nil {
		t.Fatalf("reading gzip content: %v", err)
	}
	if !strings.Contains(string(data), `{"test": true}`) {
		t.Errorf("gzip content missing expected data, got: %s", string(data))
	}
}

func TestRun_OutputFlag_InvalidPath_ReturnsError(t *testing.T) {
	t.Parallel()

	// Given — an output path in a non-existent directory.
	tmpDir := t.TempDir()
	outputPath := filepath.Join(tmpDir, "nonexistent", "deep", "backup.jsonl.gz")

	npDir := filepath.Join(tmpDir, ".np")
	if err := os.MkdirAll(npDir, 0o755); err != nil {
		t.Fatalf("creating .np dir: %v", err)
	}
	dbPath := filepath.Join(npDir, "np.db")

	var buf bytes.Buffer
	input := backupcmd.RunInput{
		DiscoverFunc: func() (string, error) { return dbPath, nil },
		BackupFunc:   func(w io.WriteCloser) (int, error) { return 0, nil },
		WriteTo:      &buf,
		Output:       outputPath,
	}

	// When
	err := backupcmd.Run(t.Context(), input)

	// Then — should return an error about creating the file.
	if err == nil {
		t.Fatal("expected error for invalid output path, got nil")
	}
	if !strings.Contains(err.Error(), "creating backup file") {
		t.Errorf("error should mention file creation, got: %v", err)
	}
}

// --- Tests: --output flag with directory ---

func TestRun_OutputFlagDirectory_UsesDefaultFilenameInDirectory(t *testing.T) {
	t.Parallel()

	// Given — a directory as the output path.
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "backups")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatalf("creating output dir: %v", err)
	}

	var buf bytes.Buffer
	input := backupcmd.RunInput{
		DiscoverFunc: func() (string, error) { return "", nil },
		BackupFunc:   func(w io.WriteCloser) (int, error) { return 4, nil },
		WriteTo:      &buf,
		Output:       outputDir,
		Prefix:       "TST",
	}

	// When
	err := backupcmd.Run(t.Context(), input)
	// Then — backup file created in the specified directory with the default filename.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatalf("reading output dir: %v", err)
	}

	var found bool
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "backup-tst.") && strings.HasSuffix(e.Name(), ".jsonl.gz") {
			found = true
			break
		}
	}
	if !found {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected backup file with prefix in output directory, found: %v", names)
	}
}

func TestRun_OutputFlagDirectory_NoPrefix_UsesOriginalFilename(t *testing.T) {
	t.Parallel()

	// Given — a directory as the output path, no prefix.
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "backups")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatalf("creating output dir: %v", err)
	}

	var buf bytes.Buffer
	input := backupcmd.RunInput{
		DiscoverFunc: func() (string, error) { return "", nil },
		BackupFunc:   func(w io.WriteCloser) (int, error) { return 2, nil },
		WriteTo:      &buf,
		Output:       outputDir,
	}

	// When
	err := backupcmd.Run(t.Context(), input)
	// Then — backup file uses original format without prefix.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	entries, err := os.ReadDir(outputDir)
	if err != nil {
		t.Fatalf("reading output dir: %v", err)
	}

	var found bool
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "backup.") && strings.HasSuffix(e.Name(), ".jsonl.gz") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected backup file with original format in output directory")
	}
}

func TestRun_OutputFlagDirectory_JSON_PathIncludesDirectory(t *testing.T) {
	t.Parallel()

	// Given — a directory as the output path with JSON output.
	tmpDir := t.TempDir()
	outputDir := filepath.Join(tmpDir, "backups")
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		t.Fatalf("creating output dir: %v", err)
	}

	var buf bytes.Buffer
	input := backupcmd.RunInput{
		DiscoverFunc: func() (string, error) { return "", nil },
		BackupFunc:   func(w io.WriteCloser) (int, error) { return 3, nil },
		WriteTo:      &buf,
		Output:       outputDir,
		Prefix:       "NP",
		JSON:         true,
	}

	// When
	err := backupcmd.Run(t.Context(), input)
	// Then — JSON output path is inside the specified directory.
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var result map[string]any
	if jsonErr := json.Unmarshal(buf.Bytes(), &result); jsonErr != nil {
		t.Fatalf("invalid JSON: %v\nraw: %s", jsonErr, buf.String())
	}
	path, ok := result["path"].(string)
	if !ok {
		t.Fatalf("path not a string: %v", result["path"])
	}
	if !strings.HasPrefix(path, outputDir) {
		t.Errorf("path should start with output dir %q, got: %s", outputDir, path)
	}
	if !strings.Contains(path, "backup-np.") {
		t.Errorf("path should contain 'backup-np.', got: %s", path)
	}
}
