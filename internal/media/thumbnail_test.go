package media

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"strings"
	"testing"
)

// createTestImage creates a test image of specified dimensions.
func createTestImage(width, height int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	// Fill with a solid color
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{100, 150, 200, 255})
		}
	}
	return img
}

// encodeTestImageJPEG encodes a test image as JPEG.
func encodeTestImageJPEG(img image.Image) []byte {
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
	return buf.Bytes()
}

// encodeTestImagePNG encodes a test image as PNG.
func encodeTestImagePNG(img image.Image) []byte {
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func TestDefaultThumbnailOptions(t *testing.T) {
	opts := DefaultThumbnailOptions()

	if opts.MaxWidth != MaxThumbnailSize {
		t.Errorf("MaxWidth = %d, want %d", opts.MaxWidth, MaxThumbnailSize)
	}
	if opts.MaxHeight != MaxThumbnailSize {
		t.Errorf("MaxHeight = %d, want %d", opts.MaxHeight, MaxThumbnailSize)
	}
	if opts.Format != ThumbnailJPEG {
		t.Errorf("Format = %v, want %v", opts.Format, ThumbnailJPEG)
	}
	if opts.Quality != 85 {
		t.Errorf("Quality = %d, want 85", opts.Quality)
	}
}

func TestGenerateThumbnail(t *testing.T) {
	tests := []struct {
		name        string
		imgWidth    int
		imgHeight   int
		opts        ThumbnailOptions
		wantNil     bool
		wantResized bool
	}{
		{
			name:        "large image gets resized",
			imgWidth:    1000,
			imgHeight:   800,
			opts:        DefaultThumbnailOptions(),
			wantNil:     false,
			wantResized: true,
		},
		{
			name:        "small image stays same size",
			imgWidth:    100,
			imgHeight:   100,
			opts:        DefaultThumbnailOptions(),
			wantNil:     false,
			wantResized: false,
		},
		{
			name:        "image at max size stays same",
			imgWidth:    MaxThumbnailSize,
			imgHeight:   MaxThumbnailSize,
			opts:        DefaultThumbnailOptions(),
			wantNil:     false,
			wantResized: false,
		},
		{
			name:        "tall image gets resized",
			imgWidth:    200,
			imgHeight:   1000,
			opts:        DefaultThumbnailOptions(),
			wantNil:     false,
			wantResized: true,
		},
		{
			name:        "wide image gets resized",
			imgWidth:    1000,
			imgHeight:   200,
			opts:        DefaultThumbnailOptions(),
			wantNil:     false,
			wantResized: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			img := createTestImage(tt.imgWidth, tt.imgHeight)
			data := encodeTestImageJPEG(img)

			result, err := GenerateThumbnail(data, tt.opts)
			if err != nil {
				t.Fatalf("GenerateThumbnail() error = %v", err)
			}

			if tt.wantNil && result != nil {
				t.Errorf("expected nil result, got %d bytes", len(result))
			}
			if !tt.wantNil && result == nil {
				t.Error("expected non-nil result, got nil")
			}

			if result != nil {
				// Decode result to check dimensions
				decoded, _, err := image.Decode(bytes.NewReader(result))
				if err != nil {
					t.Fatalf("failed to decode result: %v", err)
				}

				bounds := decoded.Bounds()
				if tt.wantResized {
					if bounds.Dx() > tt.opts.MaxWidth || bounds.Dy() > tt.opts.MaxHeight {
						t.Errorf("result too large: %dx%d (max %dx%d)",
							bounds.Dx(), bounds.Dy(), tt.opts.MaxWidth, tt.opts.MaxHeight)
					}
				}
			}
		})
	}
}

func TestGenerateThumbnail_EmptyData(t *testing.T) {
	result, err := GenerateThumbnail([]byte{}, DefaultThumbnailOptions())
	if err != nil {
		t.Errorf("GenerateThumbnail() error = %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for empty data, got %d bytes", len(result))
	}
}

func TestGenerateThumbnail_InvalidData(t *testing.T) {
	result, err := GenerateThumbnail([]byte("not an image"), DefaultThumbnailOptions())
	if err != nil {
		t.Errorf("GenerateThumbnail() error = %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for invalid data, got %d bytes", len(result))
	}
}

