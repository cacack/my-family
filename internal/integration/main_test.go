package integration_test

import (
	"flag"
	"fmt"
	"os"
	"testing"
)

// TestMain keeps the whole suite out of the fast tier: `go test -short` (the
// pre-commit hook) skips it, while pre-push, `make test` and CI run it (#1524).
// Skipping here rather than in each test covers new scenarios automatically.
func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		fmt.Println("skipping internal/integration in -short mode; pre-push and CI run it")
		os.Exit(0)
	}
	os.Exit(m.Run())
}
