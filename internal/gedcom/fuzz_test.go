package gedcom

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func FuzzImport(f *testing.F) {
	// Seed from testdata GEDCOM files
	testdataDir := filepath.Join("..", "..", "testdata", "gedcom-5.5")
	entries, err := os.ReadDir(testdataDir)
	if err == nil {
		for _, entry := range entries {
			if filepath.Ext(entry.Name()) == ".ged" {
				data, err := os.ReadFile(filepath.Join(testdataDir, entry.Name()))
				if err == nil {
					f.Add(data)
				}
			}
		}
	}

	// Synthetic seeds
	f.Add([]byte(""))
	f.Add([]byte("0 HEAD\n1 SOUR test\n0 TRLR\n"))
	f.Add([]byte("0 HEAD\r\n1 SOUR test\r\n0 TRLR\r\n"))
	f.Add([]byte("0 HEAD\n1 CHAR ANSEL\n0 @I1@ INDI\n1 NAME John /Doe/\n0 TRLR\n"))
	f.Add([]byte("0 HEAD\n0 @I1@ INDI\n1 NAME Test\n1 BIRT\n2 DATE 1 JAN 1900\n0 TRLR\n"))
	f.Add([]byte("0 HEAD\n0 @F1@ FAM\n1 HUSB @I1@\n1 WIFE @I2@\n0 TRLR\n"))
	f.Add([]byte("not a gedcom file at all"))
	f.Add([]byte("0"))
	f.Add([]byte("0 HEAD\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		// Import must not panic on any input.
		// Errors are acceptable; panics are not.
		imp := NewImporter()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _, _, _, _, _, _, _, _, _, _, _, _, _ = imp.Import(ctx, bytes.NewReader(data))
	})
}
