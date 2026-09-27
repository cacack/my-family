//go:build !cgo

package storage

// cgoEnabled reports whether this binary was built with cgo. The SQLite driver
// (github.com/mattn/go-sqlite3) is a cgo package: without cgo it compiles to a
// stub whose every Open fails at runtime.
const cgoEnabled = false
