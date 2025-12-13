//go:build !dev

// Package web provides embedded static files for the frontend.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var embeddedFiles embed.FS

// GetFileSystem returns the embedded frontend files.
// In production builds, this returns the embedded SvelteKit build output.
func GetFileSystem() (fs.FS, error) {
	return fs.Sub(embeddedFiles, "dist")
}

// IsDev returns false for production builds.
func IsDev() bool {
	return false
}
