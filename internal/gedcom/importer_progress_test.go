package gedcom_test

import (
	"context"
	"strings"
	"testing"

	"github.com/cacack/my-family/internal/gedcom"
)

// TestImportWithOptions_ProgressCallback verifies that ImportWithOptions forwards
// decoder progress callbacks to the caller-supplied OnProgress function and that
// the final byte count reflects the full input size.
func TestImportWithOptions_ProgressCallback(t *testing.T) {
	importer := gedcom.NewImporter()
	ctx := context.Background()

	totalSize := int64(len(sampleGedcom))

	var calls int
	var lastBytesRead, lastTotalBytes int64
	opts := gedcom.ImportOptions{
		TotalSize: totalSize,
		OnProgress: func(bytesRead, totalBytes int64) {
			calls++
			lastBytesRead = bytesRead
			lastTotalBytes = totalBytes
		},
	}

	result, _, _, _, _, _, _, _, _, _, _, _, _, err := importer.ImportWithOptions(ctx, strings.NewReader(sampleGedcom), opts)
	if err != nil {
		t.Fatalf("ImportWithOptions failed: %v", err)
	}

	// Parsing must still succeed exactly as the plain Import path does.
	if result.PersonsImported != 3 {
		t.Errorf("PersonsImported = %d, want 3", result.PersonsImported)
	}

	if calls == 0 {
		t.Fatal("OnProgress was never called")
	}
	if lastTotalBytes != totalSize {
		t.Errorf("reported total bytes = %d, want %d", lastTotalBytes, totalSize)
	}
	if lastBytesRead <= 0 {
		t.Errorf("reported bytes read = %d, want > 0", lastBytesRead)
	}
	if lastBytesRead > lastTotalBytes {
		t.Errorf("bytes read %d exceeds total %d", lastBytesRead, lastTotalBytes)
	}
}

// TestImportWithOptions_NilCallback verifies that omitting OnProgress is safe and
// behaves identically to the plain Import path (zero progress-reporting overhead).
func TestImportWithOptions_NilCallback(t *testing.T) {
	importer := gedcom.NewImporter()
	ctx := context.Background()

	result, _, _, _, _, _, _, _, _, _, _, _, _, err := importer.ImportWithOptions(ctx, strings.NewReader(sampleGedcom), gedcom.ImportOptions{})
	if err != nil {
		t.Fatalf("ImportWithOptions failed: %v", err)
	}
	if result.PersonsImported != 3 {
		t.Errorf("PersonsImported = %d, want 3", result.PersonsImported)
	}
	if result.FamiliesImported != 1 {
		t.Errorf("FamiliesImported = %d, want 1", result.FamiliesImported)
	}
}
