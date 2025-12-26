# Stage 7: Thumbnail Generation

<objective>
Create a thumbnail generation package using the disintegration/imaging library for resizing uploaded images.
</objective>

<context>
This stage can run in parallel with Stages 3-4 (database implementations).

External dependency:
- github.com/disintegration/imaging - Pure Go image processing library

Reference patterns:
- @file:internal/domain/media.go - Media struct for context
</context>

<requirements>

## 1. Add Dependency

```bash
go get github.com/disintegration/imaging
```

## 2. Create internal/media/thumbnail.go

```go
// Package media provides media processing utilities.
package media

import (
    "bytes"
    "image"
    "image/jpeg"
    _ "image/gif"  // Register GIF decoder
    _ "image/png"  // Register PNG decoder

    "github.com/disintegration/imaging"
)

// ThumbnailOptions configures thumbnail generation.
type ThumbnailOptions struct {
    MaxSize int     // Maximum dimension (width or height)
    Quality int     // JPEG quality (1-100)
}

// DefaultThumbnailOptions returns default thumbnail settings.
func DefaultThumbnailOptions() ThumbnailOptions {
    return ThumbnailOptions{
        MaxSize: 300,
        Quality: 85,
    }
}

// GenerateThumbnail creates a thumbnail from image bytes.
// Returns nil if the input is not a valid image or if generation fails.
// The thumbnail is always output as JPEG.
//
// maxSize is the maximum dimension - aspect ratio is preserved.
func GenerateThumbnail(data []byte, maxSize int) ([]byte, error) {
    if len(data) == 0 {
        return nil, nil
    }

    // Decode image
    img, _, err := image.Decode(bytes.NewReader(data))
    if err != nil {
        // Not a valid image - return nil without error
        // This allows non-image files to pass through
        return nil, nil
    }

    // Get dimensions
    bounds := img.Bounds()
    width := bounds.Dx()
    height := bounds.Dy()

    // If already smaller than max, just re-encode as JPEG
    if width <= maxSize && height <= maxSize {
        return encodeJPEG(img, 85)
    }

    // Calculate new dimensions preserving aspect ratio
    var newWidth, newHeight int
    if width > height {
        newWidth = maxSize
        newHeight = 0 // auto-calculate
    } else {
        newWidth = 0  // auto-calculate
        newHeight = maxSize
    }

    // Resize using Lanczos filter (high quality)
    thumbnail := imaging.Resize(img, newWidth, newHeight, imaging.Lanczos)

    // Encode as JPEG
    return encodeJPEG(thumbnail, 85)
}

// GenerateThumbnailWithOptions creates a thumbnail with custom options.
func GenerateThumbnailWithOptions(data []byte, opts ThumbnailOptions) ([]byte, error) {
    if len(data) == 0 {
        return nil, nil
    }

    img, _, err := image.Decode(bytes.NewReader(data))
    if err != nil {
        return nil, nil
    }

    bounds := img.Bounds()
    width := bounds.Dx()
    height := bounds.Dy()

    if width <= opts.MaxSize && height <= opts.MaxSize {
        return encodeJPEG(img, opts.Quality)
    }

    var newWidth, newHeight int
    if width > height {
        newWidth = opts.MaxSize
        newHeight = 0
    } else {
        newWidth = 0
        newHeight = opts.MaxSize
    }

    thumbnail := imaging.Resize(img, newWidth, newHeight, imaging.Lanczos)
    return encodeJPEG(thumbnail, opts.Quality)
}

// encodeJPEG encodes an image as JPEG with the specified quality.
func encodeJPEG(img image.Image, quality int) ([]byte, error) {
    var buf bytes.Buffer
    err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
    if err != nil {
        return nil, err
    }
    return buf.Bytes(), nil
}

// IsImageMimeType checks if a MIME type is a supported image format.
func IsImageMimeType(mimeType string) bool {
    switch mimeType {
    case "image/jpeg", "image/jpg", "image/png", "image/gif", "image/webp":
        return true
    default:
        return false
    }
}
```

## 3. Create internal/media/thumbnail_test.go