func TestGenerateThumbnail_PNGFormat(t *testing.T) {
	img := createTestImage(500, 500)
	data := encodeTestImagePNG(img)

	opts := ThumbnailOptions{
		MaxWidth:  200,
		MaxHeight: 200,
		Format:    ThumbnailPNG,
	}

	result, err := GenerateThumbnail(data, opts)
	if err != nil {
		t.Fatalf("GenerateThumbnail() error = %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Verify it's a PNG by checking magic bytes
	if !bytes.HasPrefix(result, []byte{0x89, 0x50, 0x4E, 0x47}) {
		t.Error("result does not have PNG magic bytes")
	}
}

func TestGenerateThumbnail_JPEGFormat(t *testing.T) {
	img := createTestImage(500, 500)
	data := encodeTestImageJPEG(img)

	opts := ThumbnailOptions{
		MaxWidth:  200,
		MaxHeight: 200,
		Format:    ThumbnailJPEG,
		Quality:   90,
	}

	result, err := GenerateThumbnail(data, opts)
	if err != nil {
		t.Fatalf("GenerateThumbnail() error = %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Verify it's a JPEG by checking magic bytes
	if !bytes.HasPrefix(result, []byte{0xFF, 0xD8, 0xFF}) {
		t.Error("result does not have JPEG magic bytes")
	}
}

func TestGenerateThumbnail_InvalidQuality(t *testing.T) {
	img := createTestImage(500, 500)
	data := encodeTestImageJPEG(img)

	// Quality <= 0 should default to 85
	opts := ThumbnailOptions{
		MaxWidth:  200,
		MaxHeight: 200,
		Format:    ThumbnailJPEG,
		Quality:   0,
	}

	result, err := GenerateThumbnail(data, opts)
	if err != nil {
		t.Fatalf("GenerateThumbnail() error = %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Quality > 100 should also default to 85
	opts.Quality = 150
	result, err = GenerateThumbnail(data, opts)
	if err != nil {
		t.Fatalf("GenerateThumbnail() error = %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestGenerateThumbnailFromReader(t *testing.T) {
	img := createTestImage(500, 500)
	data := encodeTestImageJPEG(img)
	reader := bytes.NewReader(data)

	result, err := GenerateThumbnailFromReader(reader, DefaultThumbnailOptions())
	if err != nil {
		t.Fatalf("GenerateThumbnailFromReader() error = %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	// Verify the thumbnail was generated
	decoded, _, err := image.Decode(bytes.NewReader(result))
	if err != nil {
		t.Fatalf("failed to decode result: %v", err)
	}
	bounds := decoded.Bounds()
	if bounds.Dx() > MaxThumbnailSize || bounds.Dy() > MaxThumbnailSize {
		t.Errorf("result too large: %dx%d", bounds.Dx(), bounds.Dy())
	}
}

func TestIsImageMimeType(t *testing.T) {
	tests := []struct {
		mimeType string
		want     bool
	}{
		{"image/jpeg", true},
		{"image/png", true},
		{"image/gif", true},
		{"IMAGE/JPEG", true}, // Case insensitive
		{"Image/PNG", true},
		{"image/webp", false},
		{"image/svg+xml", false},
		{"application/pdf", false},
		{"text/plain", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			got := IsImageMimeType(tt.mimeType)
			if got != tt.want {
				t.Errorf("IsImageMimeType(%q) = %v, want %v", tt.mimeType, got, tt.want)
			}
		})
	}
}

func TestGenerateThumbnail_AspectRatioPreserved(t *testing.T) {
	// Create a wide image (2:1 aspect ratio)
	img := createTestImage(600, 300)
	data := encodeTestImageJPEG(img)

	opts := ThumbnailOptions{
		MaxWidth:  200,
		MaxHeight: 200,
		Format:    ThumbnailJPEG,
		Quality:   85,
	}

	result, err := GenerateThumbnail(data, opts)
	if err != nil {
		t.Fatalf("GenerateThumbnail() error = %v", err)
	}

	decoded, _, err := image.Decode(bytes.NewReader(result))
	if err != nil {
		t.Fatalf("failed to decode result: %v", err)
	}

	bounds := decoded.Bounds()
	// Should fit within bounds with aspect ratio preserved
	if bounds.Dx() > opts.MaxWidth || bounds.Dy() > opts.MaxHeight {
		t.Errorf("result exceeds bounds: %dx%d", bounds.Dx(), bounds.Dy())
	}

	// Aspect ratio should be approximately 2:1
	aspectRatio := float64(bounds.Dx()) / float64(bounds.Dy())
	if aspectRatio < 1.9 || aspectRatio > 2.1 {
		t.Errorf("aspect ratio not preserved: %f (expected ~2.0)", aspectRatio)
	}
}

func TestGenerateThumbnailFromReader_ErrorReading(t *testing.T) {
	// Use a reader that will fail
	reader := &errorReader{}
	_, err := GenerateThumbnailFromReader(reader, DefaultThumbnailOptions())
	if err == nil {
		t.Error("expected error from failing reader")
	}
	if !strings.Contains(err.Error(), "read image data") {
		t.Errorf("error should mention 'read image data', got: %v", err)
	}
}

// errorReader is a Reader that always returns an error.
type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, &testError{msg: "simulated read error"}
}

type testError struct {
	msg string
}

func (e *testError) Error() string {
	return e.msg
}
