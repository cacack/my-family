package memory_test

import (
	"testing"

	"github.com/cacack/my-family/internal/repository/memory"
	"github.com/cacack/my-family/internal/testutil/settingstest"
)

func TestSettingsStore(t *testing.T) {
	settingstest.Run(t, memory.NewSettingsStore())
}
