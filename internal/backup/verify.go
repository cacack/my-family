package backup

import (
	"archive/zip"
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// maxManifestSize bounds the manifest a reader decodes; a real one is a few
// hundred bytes.
const maxManifestSize = 1 << 20

// VerifyFile verifies the archive at path; see Verify.
func VerifyFile(path string) (*Manifest, error) {
	// #nosec G304 -- path is the archive the operator named.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	return Verify(f, info.Size())
}

// Verify checks an archive without a database (ADR-010 §5): every entry's
// SHA-256 matches the manifest, the manifest lists every entry and the zip
// holds every listed one, and events.ndjson holds event_count events ending at
// last_position. An error names the entry at fault. The zip's own CRC-32 is
// not relied on.
func Verify(r io.ReaderAt, size int64) (*Manifest, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("not a readable zip archive: %w", err)
	}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		if _, dup := files[f.Name]; dup {
			return nil, fmt.Errorf("entry %s appears twice", f.Name)
		}
		files[f.Name] = f
	}

	manifest, err := readManifest(files[ManifestName])
	if err != nil {
		return nil, err
	}
	listed := map[string]bool{ManifestName: true}
	for _, e := range manifest.Entries {
		listed[e.Name] = true
		f, ok := files[e.Name]
		if !ok {
			return nil, fmt.Errorf("entry %s is listed in the manifest but missing", e.Name)
		}
		sum, err := entrySum(f, e.Name == EventsEntry, manifest)
		if err != nil {
			return nil, fmt.Errorf("entry %s is damaged: %w", e.Name, err)
		}
		if sum != e.SHA256 {
			return nil, fmt.Errorf("entry %s is damaged: its SHA-256 does not match the manifest", e.Name)
		}
	}
	for _, f := range zr.File {
		if !listed[f.Name] {
			return nil, fmt.Errorf("entry %s is not listed in the manifest", f.Name)
		}
	}
	if !listed[EventsEntry] {
		return nil, fmt.Errorf("entry %s is missing from the manifest", EventsEntry)
	}
	return manifest, nil
}

func readManifest(f *zip.File) (*Manifest, error) {
	if f == nil {
		return nil, fmt.Errorf("entry %s is missing", ManifestName)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("entry %s is damaged: %w", ManifestName, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, maxManifestSize))
	if err != nil {
		return nil, fmt.Errorf("entry %s is damaged: %w", ManifestName, err)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("entry %s is damaged: %w", ManifestName, err)
	}
	if m.FormatVersion < 1 {
		return nil, fmt.Errorf("entry %s is damaged: format_version %d", ManifestName, m.FormatVersion)
	}
	if m.FormatVersion > FormatVersion {
		return nil, fmt.Errorf("the archive's format_version %d is newer than this release reads (%d); use a newer my-family", m.FormatVersion, FormatVersion)
	}
	return &m, nil
}

// entrySum returns the SHA-256 of f's content. For the event log it also
// checks the lines against the manifest's event_count and last_position.
func entrySum(f *zip.File, events bool, m *Manifest) (string, error) {
	rc, err := f.Open()
	if err != nil {
		return "", err
	}
	defer rc.Close()
	h := sha256.New()
	if !events {
		// #nosec G110 -- verifying means reading every byte; they are hashed, not held.
		if _, err := io.Copy(h, rc); err != nil {
			return "", err
		}
		return hexSum(h), nil
	}

	lines := bufio.NewReader(io.TeeReader(rc, h))
	var count, last int64
	for {
		line, err := lines.ReadBytes('\n')
		if len(bytes.TrimSpace(line)) > 0 {
			var event struct {
				Position *int64 `json:"position"`
			}
			if jerr := json.Unmarshal(line, &event); jerr != nil || event.Position == nil {
				return "", fmt.Errorf("line %d is not an event", count+1)
			}
			if *event.Position <= last {
				return "", fmt.Errorf("line %d is out of position order", count+1)
			}
			count, last = count+1, *event.Position
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	if count != m.EventCount {
		return "", fmt.Errorf("it holds %d events, the manifest says %d", count, m.EventCount)
	}
	if last != m.LastPosition {
		return "", fmt.Errorf("it ends at position %d, the manifest says %d", last, m.LastPosition)
	}
	return hexSum(h), nil
}
