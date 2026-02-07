package media

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"testing"
)

// makeJPEG creates a minimal valid JPEG image.
func makeJPEG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 50})
	return buf.Bytes()
}

// makePNG creates a minimal valid PNG image.
func makePNG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// makeGIF creates a minimal valid GIF image.
func makeGIF(w, h int) []byte {
	palette := []color.Color{color.Black, color.White}
	img := image.NewPaletted(image.Rect(0, 0, w, h), palette)
	var buf bytes.Buffer
	_ = gif.Encode(&buf, img, nil)
	return buf.Bytes()
}

func FuzzGenerateThumbnail(f *testing.F) {
	// Seed with small valid images of each supported format
	f.Add(makeJPEG(1, 1))
	f.Add(makeJPEG(100, 100))
	f.Add(makeJPEG(500, 500))
	f.Add(makePNG(1, 1))
	f.Add(makePNG(100, 100))
	f.Add(makePNG(500, 500))
	f.Add(makeGIF(1, 1))
	f.Add(makeGIF(100, 100))

	// Edge cases
	f.Add([]byte{})
	f.Add([]byte("not an image"))
	f.Add([]byte{0xFF, 0xD8, 0xFF})                               // truncated JPEG header
	f.Add([]byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}) // PNG header only

	f.Fuzz(func(t *testing.T, data []byte) {
		// GenerateThumbnail must not panic on any input.
		// Errors and nil results are acceptable; panics are not.
		opts := DefaultThumbnailOptions()
		_, _ = GenerateThumbnail(data, opts)
	})
}
