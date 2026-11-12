package repository

import (
	"bytes"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestUnmarshalJSONColumns(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(prev)

	id := uuid.New()

	if got := UnmarshalAddress(nil, "repositories", id); got != nil {
		t.Errorf("empty address = %+v, want nil", got)
	}
	if got := UnmarshalAddress([]byte(`{"city":"Springfield"}`), "repositories", id); got == nil || got.City != "Springfield" {
		t.Errorf("valid address = %+v, want city Springfield", got)
	}
	if got := UnmarshalStrings([]byte(`["555-0100"]`), "submitters", "phone", id); !slices.Equal(got, []string{"555-0100"}) {
		t.Errorf("valid phone = %v", got)
	}
	if buf.Len() != 0 {
		t.Fatalf("valid input logged a warning: %s", buf.String())
	}

	if got := UnmarshalAddress([]byte(`{not json`), "repositories", id); got != nil {
		t.Errorf("corrupt address = %+v, want nil", got)
	}
	if got := UnmarshalStrings([]byte(`"not-a-list"`), "submitters", "email", id); got != nil {
		t.Errorf("corrupt email = %v, want nil", got)
	}

	logged := buf.String()
	for _, want := range []string{"table=repositories column=address", "table=submitters column=email", id.String()} {
		if !strings.Contains(logged, want) {
			t.Errorf("warning log missing %q:\n%s", want, logged)
		}
	}
}
