//go:build dev

// Package web provides static file serving for the frontend.
package web

import (
	"io/fs"
	"os"
)

// GetFileSystem returns the development frontend directory.
// In dev mode, this returns the local filesystem for hot reload support.
func GetFileSystem() (fs.FS, error) {
	return os.DirFS("web/build"), nil
}

// IsDev returns true for development builds.
func IsDev() bool {
	return true
}