```go
package media

import (
    "image"
    "image/color"
    "image/jpeg"
    "image/png"
    "bytes"
    "testing"
)

func TestGenerateThumbnail_JPEG(t *testing.T) {
    // Create a 500x500 test image
    img := createTestImage(500, 500)
    data := encodeTestJPEG(img)

    thumb, err := GenerateThumbnail(data, 300)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if thumb == nil {
        t.Fatal("expected thumbnail, got nil")
    }

    // Verify thumbnail dimensions
    decoded, _, err := image.Decode(bytes.NewReader(thumb))
    if err != nil {
        t.Fatalf("failed to decode thumbnail: %v", err)
    }

    bounds := decoded.Bounds()
    if bounds.Dx() > 300 || bounds.Dy() > 300 {
        t.Errorf("thumbnail too large: %dx%d", bounds.Dx(), bounds.Dy())
    }
}

func TestGenerateThumbnail_PNG(t *testing.T) {
    img := createTestImage(400, 600)
    data := encodeTestPNG(img)

    thumb, err := GenerateThumbnail(data, 300)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if thumb == nil {
        t.Fatal("expected thumbnail, got nil")
    }

    // Verify aspect ratio preserved (should be 200x300 or similar)
    decoded, _, _ := image.Decode(bytes.NewReader(thumb))
    bounds := decoded.Bounds()

    // Height should be 300, width should be ~200
    if bounds.Dy() != 300 {
        t.Errorf("expected height 300, got %d", bounds.Dy())
    }
}

func TestGenerateThumbnail_SmallImage(t *testing.T) {
    // Image smaller than max should just be re-encoded
    img := createTestImage(100, 100)
    data := encodeTestJPEG(img)

    thumb, err := GenerateThumbnail(data, 300)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if thumb == nil {
        t.Fatal("expected thumbnail, got nil")
    }
}

func TestGenerateThumbnail_InvalidData(t *testing.T) {
    // Non-image data should return nil without error
    thumb, err := GenerateThumbnail([]byte("not an image"), 300)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if thumb != nil {
        t.Error("expected nil for non-image data")
    }
}

func TestGenerateThumbnail_EmptyData(t *testing.T) {
    thumb, err := GenerateThumbnail(nil, 300)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if thumb != nil {
        t.Error("expected nil for empty data")
    }
}

func TestIsImageMimeType(t *testing.T) {
    tests := []struct {
        mime string
        want bool
    }{
        {"image/jpeg", true},
        {"image/png", true},
        {"image/gif", true},
        {"image/webp", true},
        {"application/pdf", false},
        {"text/plain", false},
        {"", false},
    }

    for _, tt := range tests {
        if got := IsImageMimeType(tt.mime); got != tt.want {
            t.Errorf("IsImageMimeType(%q) = %v, want %v", tt.mime, got, tt.want)
        }
    }
}

// Test helpers

func createTestImage(width, height int) image.Image {
    img := image.NewRGBA(image.Rect(0, 0, width, height))
    for y := 0; y < height; y++ {
        for x := 0; x < width; x++ {
            img.Set(x, y, color.RGBA{
                R: uint8(x % 256),
                G: uint8(y % 256),
                B: 128,
                A: 255,
            })
        }
    }
    return img
}

func encodeTestJPEG(img image.Image) []byte {
    var buf bytes.Buffer
    jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90})
    return buf.Bytes()
}

func encodeTestPNG(img image.Image) []byte {
    var buf bytes.Buffer
    png.Encode(&buf, img)
    return buf.Bytes()
}
```

</requirements>

<implementation>

1. Add dependency: `go get github.com/disintegration/imaging`
2. Create `internal/media/` directory
3. Create `internal/media/thumbnail.go` with generation functions
4. Create `internal/media/thumbnail_test.go` with comprehensive tests
5. Run tests: `go test ./internal/media/...`

</implementation>

<verification>
```bash
# Install dependency
go get github.com/disintegration/imaging

# Build media package
go build ./internal/media/...

# Run tests
go test ./internal/media/... -v

# Check test coverage
go test ./internal/media/... -cover
```
</verification>

<output>
After completing this stage, provide:
1. Functions implemented
2. Test coverage
3. Image formats supported

Example output:
```
Stage 7 Complete: Thumbnail Generation

Created internal/media/thumbnail.go:
- GenerateThumbnail(data, maxSize) - simple API
- GenerateThumbnailWithOptions(data, opts) - configurable
- DefaultThumbnailOptions() - 300px, 85% quality
- IsImageMimeType(mime) - format check helper

Features:
- Preserves aspect ratio
- Lanczos resampling for quality
- Outputs JPEG for consistency
- Gracefully handles non-images (returns nil)

Tests (internal/media/thumbnail_test.go):
- JPEG thumbnail generation
- PNG thumbnail generation
- Small image passthrough
- Invalid/empty data handling
- MIME type detection

Test coverage: 95%+

Ready for Stage 8 (API)
```
</output>
