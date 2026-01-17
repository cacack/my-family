// Package media provides media processing utilities for the genealogy application.
package media

import (
	"bytes"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"strings"

	"golang.org/x/image/draw"
)

// MaxThumbnailSize is the maximum dimension (width or height) for thumbnails.
const MaxThumbnailSize = 300

// ThumbnailFormat is the output format for thumbnails.
type ThumbnailFormat string

const (
	ThumbnailJPEG ThumbnailFormat = "jpeg"
	ThumbnailPNG  ThumbnailFormat = "png"
)

// ThumbnailOptions configures thumbnail generation.
type ThumbnailOptions struct {
	MaxWidth  int
	MaxHeight int
	Format    ThumbnailFormat
	Quality   int // JPEG quality (1-100), ignored for PNG
}

// DefaultThumbnailOptions returns sensible defaults for thumbnail generation.
func DefaultThumbnailOptions() ThumbnailOptions {
	return ThumbnailOptions{
		MaxWidth:  MaxThumbnailSize,
		MaxHeight: MaxThumbnailSize,
		Format:    ThumbnailJPEG,
		Quality:   85,
	}
}

// GenerateThumbnail creates a thumbnail from image data.
// Returns nil if the input is not a supported image format.
func GenerateThumbnail(data []byte, opts ThumbnailOptions) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}

	// Decode image
	img, format, err := decodeImage(data)
	if err != nil {
		// Not a valid image - return nil without error
		return nil, nil
	}

	// Skip if already smaller than thumbnail size
	bounds := img.Bounds()
	if bounds.Dx() <= opts.MaxWidth && bounds.Dy() <= opts.MaxHeight {
		// Already small enough - return original in thumbnail format
		return encodeImage(img, opts)
	}

	// Resize with aspect ratio preserved using CatmullRom resampling
	thumbnail := fitImage(img, opts.MaxWidth, opts.MaxHeight)

	// Encode thumbnail
	result, err := encodeImage(thumbnail, opts)
	if err != nil {
		return nil, fmt.Errorf("encode thumbnail: %w (original format: %s)", err, format)
	}

	return result, nil
}

// GenerateThumbnailFromReader creates a thumbnail from an io.Reader.
func GenerateThumbnailFromReader(r io.Reader, opts ThumbnailOptions) ([]byte, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read image data: %w", err)
	}
	return GenerateThumbnail(data, opts)
}

// IsImageMimeType checks if the MIME type represents an image we can process.
func IsImageMimeType(mimeType string) bool {
	mimeType = strings.ToLower(mimeType)
	switch mimeType {
	case "image/jpeg", "image/png", "image/gif":
		return true
	default:
		return false
	}
}

// fitImage resizes an image to fit within maxWidth x maxHeight while preserving aspect ratio.
// Uses CatmullRom interpolation for high-quality results.
func fitImage(img image.Image, maxWidth, maxHeight int) *image.RGBA {
	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	// Calculate scale factor to fit within maxWidth × maxHeight
	scaleW := float64(maxWidth) / float64(srcW)
	scaleH := float64(maxHeight) / float64(srcH)
	scale := scaleW
	if scaleH < scaleW {
		scale = scaleH
	}

	newW := int(float64(srcW) * scale)
	newH := int(float64(srcH) * scale)

	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, bounds, draw.Src, nil)
	return dst
}

// decodeImage decodes image data to an image.Image.
func decodeImage(data []byte) (image.Image, string, error) {
	r := bytes.NewReader(data)
	img, format, err := image.Decode(r)
	if err != nil {
		return nil, "", fmt.Errorf("decode image: %w", err)
	}
	return img, format, nil
}

// encodeImage encodes an image.Image to bytes in the specified format.
func encodeImage(img image.Image, opts ThumbnailOptions) ([]byte, error) {
	var buf bytes.Buffer

	switch opts.Format {
	case ThumbnailPNG:
		if err := png.Encode(&buf, img); err != nil {
			return nil, err
		}
	default: // ThumbnailJPEG or any unknown format defaults to JPEG
		quality := opts.Quality
		if quality <= 0 || quality > 100 {
			quality = 85
		}
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

// init registers image decoders
func init() {
	// Standard library decoders are auto-registered via blank imports
	// but we need to ensure gif is registered for reading
	_ = gif.Decode
}
